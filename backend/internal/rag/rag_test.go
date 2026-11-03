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

	msgs, citations := engine.BuildMessages("What is Newton's second law?", chunks, nil)

	if len(msgs) != 2 { // system + user
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if len(citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(citations))
	}

	// Test citation filtering
	answer := "According to [Page 5], Newton's second law relates force and acceleration."
	filtered := filterRelevantCitations(answer, citations)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered citation, got %d", len(filtered))
	}

	if filtered[0].PageNumber != 5 {
		t.Errorf("expected citation for page 5, got %d", filtered[0].PageNumber)
	}

	if got := filterRelevantCitations("No page references here.", citations); len(got) != 0 {
		t.Errorf("uncited answer should get no citations, got %d", len(got))
	}
	if got := filterRelevantCitations("As shown on [Page 99].", citations); len(got) != 0 {
		t.Errorf("citations to pages that were not retrieved must be dropped, got %d", len(got))
	}
	if got := filterRelevantCitations("Hooke saw cells 【Page 5】.", citations); len(got) != 1 {
		t.Errorf("full-width brackets should be recognized, got %d", len(got))
	}
	if got := filterRelevantCitations("See [Pages 5 and 12].", citations); len(got) != 2 {
		t.Errorf("multi-page citations should match both pages, got %d", len(got))
	}
	if got := filterRelevantCitations("Hooke, 1665 [Page 5].", citations); len(got) != 1 {
		t.Errorf("narrow no-break space inside the citation should match, got %d", len(got))
	}
	if got := normalizeCitations("x 【Page 5】"); got != "x [Page 5]" {
		t.Errorf("normalizeCitations: got %q", got)
	}
}
