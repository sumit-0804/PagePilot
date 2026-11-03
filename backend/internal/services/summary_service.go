package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/vectorstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultSectionChars = 12000 // ~3k tokens per LLM call
	maxSections         = 20    // bounds the number of LLM calls for very long documents
)

type SummaryService struct {
	db           *gorm.DB
	docs         *DocumentService
	ragEngine    *rag.Engine
	llmClient    rag.LLMClient
	sectionChars int
}

func NewSummaryService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine, llm rag.LLMClient) *SummaryService {
	return &SummaryService{db: db, docs: docs, ragEngine: ragEngine, llmClient: llm, sectionChars: defaultSectionChars}
}

type GenerateSummaryRequest struct {
	DocumentID string `json:"document_id"`
	Type       string `json:"type"`    // see summaryInstructions
	Topic      string `json:"topic"`   // optional focus
	Refresh    bool   `json:"refresh"` // regenerate instead of returning the cached summary
}

type SummaryResponse struct {
	DocumentID string            `json:"document_id"`
	Type       string            `json:"type"`
	Topic      string            `json:"topic"`
	Summary    string            `json:"summary"`
	Citations  []models.Citation `json:"citations"`
	Cached     bool              `json:"cached"`
}

var summaryInstructions = map[string]string{
	"complete":    "Write a comprehensive, structured summary with the core concepts, bullet points and key takeaways.",
	"chapter":     "Write a chapter-by-chapter (section-by-section) breakdown with the main ideas of each part.",
	"revision":    "Write concise, high-yield revision notes for quick pre-exam review.",
	"detailed":    "Write detailed study notes that keep explanations, examples and reasoning.",
	"definitions": "Write a glossary of all key terms, definitions and acronyms.",
	"concepts":    "List and briefly explain the key concepts, from most to least important.",
	"formulas":    "List and explain all formulas, equations and quantitative relationships, using KaTeX notation.",
	"exam":        "Write exam preparation notes: must-know points, common mistakes and likely exam questions with short answers.",
}

func (s *SummaryService) GenerateSummary(ctx context.Context, userID string, req GenerateSummaryRequest) (*SummaryResponse, error) {
	if _, err := s.docs.GetReady(ctx, userID, req.DocumentID); err != nil {
		return nil, err
	}
	if _, ok := summaryInstructions[req.Type]; !ok {
		req.Type = "complete"
	}
	req.Topic = strings.TrimSpace(req.Topic)

	if !req.Refresh {
		cached, err := s.cached(ctx, userID, req)
		if err != nil || cached != nil {
			return cached, err
		}
	}

	var chunks []vectorstore.SearchResult
	var err error
	if req.Topic != "" {
		chunks, err = s.ragEngine.Retrieve(ctx, req.Topic, userID, req.DocumentID, 25)
	} else {
		chunks, err = s.allChunks(ctx, userID, req.DocumentID)
	}
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, errors.New("no content found to summarize")
	}

	var summary string
	if totalChars(chunks) <= s.sectionChars {
		summary, err = s.summarize(ctx, req, formatExcerpts(chunks))
	} else {
		summary, err = s.summarizeInSections(ctx, req, chunks)
	}
	if err != nil {
		return nil, err
	}

	summary = rag.NormalizeCitations(summary)
	citations := rag.FilterCitations(summary, rag.CitationsFor(chunks))
	if err := s.store(ctx, userID, req, summary, citations); err != nil {
		return nil, err
	}
	return &SummaryResponse{DocumentID: req.DocumentID, Type: req.Type, Topic: req.Topic, Summary: summary, Citations: citations}, nil
}

func (s *SummaryService) summarize(ctx context.Context, req GenerateSummaryRequest, excerpts string) (string, error) {
	prompt := fmt.Sprintf(`%s%s
Ground the summary STRICTLY in the document excerpts below.
Cite the page of every main point in square brackets, exactly like [Page X], using only page numbers shown below.

EXCERPTS:
%s`, summaryInstructions[req.Type], topicLine(req.Topic), excerpts)
	return s.generate(ctx, prompt)
}

// summarizeInSections summarizes consecutive parts of a long document, then combines the parts,
// so the whole document is covered without sending it to the LLM in one request.
func (s *SummaryService) summarizeInSections(ctx context.Context, req GenerateSummaryRequest, chunks []vectorstore.SearchResult) (string, error) {
	size := max(s.sectionChars, totalChars(chunks)/maxSections+1)
	sections := splitSections(chunks, size)

	var notes strings.Builder
	for i, section := range sections {
		first, last := section[0].PageNumber, section[len(section)-1].PageNumber
		prompt := fmt.Sprintf(`Summarize this part of the document (pages %d-%d) as study notes.
Keep every important concept, definition, formula and example.
Cite the page of every point in square brackets, exactly like [Page X], using only page numbers shown below.

EXCERPTS:
%s`, first, last, formatExcerpts(section))
		part, err := s.generate(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("summarizing section %d of %d: %w", i+1, len(sections), err)
		}
		fmt.Fprintf(&notes, "--- SECTION %d (pages %d-%d) ---\n%s\n\n", i+1, first, last, rag.NormalizeCitations(part))
	}

	prompt := fmt.Sprintf(`%s%s
Below are notes covering the whole document, section by section. Combine them into one result.
Keep the [Page X] citations from the notes; never add a page number that is not in the notes.

SECTION NOTES:
%s`, summaryInstructions[req.Type], topicLine(req.Topic), notes.String())
	return s.generate(ctx, prompt)
}

func (s *SummaryService) generate(ctx context.Context, prompt string) (string, error) {
	out, err := s.llmClient.Generate(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are an academic summarizer. Write structured Markdown grounded in page citations."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("summary generation failed: %w", err)
	}
	return out, nil
}

func (s *SummaryService) allChunks(ctx context.Context, userID, docID string) ([]vectorstore.SearchResult, error) {
	var chunks []models.DocumentChunk
	err := s.db.WithContext(ctx).
		Select("id", "document_id", "document_name", "page_number", "chunk_index", "chunk_text").
		Where("user_id = ? AND document_id = ?", userID, docID).
		Order("chunk_index").Find(&chunks).Error
	if err != nil {
		return nil, fmt.Errorf("loading chunks: %w", err)
	}
	results := make([]vectorstore.SearchResult, len(chunks))
	for i, c := range chunks {
		results[i] = vectorstore.SearchResult{
			ChunkID: c.ID, DocumentID: c.DocumentID, DocumentName: c.DocumentName,
			PageNumber: c.PageNumber, ChunkIndex: c.ChunkIndex, ChunkText: c.ChunkText,
		}
	}
	return results, nil
}

func (s *SummaryService) cached(ctx context.Context, userID string, req GenerateSummaryRequest) (*SummaryResponse, error) {
	var row models.Summary
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND document_id = ? AND type = ? AND topic = ?", userID, req.DocumentID, req.Type, req.Topic).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cached summary: %w", err)
	}
	var citations []models.Citation
	if row.CitationsJSON != "" {
		if err := json.Unmarshal([]byte(row.CitationsJSON), &citations); err != nil {
			return nil, fmt.Errorf("reading cached citations: %w", err)
		}
	}
	return &SummaryResponse{DocumentID: row.DocumentID, Type: row.Type, Topic: row.Topic, Summary: row.Content, Citations: citations, Cached: true}, nil
}

func (s *SummaryService) store(ctx context.Context, userID string, req GenerateSummaryRequest, content string, citations []models.Citation) error {
	citationsJSON, err := json.Marshal(citations)
	if err != nil {
		return err
	}
	row := models.Summary{
		ID: uuid.NewString(), UserID: userID, DocumentID: req.DocumentID, Type: req.Type, Topic: req.Topic,
		Content: content, CitationsJSON: string(citationsJSON), UpdatedAt: time.Now(),
	}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "document_id"}, {Name: "type"}, {Name: "topic"}},
		DoUpdates: clause.AssignmentColumns([]string{"content", "citations_json", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("caching summary: %w", err)
	}
	return nil
}

func splitSections(chunks []vectorstore.SearchResult, size int) [][]vectorstore.SearchResult {
	var sections [][]vectorstore.SearchResult
	var current []vectorstore.SearchResult
	chars := 0
	for _, c := range chunks {
		if chars+len(c.ChunkText) > size && len(current) > 0 {
			sections = append(sections, current)
			current, chars = nil, 0
		}
		current = append(current, c)
		chars += len(c.ChunkText)
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

func totalChars(chunks []vectorstore.SearchResult) int {
	n := 0
	for _, c := range chunks {
		n += len(c.ChunkText)
	}
	return n
}

func topicLine(topic string) string {
	if topic == "" {
		return ""
	}
	return "\nFocus on this topic: " + topic
}
