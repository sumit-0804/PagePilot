package vectorstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

type SearchResult struct {
	ChunkID      string  `json:"chunk_id"`
	DocumentID   string  `json:"document_id"`
	DocumentName string  `json:"document_name"`
	PageNumber   int     `json:"page_number"`
	ChunkIndex   int     `json:"chunk_index"`
	ChunkText    string  `json:"chunk_text"`
	Score        float32 `json:"score"`
}

// VectorStore saves embedded chunks and searches within some of a single user's documents.
type VectorStore interface {
	// ReplaceChunks atomically replaces all chunks of a document; each chunk carries its embedding.
	ReplaceChunks(ctx context.Context, documentID string, chunks []models.DocumentChunk) error
	Search(ctx context.Context, queryEmbedding []float32, userID string, documentIDs []string, limit int) ([]SearchResult, error)
}

var ErrScopeRequired = errors.New("vector search requires a user_id and at least one document_id")

// PgVectorStore keeps vectors in document_chunks.embedding (pgvector).
type PgVectorStore struct {
	db *gorm.DB
}

func NewPgVectorStore(db *gorm.DB) *PgVectorStore {
	return &PgVectorStore{db: db}
}

func (p *PgVectorStore) ReplaceChunks(ctx context.Context, documentID string, chunks []models.DocumentChunk) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ?", documentID).Delete(&models.DocumentChunk{}).Error; err != nil {
			return fmt.Errorf("clearing old chunks: %w", err)
		}
		if len(chunks) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(chunks, 100).Error; err != nil {
			return fmt.Errorf("inserting chunks: %w", err)
		}
		return nil
	})
}

func (p *PgVectorStore) Search(ctx context.Context, queryEmbedding []float32, userID string, documentIDs []string, limit int) ([]SearchResult, error) {
	if userID == "" || len(documentIDs) == 0 {
		return nil, ErrScopeRequired
	}
	if limit <= 0 {
		limit = 5
	}
	vec := pgvector.NewVector(queryEmbedding)
	var results []SearchResult
	err := p.db.WithContext(ctx).Raw(`
		select id as chunk_id, document_id, document_name, page_number, chunk_index, chunk_text,
		       1 - (embedding <=> ?) as score
		from document_chunks
		where user_id = ? and document_id in ?
		order by embedding <=> ?
		limit ?`, vec, userID, documentIDs, vec, limit).Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	return results, nil
}

// MemoryVectorStore is an in-memory implementation for tests only.
type MemoryVectorStore struct {
	mu     sync.RWMutex
	chunks []models.DocumentChunk
}

func NewMemoryVectorStore() *MemoryVectorStore {
	return &MemoryVectorStore{}
}

func (m *MemoryVectorStore) ReplaceChunks(_ context.Context, documentID string, chunks []models.DocumentChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.chunks[:0]
	for _, c := range m.chunks {
		if c.DocumentID != documentID {
			kept = append(kept, c)
		}
	}
	m.chunks = append(kept, chunks...)
	return nil
}

func (m *MemoryVectorStore) Search(_ context.Context, queryEmbedding []float32, userID string, documentIDs []string, limit int) ([]SearchResult, error) {
	if userID == "" || len(documentIDs) == 0 {
		return nil, ErrScopeRequired
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []SearchResult
	for _, c := range m.chunks {
		if c.UserID != userID || !slices.Contains(documentIDs, c.DocumentID) {
			continue
		}
		results = append(results, SearchResult{
			ChunkID:      c.ID,
			DocumentID:   c.DocumentID,
			DocumentName: c.DocumentName,
			PageNumber:   c.PageNumber,
			ChunkIndex:   c.ChunkIndex,
			ChunkText:    c.ChunkText,
			Score:        cosineSimilarity(queryEmbedding, c.Embedding.Slice()),
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
