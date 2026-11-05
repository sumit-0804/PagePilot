package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FeedbackReasons are the optional reasons a user can give with a thumbs down (or up).
var FeedbackReasons = map[string]bool{"incorrect": true, "not_grounded": true, "unhelpful": true, "unclear": true, "other": true}

// FeedbackScorer mirrors ratings to an evaluation tool (Langfuse). Supabase stays the source of truth.
type FeedbackScorer interface {
	Score(ctx context.Context, traceID, messageID string, rating int, reason string) error
	Remove(ctx context.Context, messageID string) error
}

// FeedbackService stores thumbs up/down on tutor answers.
type FeedbackService struct {
	db     *gorm.DB
	scorer FeedbackScorer // nil when Langfuse is not configured
}

func NewFeedbackService(db *gorm.DB, scorer FeedbackScorer) *FeedbackService {
	return &FeedbackService{db: db, scorer: scorer}
}

// ownedAnswer finds an assistant message in one of the user's conversations.
func (s *FeedbackService) ownedAnswer(ctx context.Context, userID, messageID string) (*models.Message, error) {
	if _, err := uuid.Parse(messageID); err != nil {
		return nil, ErrNotFound
	}
	var msg models.Message
	err := s.db.WithContext(ctx).
		Joins("join conversations c on c.id = messages.conversation_id").
		Where("messages.id = ? AND c.user_id = ?", messageID, userID).
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding message: %w", err)
	}
	if msg.Role != "assistant" {
		return nil, fmt.Errorf("%w: only answers can be rated", ErrInvalidInput)
	}
	return &msg, nil
}

// Rate records (or changes) the user's rating of an answer.
func (s *FeedbackService) Rate(ctx context.Context, userID, messageID string, fb models.Feedback) (*models.Feedback, error) {
	if fb.Rating != 1 && fb.Rating != -1 {
		return nil, fmt.Errorf("%w: rating must be 1 or -1", ErrInvalidInput)
	}
	if fb.Reason != "" && !FeedbackReasons[fb.Reason] {
		return nil, fmt.Errorf("%w: unknown reason %q", ErrInvalidInput, fb.Reason)
	}
	msg, err := s.ownedAnswer(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}

	row := models.MessageFeedback{MessageID: msg.ID, Rating: fb.Rating, UpdatedAt: time.Now()}
	if fb.Reason != "" {
		row.Reason = &fb.Reason
	}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rating", "reason", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return nil, fmt.Errorf("saving feedback: %w", err)
	}

	if s.scorer != nil && msg.TraceID != nil {
		s.mirror(ctx, func(ctx context.Context) error {
			return s.scorer.Score(ctx, *msg.TraceID, msg.ID, fb.Rating, fb.Reason)
		})
	}
	return &fb, nil
}

// Clear removes the user's rating of an answer.
func (s *FeedbackService) Clear(ctx context.Context, userID, messageID string) error {
	msg, err := s.ownedAnswer(ctx, userID, messageID)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&models.MessageFeedback{}, "message_id = ?", msg.ID).Error; err != nil {
		return fmt.Errorf("deleting feedback: %w", err)
	}
	if s.scorer != nil && msg.TraceID != nil {
		s.mirror(ctx, func(ctx context.Context) error { return s.scorer.Remove(ctx, msg.ID) })
	}
	return nil
}

// mirror updates Langfuse in the background so a slow or failing Langfuse never delays the click.
func (s *FeedbackService) mirror(ctx context.Context, call func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	go func() {
		defer cancel()
		if err := call(ctx); err != nil {
			log.Printf("feedback: updating Langfuse score: %v", err)
		}
	}()
}

// attachFeedback fills in each message's rating with one query.
func attachFeedback(ctx context.Context, db *gorm.DB, messages []models.Message) error {
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.Role == "assistant" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []models.MessageFeedback
	if err := db.WithContext(ctx).Where("message_id IN ?", ids).Find(&rows).Error; err != nil {
		return fmt.Errorf("loading feedback: %w", err)
	}
	byID := make(map[string]*models.Feedback, len(rows))
	for _, r := range rows {
		fb := &models.Feedback{Rating: r.Rating}
		if r.Reason != nil {
			fb.Reason = *r.Reason
		}
		byID[r.MessageID] = fb
	}
	for i := range messages {
		messages[i].Feedback = byID[messages[i].ID]
	}
	return nil
}
