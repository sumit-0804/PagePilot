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
	"github.com/pagepilot/backend/internal/observability"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
)

// ErrLoginRequired is returned when a guest uses up the free trial or calls an account-only feature.
var ErrLoginRequired = errors.New("login_required")

// GuestMessageLimit is how many messages a visitor can send before they must sign in.
const GuestMessageLimit = 1

// MaxAttachedDocuments bounds how many documents one conversation can search.
const MaxAttachedDocuments = 10

// ChatService stores conversations and messages; a Responder produces each reply.
type ChatService struct {
	db        *gorm.DB
	docs      *DocumentService
	responder Responder
	llm       rag.LLMClient // suggests follow-ups under answers
}

func NewChatService(db *gorm.DB, docs *DocumentService, responder Responder, llm rag.LLMClient) *ChatService {
	return &ChatService{db: db, docs: docs, responder: responder, llm: llm}
}

// CreateConversation starts a chat, optionally attached to some of the user's READY documents.
func (s *ChatService) CreateConversation(ctx context.Context, userID string, docIDs []string, title string) (*models.Conversation, error) {
	docIDs = uniqueIDs(docIDs)
	if len(docIDs) > MaxAttachedDocuments {
		return nil, fmt.Errorf("%w: at most %d documents per chat", ErrInvalidInput, MaxAttachedDocuments)
	}
	docs := make([]models.Document, 0, len(docIDs))
	for _, id := range docIDs {
		doc, err := s.docs.GetReady(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		docs = append(docs, *doc)
	}
	if title == "" {
		title = "New chat"
	}
	conv := &models.Conversation{ID: uuid.NewString(), UserID: userID, Title: title}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(conv).Error; err != nil {
			return err
		}
		for _, d := range docs {
			link := models.ConversationDocument{ConversationID: conv.ID, DocumentID: d.ID, UserID: userID}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating conversation: %w", err)
	}
	conv.Documents = docs
	return conv, nil
}

// AttachDocument adds one of the user's READY documents to a conversation they own.
func (s *ChatService) AttachDocument(ctx context.Context, userID, convID, docID string) (*models.Conversation, error) {
	conv, err := s.GetConversation(ctx, userID, convID)
	if err != nil {
		return nil, err
	}
	if _, err := s.docs.GetReady(ctx, userID, docID); err != nil {
		return nil, err
	}
	for _, d := range conv.Documents {
		if d.ID == docID {
			return conv, nil
		}
	}
	if len(conv.Documents) >= MaxAttachedDocuments {
		return nil, fmt.Errorf("%w: at most %d documents per chat", ErrInvalidInput, MaxAttachedDocuments)
	}
	link := models.ConversationDocument{ConversationID: convID, DocumentID: docID, UserID: userID}
	if err := s.db.WithContext(ctx).Create(&link).Error; err != nil {
		return nil, fmt.Errorf("attaching document: %w", err)
	}
	return s.GetConversation(ctx, userID, convID)
}

// DetachDocument removes a document from a conversation; the document itself is kept.
func (s *ChatService) DetachDocument(ctx context.Context, userID, convID, docID string) (*models.Conversation, error) {
	if _, err := uuid.Parse(docID); err != nil {
		return nil, ErrNotFound
	}
	if _, err := s.GetConversation(ctx, userID, convID); err != nil {
		return nil, err
	}
	res := s.db.WithContext(ctx).Delete(&models.ConversationDocument{}, "conversation_id = ? AND document_id = ? AND user_id = ?", convID, docID, userID)
	if res.Error != nil {
		return nil, fmt.Errorf("detaching document: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.GetConversation(ctx, userID, convID)
}

func (s *ChatService) ListConversations(ctx context.Context, userID string) ([]models.Conversation, error) {
	var convs []models.Conversation
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&convs).Error
	return convs, err
}

// GetConversation returns the conversation with its messages and attached documents,
// or ErrNotFound if the user doesn't own it.
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
	conv.Documents = []models.Document{}
	err = s.db.WithContext(ctx).
		Joins("join conversation_documents cd on cd.document_id = documents.id").
		Where("cd.conversation_id = ? AND documents.user_id = ?", convID, userID).
		Order("cd.created_at").Find(&conv.Documents).Error
	if err != nil {
		return nil, fmt.Errorf("loading attached documents: %w", err)
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

// ClaimGuestData moves everything a guest made to the account they signed in to.
func (s *ChatService) ClaimGuestData(ctx context.Context, guestID, userID string) error {
	if guestID == userID {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"conversations", "quiz_sessions", "quiz_answers", "flashcard_sets"} {
			if err := tx.Table(table).Where("user_id = ?", guestID).Update("user_id", userID).Error; err != nil {
				return fmt.Errorf("claiming %s: %w", table, err)
			}
		}
		return nil
	})
}

// CheckGuestAllowance returns ErrLoginRequired once a guest has used their free messages.
func (s *ChatService) CheckGuestAllowance(ctx context.Context, userID string) error {
	var sent int64
	err := s.db.WithContext(ctx).Model(&models.Message{}).
		Joins("join conversations c on c.id = messages.conversation_id").
		Where("c.user_id = ? AND messages.role = 'user'", userID).Count(&sent).Error
	if err != nil {
		return fmt.Errorf("counting guest messages: %w", err)
	}
	if sent >= GuestMessageLimit {
		return ErrLoginRequired
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
	Style    string                    // explanation style; overrides one detected from the message
	Guest    bool                      // the caller is an anonymous visitor on the free trial
	OnStatus func(status string) error // progress updates such as "Searching the web"
}

// SendMessage handles one chat message: the responder answers it (streaming tokens to streamWriter
// when given), searching attached documents or the web, or creates a quiz, flashcards or a summary.
func (s *ChatService) SendMessage(ctx context.Context, userID, convID, content string, opts SendOptions, streamWriter func(token string) error) (_ *SendMessageResult, err error) {
	userKind := "account"
	if opts.Guest {
		userKind = "guest"
	}
	ctx = observability.WithTraceAttributes(ctx,
		observability.AttrTraceName.String("chat"),
		observability.AttrTraceTags.StringSlice([]string{"chat", userKind}),
		observability.AttrSessionID.String(convID),
		observability.AttrUserID.String(observability.UserRef(userID)),
	)
	ctx, span := observability.Start(ctx, "chat.turn", observability.TypeAgent, observability.AttrGuest.Bool(opts.Guest))
	defer func() { observability.End(span, err) }()
	observability.SetContent(span, observability.AttrInput, content)

	if opts.Style != "" {
		if _, ok := rag.ExplanationStyles[opts.Style]; !ok {
			return nil, fmt.Errorf("%w: unknown style %q", ErrInvalidInput, opts.Style)
		}
	}
	conv, err := s.GetConversation(ctx, userID, convID)
	if err != nil {
		return nil, err
	}
	if opts.Guest {
		if err := s.CheckGuestAllowance(ctx, userID); err != nil {
			return nil, err
		}
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

	reply, err := s.responder.Respond(ctx, ReplyRequest{
		UserID: userID, Guest: opts.Guest, Conversation: conv, Message: content, Style: opts.Style,
		History: history, OnStatus: opts.OnStatus,
	}, streamWriter)
	if err != nil {
		return nil, err
	}
	if reply.Citations == nil {
		reply.Citations = []models.Citation{}
	}
	span.SetAttributes(
		observability.AttrDocumentCount.Int(len(conv.Documents)),
		observability.AttrReplyType.String(reply.Type),
		observability.AttrCitationCount.Int(len(reply.Citations)),
	)
	observability.SetContent(span, observability.AttrOutput, reply.Content)
	if reply.Type == "chat" && !opts.Guest && reply.Metadata == "" {
		// Follow-ups are a nice-to-have: on failure the UI falls back to its generic chips.
		if followUps, err := suggestFollowUps(ctx, s.llm, content, reply.Content); err != nil {
			log.Printf("suggesting follow-ups: %v", err)
		} else if len(followUps) > 0 {
			meta, _ := json.Marshal(map[string]any{"follow_ups": followUps}) // plain structs always marshal
			reply.Metadata = string(meta)
		}
	}

	citationsJSON, err := json.Marshal(reply.Citations)
	if err != nil {
		return nil, err
	}
	assistantMsg := &models.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "assistant",
		Content:        reply.Content,
		Type:           reply.Type,
		CitationsJSON:  string(citationsJSON),
		MetadataJSON:   reply.Metadata,
		CreatedAt:      time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(assistantMsg).Error; err != nil {
		return nil, fmt.Errorf("saving assistant message: %w", err)
	}
	if err := s.db.WithContext(ctx).Model(conv).Update("updated_at", time.Now()).Error; err != nil {
		return nil, fmt.Errorf("updating conversation: %w", err)
	}

	return &SendMessageResult{UserMessage: userMsg, AssistantMessage: assistantMsg, Citations: reply.Citations}, nil
}

func shortTitle(s string) string {
	if utf8.RuneCountInString(s) <= 40 {
		return s
	}
	return string([]rune(s)[:40]) + "..."
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
