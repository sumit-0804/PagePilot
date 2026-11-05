package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pagepilot/backend/internal/embeddings"
)

// slowEmbedder counts how many documents are being embedded at the same time.
type slowEmbedder struct {
	*embeddings.BuiltinEmbedder
	now, peak atomic.Int32
}

func (s *slowEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	n := s.now.Add(1)
	defer s.now.Add(-1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(150 * time.Millisecond)
	return s.BuiltinEmbedder.EmbedDocuments(ctx, texts)
}

func TestDocumentsAreProcessedAtMostTwoAtATime(t *testing.T) {
	h := newHarness(t)
	user := createUser(t, h.db)
	emb := &slowEmbedder{BuiltinEmbedder: embeddings.NewBuiltinEmbedder(1024)}
	docs := NewDocumentService(h.db, h.files, emb, nil, 100)
	docs.vectors = h.docs.vectors

	texts := []string{"Cells divide by mitosis.", "Water evaporates from oceans.", "TCP uses a handshake.", "Napoleon took power in 1799.", "Residuals measure error."}
	var wg sync.WaitGroup
	for i, text := range texts {
		doc, err := docs.Upload(context.Background(), user, "note.txt", []byte(text+" "+string(rune('a'+i))))
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(id string) { defer wg.Done(); docs.waitReadyForTest(t, user, id) }(doc.ID)
	}
	wg.Wait()

	if p := emb.peak.Load(); p != maxConcurrentProcessing {
		t.Errorf("peak concurrent processing = %d, want %d", p, maxConcurrentProcessing)
	}
}

func (s *DocumentService) waitReadyForTest(t *testing.T, userID, docID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		doc, err := s.Get(context.Background(), userID, docID)
		if err != nil {
			t.Error(err)
			return
		}
		if doc.Status == "READY" || doc.Status == "FAILED" {
			if doc.Status == "FAILED" {
				t.Errorf("document failed: %s", doc.ErrorMessage)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("document %s did not finish", docID)
}
