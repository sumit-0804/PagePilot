package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/vectorstore"
	"gorm.io/gorm"
)

// loadExcerpts picks the chunks a quiz, flashcard set or summary is grounded in: the most relevant
// chunks for a topic, or an even spread across the whole document when there is no topic.
func loadExcerpts(ctx context.Context, db *gorm.DB, engine *rag.Engine, userID, docID, topic string, limit int) ([]vectorstore.SearchResult, error) {
	if strings.TrimSpace(topic) != "" {
		results, err := engine.Retrieve(ctx, topic, userID, docID, limit)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no content found for topic %q", topic)
		}
		return results, nil
	}

	var total int64
	if err := db.WithContext(ctx).Model(&models.DocumentChunk{}).
		Where("user_id = ? AND document_id = ?", userID, docID).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("counting chunks: %w", err)
	}
	if total == 0 {
		return nil, fmt.Errorf("document has no content")
	}
	step := max(1, int(total)/limit)

	var chunks []models.DocumentChunk
	if err := db.WithContext(ctx).
		Select("id", "document_id", "document_name", "page_number", "chunk_index", "chunk_text").
		Where("user_id = ? AND document_id = ? AND chunk_index % ? = 0", userID, docID, step).
		Order("chunk_index").Limit(limit).Find(&chunks).Error; err != nil {
		return nil, fmt.Errorf("loading chunks: %w", err)
	}

	results := make([]vectorstore.SearchResult, len(chunks))
	for i, c := range chunks {
		results[i] = vectorstore.SearchResult{
			ChunkID:      c.ID,
			DocumentID:   c.DocumentID,
			DocumentName: c.DocumentName,
			PageNumber:   c.PageNumber,
			ChunkIndex:   c.ChunkIndex,
			ChunkText:    c.ChunkText,
		}
	}
	return results, nil
}

// ownedConversationID validates an optional conversation link: it must be the user's and about the same document.
func ownedConversationID(ctx context.Context, db *gorm.DB, userID, docID, convID string) (*string, error) {
	if convID == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(convID); err != nil {
		return nil, ErrNotFound
	}
	var count int64
	if err := db.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ? AND user_id = ? AND document_id = ?", convID, userID, docID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNotFound
	}
	return &convID, nil
}

func formatExcerpts(results []vectorstore.SearchResult) string {
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "[Page %d]: %s\n\n", r.PageNumber, r.ChunkText)
	}
	return b.String()
}

func excerptPages(results []vectorstore.SearchResult) map[int]bool {
	pages := make(map[int]bool, len(results))
	for _, r := range results {
		pages[r.PageNumber] = true
	}
	return pages
}

// verifiedPage keeps an LLM-reported page only if it was among the excerpts sent to the LLM.
func verifiedPage(page int, allowed map[int]bool) *int {
	if !allowed[page] {
		return nil
	}
	return &page
}
