package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
}

func NewChatService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine) *ChatService {
	return &ChatService{db: db, docs: docs, ragEngine: ragEngine}
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

// SendMessage answers a question in a conversation, streaming tokens to streamWriter when given.
func (s *ChatService) SendMessage(ctx context.Context, userID, convID, content string, streamWriter func(token string) error) (*SendMessageResult, error) {
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

	var result *rag.RAGResult
	if streamWriter != nil {
		result, err = s.ragEngine.StreamQuery(ctx, content, conv.DocumentID, userID, history, streamWriter)
	} else {
		result, err = s.ragEngine.Query(ctx, content, conv.DocumentID, userID, history)
	}
	if err != nil {
		return nil, fmt.Errorf("generating answer: %w", err)
	}

	citationsJSON, err := json.Marshal(result.Citations)
	if err != nil {
		return nil, err
	}
	assistantMsg := &models.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "assistant",
		Content:        result.Answer,
		Type:           "chat",
		CitationsJSON:  string(citationsJSON),
		CreatedAt:      time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(assistantMsg).Error; err != nil {
		return nil, fmt.Errorf("saving assistant message: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(conv).Update("updated_at", time.Now()).Error; err != nil {
		return nil, fmt.Errorf("updating conversation: %w", err)
	}

	return &SendMessageResult{UserMessage: userMsg, AssistantMessage: assistantMsg, Citations: result.Citations}, nil
}

func shortTitle(s string) string {
	if utf8.RuneCountInString(s) <= 40 {
		return s
	}
	return string([]rune(s)[:40]) + "..."
}
