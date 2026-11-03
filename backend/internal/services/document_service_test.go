package services

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/models"
)

func TestUploadRejectsInvalidFiles(t *testing.T) {
	svc := NewDocumentService(nil, nil, nil, nil, 0) // validation runs before any dependency is used
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, ErrEmptyFile},
		{"not a pdf", []byte("plain text pretending to be a pdf"), ErrNotPDF},
		{"too large", append([]byte("%PDF-"), bytes.Repeat([]byte("x"), MaxUploadBytes)...), ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Upload(context.Background(), "user", "a.pdf", tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestUploadProcessesAndDeduplicates(t *testing.T) {
	h := newHarness(t)
	user := createUser(t, h.db)

	doc := h.uploadReady(t, user)
	if doc.PageCount != 12 || doc.ChunksTotal == 0 || doc.ChunksEmbedded != doc.ChunksTotal {
		t.Fatalf("unexpected processing result: pages=%d chunks=%d/%d", doc.PageCount, doc.ChunksEmbedded, doc.ChunksTotal)
	}
	if doc.StoragePath != user+"/"+doc.ID+".pdf" {
		t.Fatalf("unexpected storage path %q", doc.StoragePath)
	}
	if _, err := h.files.Read(context.Background(), doc.StoragePath); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}

	var chunkCount int64
	h.db.Model(&models.DocumentChunk{}).Where("document_id = ?", doc.ID).Count(&chunkCount)
	if int(chunkCount) != doc.ChunksTotal {
		t.Fatalf("expected %d stored chunks, got %d", doc.ChunksTotal, chunkCount)
	}

	again, err := h.docs.Upload(context.Background(), user, "copy.pdf", readFixture(t, "sample.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != doc.ID {
		t.Fatalf("identical upload should reuse %s, got %s", doc.ID, again.ID)
	}
}

func TestBadPDFsFailWithMessage(t *testing.T) {
	h := newHarness(t)
	user := createUser(t, h.db)

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"corrupted", []byte("%PDF-1.4 this is not really a pdf"), "could not read this PDF"},
		{"no text layer", readFixture(t, "no_text.pdf"), "no extractable text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := h.upload(t, user, tc.data)
			if doc.Status != models.StatusFailed || !strings.Contains(doc.ErrorMessage, tc.want) {
				t.Fatalf("expected FAILED containing %q, got %s: %q", tc.want, doc.Status, doc.ErrorMessage)
			}
		})
	}
}

func TestDocumentsAreIsolatedBetweenUsers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	alice, bob := createUser(t, h.db), createUser(t, h.db)

	// Same file for both users, so their chunks have identical vectors: only the filters keep them apart.
	aliceDoc := h.uploadReady(t, alice)
	bobDoc := h.uploadReady(t, bob)
	if aliceDoc.ID == bobDoc.ID {
		t.Fatal("dedup must not cross users")
	}

	results, err := h.engine.Retrieve(ctx, "cell membrane phospholipid bilayer", alice, aliceDoc.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	for _, r := range results {
		if r.DocumentID != aliceDoc.ID {
			t.Fatalf("search leaked chunk from document %s", r.DocumentID)
		}
	}

	leaked, err := h.engine.Retrieve(ctx, "cell membrane", bob, aliceDoc.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) != 0 {
		t.Fatalf("bob's search over alice's document returned %d results", len(leaked))
	}

	if _, err := h.docs.Get(ctx, bob, aliceDoc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get: expected ErrNotFound, got %v", err)
	}
	if _, err := h.docs.FileURL(ctx, bob, aliceDoc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FileURL: expected ErrNotFound, got %v", err)
	}
	if err := h.docs.Delete(ctx, bob, aliceDoc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete: expected ErrNotFound, got %v", err)
	}
	if _, err := h.chat.CreateConversation(ctx, bob, aliceDoc.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateConversation: expected ErrNotFound, got %v", err)
	}
	docs, err := h.docs.List(ctx, bob)
	if err != nil || len(docs) != 1 || docs[0].ID != bobDoc.ID {
		t.Fatalf("bob should see only his document, got %v (err %v)", docs, err)
	}

	if err := h.docs.Delete(ctx, alice, aliceDoc.ID); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	var remaining int64
	h.db.Model(&models.DocumentChunk{}).Where("document_id = ?", aliceDoc.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("chunks should cascade on delete, %d left", remaining)
	}
}

func TestConversationRequiresReadyDocument(t *testing.T) {
	h := newHarness(t)
	user := createUser(t, h.db)
	failed := h.upload(t, user, readFixture(t, "no_text.pdf"))
	if _, err := h.chat.CreateConversation(context.Background(), user, failed.ID, ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}
}
