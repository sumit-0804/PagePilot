package pdf

import (
	"testing"
)

func TestChunkDocumentPreservesPages(t *testing.T) {
	pages := []ExtractedPage{
		{
			PageNumber: 1,
			Text:       "Neural networks are computational models inspired by biological neural systems. They consist of layers of interconnected nodes.",
		},
		{
			PageNumber: 2,
			Text:       "Activation functions introduce non-linearity into neural networks. Common activation functions include ReLU, Sigmoid, and Tanh.",
		},
		{
			PageNumber: 3,
			Text:       "Backpropagation is an algorithm used to calculate the gradient of the loss function with respect to each weight by the chain rule.",
		},
	}

	cfg := ChunkerConfig{
		TargetChunkSize: 100,
		OverlapSize:     20,
	}

	chunks := ChunkDocument("doc-123", "user-1", "Machine_Learning.pdf", pages, cfg)

	if len(chunks) == 0 {
		t.Fatalf("expected chunks, got 0")
	}

	// Verify all chunks have correct metadata
	pagesFound := make(map[int]bool)
	for i, c := range chunks {
		if c.DocumentID != "doc-123" {
			t.Errorf("chunk %d: expected docID 'doc-123', got '%s'", i, c.DocumentID)
		}
		if c.UserID != "user-1" {
			t.Errorf("chunk %d: expected userID 'user-1', got '%s'", i, c.UserID)
		}
		if c.PageNumber < 1 || c.PageNumber > 3 {
			t.Errorf("chunk %d: unexpected page number %d", i, c.PageNumber)
		}
		pagesFound[c.PageNumber] = true
		if len(c.ChunkText) < 10 {
			t.Errorf("chunk %d: chunk text unexpectedly short: '%s'", i, c.ChunkText)
		}
	}

	if len(pagesFound) < 3 {
		t.Errorf("expected chunks across all 3 pages, found in %d pages", len(pagesFound))
	}
}

func TestChunkTextSentenceSplitting(t *testing.T) {
	longText := "First sentence here. Second sentence starts right here. Third sentence follows promptly. Fourth sentence concludes the paragraph."
	chunks := chunkText(longText, 60, 15)

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks for long text, got %d", len(chunks))
	}
}
