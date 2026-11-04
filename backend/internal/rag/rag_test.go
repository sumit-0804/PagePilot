package rag

import (
	"testing"

	"github.com/pagepilot/backend/internal/vectorstore"
)

func TestBuildMessagesAndFilterCitations(t *testing.T) {
	engine := NewEngine(nil, nil, nil)

	chunks := []vectorstore.SearchResult{
		{
			ChunkID:      "c1",
			DocumentID:   "doc-1",
			DocumentName: "Physics.pdf",
			PageNumber:   5,
			ChunkText:    "Newton's second law states that F = ma.",
			Score:        0.95,
		},
		{
			ChunkID:      "c2",
			DocumentID:   "doc-1",
			DocumentName: "Physics.pdf",
			PageNumber:   12,
			ChunkText:    "Energy is conserved in closed systems.",
			Score:        0.88,
		},
	}

	msgs, citations := engine.BuildMessages("What is Newton's second law?", "", chunks, nil)

	if len(msgs) != 2 { // system + user
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if len(citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(citations))
	}

	// Test citation filtering
	answer := "According to [Page 5], Newton's second law relates force and acceleration."
	filtered := FilterCitations(answer, citations)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered citation, got %d", len(filtered))
	}

	if filtered[0].PageNumber != 5 {
		t.Errorf("expected citation for page 5, got %d", filtered[0].PageNumber)
	}

	if got := FilterCitations("No page references here.", citations); len(got) != 0 {
		t.Errorf("uncited answer should get no citations, got %d", len(got))
	}
	if got := FilterCitations("As shown on [Page 99].", citations); len(got) != 0 {
		t.Errorf("citations to pages that were not retrieved must be dropped, got %d", len(got))
	}
	if got := FilterCitations("Hooke saw cells 【Page 5】.", citations); len(got) != 1 {
		t.Errorf("full-width brackets should be recognized, got %d", len(got))
	}
	if got := FilterCitations("See [Pages 5 and 12].", citations); len(got) != 2 {
		t.Errorf("multi-page citations should match both pages, got %d", len(got))
	}
	if got := FilterCitations("Hooke, 1665 [Page 5].", citations); len(got) != 1 {
		t.Errorf("narrow no-break space inside the citation should match, got %d", len(got))
	}
	if got := NormalizeCitations("x 【Page 5】"); got != "x [Page 5]" {
		t.Errorf("normalizeCitations: got %q", got)
	}
	if got := NormalizeCitations("Founded 1987 [[Page 137]] and [ [Pages 3, 5] ]."); got != "Founded 1987 [Page 137] and [Pages 3, 5]." {
		t.Errorf("doubled brackets should collapse: got %q", got)
	}
}
