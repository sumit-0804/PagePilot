package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
)

type FlashcardService struct {
	db        *gorm.DB
	docs      *DocumentService
	ragEngine *rag.Engine
	llmClient rag.LLMClient
}

func NewFlashcardService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine, llm rag.LLMClient) *FlashcardService {
	return &FlashcardService{db: db, docs: docs, ragEngine: ragEngine, llmClient: llm}
}

type GenerateFlashcardsRequest struct {
	DocumentID     string `json:"document_id"`
	ConversationID string `json:"conversation_id"`
	Topic          string `json:"topic"`
	Count          int    `json:"count"`
}

var validCardStatuses = map[string]bool{"new": true, "known": true, "practice": true}

func (s *FlashcardService) GenerateFlashcards(ctx context.Context, userID string, req GenerateFlashcardsRequest) (*models.FlashcardSet, error) {
	if _, err := s.docs.GetReady(ctx, userID, req.DocumentID); err != nil {
		return nil, err
	}
	if req.Count <= 0 || req.Count > 30 {
		req.Count = 6
	}
	convID, err := ownedConversationID(ctx, s.db, userID, req.DocumentID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	excerpts, err := loadExcerpts(ctx, s.db, s.ragEngine, userID, req.DocumentID, req.Topic, 15)
	if err != nil {
		return nil, err
	}
	pages := excerptPages(excerpts)

	prompt := fmt.Sprintf(`Create %d high-yield active-recall study flashcards grounded in these document excerpts.
Topic: %s

RULES:
1. Front must be a clear conceptual question, term, or prompt.
2. Back must be a concise, precise explanation or definition taken from the excerpts.
3. citation_page must be the page number shown in the excerpt the card comes from.
4. Output MUST be a valid JSON array:
[
  {
    "front": "What is...",
    "back": "It is...",
    "citation_page": 2
  }
]

EXCERPTS:
%s`, req.Count, req.Topic, formatExcerpts(excerpts))

	llmResp, err := s.llmClient.Generate(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are an expert study flashcard writer. Output strictly a JSON array."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("flashcard generation failed: %w", err)
	}

	var generated []struct {
		Front        string `json:"front"`
		Back         string `json:"back"`
		CitationPage int    `json:"citation_page"`
	}
	if err := json.Unmarshal([]byte(extractJSON(llmResp)), &generated); err != nil {
		return nil, fmt.Errorf("the AI returned flashcards in an unreadable format, please try again: %w", err)
	}

	title := req.Topic
	if strings.TrimSpace(title) == "" {
		title = "Document Revision Cards"
	}
	set := &models.FlashcardSet{
		ID:             uuid.NewString(),
		UserID:         userID,
		DocumentID:     req.DocumentID,
		ConversationID: convID,
		Title:          title,
		Topic:          req.Topic,
	}
	for _, g := range generated {
		if strings.TrimSpace(g.Front) == "" || strings.TrimSpace(g.Back) == "" {
			continue
		}
		set.Cards = append(set.Cards, models.Flashcard{
			ID:           uuid.NewString(),
			SetID:        set.ID,
			Front:        g.Front,
			Back:         g.Back,
			CitationPage: verifiedPage(g.CitationPage, pages),
			Status:       "new",
		})
	}
	if len(set.Cards) == 0 {
		return nil, errors.New("the AI did not return any usable flashcards, please try again")
	}

	// Create with associations inserts the set and its cards in one transaction.
	if err := s.db.WithContext(ctx).Create(set).Error; err != nil {
		return nil, fmt.Errorf("saving flashcards: %w", err)
	}
	return set, nil
}

func (s *FlashcardService) ListFlashcardSets(ctx context.Context, userID, docID string) ([]models.FlashcardSet, error) {
	query := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if docID != "" {
		if _, err := uuid.Parse(docID); err != nil {
			return nil, ErrNotFound
		}
		query = query.Where("document_id = ?", docID)
	}
	var sets []models.FlashcardSet
	err := query.Preload("Cards", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Order("created_at DESC").Find(&sets).Error
	return sets, err
}

func (s *FlashcardService) GetFlashcardSet(ctx context.Context, userID, setID string) (*models.FlashcardSet, error) {
	if _, err := uuid.Parse(setID); err != nil {
		return nil, ErrNotFound
	}
	var set models.FlashcardSet
	err := s.db.WithContext(ctx).Preload("Cards", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		First(&set, "id = ? AND user_id = ?", setID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &set, nil
}

// UpdateCardStatus changes a card's study status, only for cards in the user's own sets.
func (s *FlashcardService) UpdateCardStatus(ctx context.Context, userID, cardID, status string) error {
	if !validCardStatuses[status] {
		return fmt.Errorf("%w: status must be new, known or practice", ErrInvalidInput)
	}
	if _, err := uuid.Parse(cardID); err != nil {
		return ErrNotFound
	}
	res := s.db.WithContext(ctx).Model(&models.Flashcard{}).
		Where("id = ? AND set_id IN (?)", cardID, s.db.Model(&models.FlashcardSet{}).Select("id").Where("user_id = ?", userID)).
		Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("updating card: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
