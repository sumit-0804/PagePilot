package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
)

type ChatService struct {
	db        *gorm.DB
	docs      *DocumentService
	ragEngine *rag.Engine
	llm       rag.LLMClient
	quiz      *QuizService
	cards     *FlashcardService
	summaries *SummaryService
}

func NewChatService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine, llm rag.LLMClient, quiz *QuizService, cards *FlashcardService, summaries *SummaryService) *ChatService {
	return &ChatService{db: db, docs: docs, ragEngine: ragEngine, llm: llm, quiz: quiz, cards: cards, summaries: summaries}
}

// CreateConversation starts a chat about one of the user's READY documents.
func (s *ChatService) CreateConversation(ctx context.Context, userID, docID, title string) (*models.Conversation, error) {
	if _, err := s.docs.GetReady(ctx, userID, docID); err != nil {
		return nil, err
	}
	if title == "" {
		title = "New Study Session"
	}
	conv := &models.Conversation{
		ID:         uuid.NewString(),
		UserID:     userID,
		DocumentID: docID,
		Title:      title,
	}
	if err := s.db.WithContext(ctx).Create(conv).Error; err != nil {
		return nil, fmt.Errorf("creating conversation: %w", err)
	}
	return conv, nil
}

func (s *ChatService) ListConversations(ctx context.Context, userID string) ([]models.Conversation, error) {
	var convs []models.Conversation
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&convs).Error
	return convs, err
}

// GetConversation returns the conversation with its messages, or ErrNotFound if the user doesn't own it.
func (s *ChatService) GetConversation(ctx context.Context, userID, convID string) (*models.Conversation, error) {
	if _, err := uuid.Parse(convID); err != nil {
		return nil, ErrNotFound
	}
	var conv models.Conversation
	err := s.db.WithContext(ctx).Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC")
	}).First(&conv, "id = ? AND user_id = ?", convID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

func (s *ChatService) DeleteConversation(ctx context.Context, userID, convID string) error {
	if _, err := uuid.Parse(convID); err != nil {
		return ErrNotFound
	}
	res := s.db.WithContext(ctx).Delete(&models.Conversation{}, "id = ? AND user_id = ?", convID, userID)
	if res.Error != nil {
		return fmt.Errorf("deleting conversation: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type SendMessageResult struct {
	UserMessage      *models.Message   `json:"user_message"`
	AssistantMessage *models.Message   `json:"assistant_message"`
	Citations        []models.Citation `json:"citations"`
}

// SendOptions are optional settings for one chat message.
type SendOptions struct {
	Style string // explanation style; overrides one detected from the message
}

// SendMessage handles one chat message: a question is answered from the document (streaming tokens to
// streamWriter when given), and a request for a quiz, flashcards or a summary creates one in the chat.
func (s *ChatService) SendMessage(ctx context.Context, userID, convID, content string, opts SendOptions, streamWriter func(token string) error) (*SendMessageResult, error) {
	if opts.Style != "" {
		if _, ok := rag.ExplanationStyles[opts.Style]; !ok {
			return nil, fmt.Errorf("%w: unknown style %q", ErrInvalidInput, opts.Style)
		}
	}
	conv, err := s.GetConversation(ctx, userID, convID)
	if err != nil {
		return nil, err
	}

	userMsg := &models.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "user",
		Content:        content,
		Type:           "chat",
		CreatedAt:      time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(userMsg).Error; err != nil {
		return nil, fmt.Errorf("saving user message: %w", err)
	}

	if len(conv.Messages) == 0 {
		if err := s.db.WithContext(ctx).Model(conv).Update("title", shortTitle(content)).Error; err != nil {
			return nil, fmt.Errorf("updating conversation title: %w", err)
		}
	}

	history := make([]rag.ChatMessage, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		history = append(history, rag.ChatMessage{Role: m.Role, Content: m.Content})
	}

	r, err := routeMessage(ctx, s.llm, history, content)
	if err != nil {
		// Routing only adds intent detection and query rewriting; answering the message directly still works.
		log.Printf("chat router failed, answering as a question: %v", err)
		r = route{Intent: intentQuestion}
	}
	style := opts.Style
	if style == "" {
		style = r.Style
	}

	reply, err := s.reply(ctx, userID, conv, content, r, style, history, streamWriter)
	if err != nil {
		return nil, err
	}

	citationsJSON, err := json.Marshal(reply.citations)
	if err != nil {
		return nil, err
	}
	assistantMsg := &models.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "assistant",
		Content:        reply.content,
		Type:           reply.msgType,
		CitationsJSON:  string(citationsJSON),
		MetadataJSON:   reply.metadata,
		CreatedAt:      time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(assistantMsg).Error; err != nil {
		return nil, fmt.Errorf("saving assistant message: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(conv).Update("updated_at", time.Now()).Error; err != nil {
		return nil, fmt.Errorf("updating conversation: %w", err)
	}

	return &SendMessageResult{UserMessage: userMsg, AssistantMessage: assistantMsg, Citations: reply.citations}, nil
}

type chatReply struct {
	content   string
	msgType   string // "chat", "quiz", "flashcards", "summary"
	metadata  string // JSON linking the message to the quiz, flashcard set or summary
	citations []models.Citation
}

func (s *ChatService) reply(ctx context.Context, userID string, conv *models.Conversation, content string, r route, style string, history []rag.ChatMessage, streamWriter func(string) error) (*chatReply, error) {
	// Study items arrive all at once; stream their intro text as a single token so the UI stays uniform.
	emit := func(text string) error {
		if streamWriter == nil {
			return nil
		}
		return streamWriter(text)
	}

	switch r.Intent {
	case intentQuiz:
		quiz, err := s.quiz.GenerateQuiz(ctx, userID, GenerateQuizRequest{
			DocumentID: conv.DocumentID, ConversationID: conv.ID, Topic: r.Topic,
			Difficulty: r.Difficulty, NumQuestions: r.NumQuestions, QuestionTypes: r.QuestionTypes,
		})
		if err != nil {
			return nil, err
		}
		text := fmt.Sprintf("Let's test your understanding of %s. %d questions, one at a time, and you'll get feedback after each answer.",
			subject(r.Topic), quiz.TotalQuestions)
		return &chatReply{content: text, msgType: "quiz", metadata: metadataJSON("quiz_session_id", quiz.SessionID), citations: []models.Citation{}}, emit(text)

	case intentFlashcards:
		set, err := s.cards.GenerateFlashcards(ctx, userID, GenerateFlashcardsRequest{
			DocumentID: conv.DocumentID, ConversationID: conv.ID, Topic: r.Topic, Count: r.Count,
		})
		if err != nil {
			return nil, err
		}
		text := fmt.Sprintf("Here are %d flashcards on %s. Flip each card, then mark it Known or Need Practice.", len(set.Cards), subject(r.Topic))
		return &chatReply{content: text, msgType: "flashcards", metadata: metadataJSON("flashcard_set_id", set.ID), citations: []models.Citation{}}, emit(text)

	case intentSummary:
		sum, err := s.summaries.GenerateSummary(ctx, userID, GenerateSummaryRequest{DocumentID: conv.DocumentID, Type: r.SummaryType, Topic: r.Topic})
		if err != nil {
			return nil, err
		}
		meta, err := json.Marshal(map[string]string{"summary_type": sum.Type, "topic": sum.Topic})
		if err != nil {
			return nil, err
		}
		return &chatReply{content: sum.Summary, msgType: "summary", metadata: string(meta), citations: sum.Citations}, emit(sum.Summary)
	}

	params := rag.AskParams{
		UserID: userID, DocumentID: conv.DocumentID, Question: content,
		SearchQuery: r.SearchQuery, Style: style, History: history,
	}
	var result *rag.RAGResult
	var err error
	if streamWriter != nil {
		result, err = s.ragEngine.StreamQuery(ctx, params, streamWriter)
	} else {
		result, err = s.ragEngine.Query(ctx, params)
	}
	if err != nil {
		return nil, fmt.Errorf("generating answer: %w", err)
	}
	return &chatReply{content: result.Answer, msgType: "chat", citations: result.Citations}, nil
}

func subject(topic string) string {
	if topic == "" {
		return "this document"
	}
	return topic
}

func metadataJSON(key, value string) string {
	b, _ := json.Marshal(map[string]string{key: value}) // a map of strings always marshals
	return string(b)
}

func shortTitle(s string) string {
	if utf8.RuneCountInString(s) <= 40 {
		return s
	}
	return string([]rune(s)[:40]) + "..."
}
