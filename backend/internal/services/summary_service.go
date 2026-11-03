package services

import (
	"context"
	"fmt"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
)

type SummaryService struct {
	db        *gorm.DB
	docs      *DocumentService
	ragEngine *rag.Engine
	llmClient rag.LLMClient
}

func NewSummaryService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine, llm rag.LLMClient) *SummaryService {
	return &SummaryService{db: db, docs: docs, ragEngine: ragEngine, llmClient: llm}
}

type GenerateSummaryRequest struct {
	DocumentID string `json:"document_id"`
	Type       string `json:"type"`  // "complete", "chapter", "revision", "formulas", "definitions"
	Topic      string `json:"topic"` // optional focus
}

type SummaryResponse struct {
	DocumentID string            `json:"document_id"`
	Type       string            `json:"type"`
	Summary    string            `json:"summary"`
	Citations  []models.Citation `json:"citations"`
}

var summaryInstructions = map[string]string{
	"complete":    "Generate a comprehensive, structured academic summary with core concepts, bullet points, and key takeaways.",
	"chapter":     "Provide a section-by-section / chapter-wise breakdown with main ideas and page references.",
	"revision":    "Provide concise, high-yield revision bullet points for rapid pre-exam review.",
	"formulas":    "Extract and explain all formulas, mathematical definitions, and equations using KaTeX notation.",
	"definitions": "Extract a glossary of all key terms, definitions, and acronyms with exact page citations.",
}

// GenerateSummary is single-pass over a spread of excerpts; hierarchical summaries come in Phase B.
func (s *SummaryService) GenerateSummary(ctx context.Context, userID string, req GenerateSummaryRequest) (*SummaryResponse, error) {
	if _, err := s.docs.GetReady(ctx, userID, req.DocumentID); err != nil {
		return nil, err
	}
	if _, ok := summaryInstructions[req.Type]; !ok {
		req.Type = "complete"
	}

	excerpts, err := loadExcerpts(ctx, s.db, s.ragEngine, userID, req.DocumentID, req.Topic, 25)
	if err != nil {
		return nil, err
	}

	var citations []models.Citation
	seen := make(map[int]bool)
	for _, c := range excerpts {
		if seen[c.PageNumber] {
			continue
		}
		seen[c.PageNumber] = true
		citations = append(citations, models.Citation{
			DocumentID:   c.DocumentID,
			DocumentName: c.DocumentName,
			PageNumber:   c.PageNumber,
			Snippet:      snippet(c.ChunkText, 120),
		})
	}

	prompt := fmt.Sprintf(`%s
Ground your summary STRICTLY in the following document excerpts.
Every main concept or claim MUST cite the page number like [Page X], using only page numbers shown below.

EXCERPTS:
%s`, summaryInstructions[req.Type], formatExcerpts(excerpts))

	llmResp, err := s.llmClient.Generate(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are an academic summarizer. Write structured Markdown summaries grounded in citations."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("summary generation failed: %w", err)
	}

	return &SummaryResponse{DocumentID: req.DocumentID, Type: req.Type, Summary: llmResp, Citations: citations}, nil
}

func snippet(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
