package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/pdf"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

const MaxUploadBytes = 50 << 20

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotPDF       = errors.New("file is not a PDF")
	ErrEmptyFile    = errors.New("file is empty")
	ErrTooLarge     = fmt.Errorf("file is larger than %d MB", MaxUploadBytes>>20)
	ErrNotReady     = errors.New("document is not ready yet")
	pdfMagicBytes   = []byte("%PDF-")
)

type DocumentService struct {
	db        *gorm.DB
	files     storage.FileStore
	embedder  embeddings.Embedder
	vectors   vectorstore.VectorStore
	batchSize int
}

func NewDocumentService(db *gorm.DB, files storage.FileStore, embedder embeddings.Embedder, vectors vectorstore.VectorStore, batchSize int) *DocumentService {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &DocumentService{db: db, files: files, embedder: embedder, vectors: vectors, batchSize: batchSize}
}

// Upload validates and stores a PDF, then processes it in the background.
// An identical file the user already has READY is returned instead of being processed again.
func (s *DocumentService) Upload(ctx context.Context, userID, filename string, data []byte) (*models.Document, error) {
	if len(data) == 0 {
		return nil, ErrEmptyFile
	}
	if len(data) > MaxUploadBytes {
		return nil, ErrTooLarge
	}
	if !bytes.HasPrefix(data, pdfMagicBytes) {
		return nil, ErrNotPDF
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	var existing models.Document
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND content_hash = ? AND status = ?", userID, hash, models.StatusReady).
		Order("created_at DESC").First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking for duplicate upload: %w", err)
	}

	docID := uuid.NewString()
	doc := &models.Document{
		ID:          docID,
		UserID:      userID,
		Filename:    cleanFilename(filename),
		StoragePath: fmt.Sprintf("%s/%s.pdf", userID, docID),
		FileSize:    int64(len(data)),
		ContentHash: hash,
		Status:      models.StatusUploaded,
	}

	if err := s.files.Save(ctx, doc.StoragePath, data, "application/pdf"); err != nil {
		return nil, fmt.Errorf("storing PDF: %w", err)
	}
	if err := s.db.WithContext(ctx).Create(doc).Error; err != nil {
		if delErr := s.files.Delete(ctx, doc.StoragePath); delErr != nil {
			log.Printf("[Doc %s] cleanup after failed insert: %v", docID, delErr)
		}
		return nil, fmt.Errorf("saving document record: %w", err)
	}

	go s.process(doc.ID)
	return doc, nil
}

// ResumeUnfinished restarts documents left mid-processing (server restart, daily quota hit).
func (s *DocumentService) ResumeUnfinished(ctx context.Context) error {
	var ids []string
	err := s.db.WithContext(ctx).Model(&models.Document{}).
		Where("status IN ?", []string{models.StatusUploaded, models.StatusExtracting, models.StatusChunking, models.StatusEmbedding, models.StatusIndexing}).
		Pluck("id", &ids).Error
	if err != nil {
		return fmt.Errorf("finding unfinished documents: %w", err)
	}
	for _, id := range ids {
		log.Printf("[Doc %s] resuming unfinished processing", id)
		go s.process(id)
	}
	return nil
}

func (s *DocumentService) process(docID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	var doc models.Document
	if err := s.db.WithContext(ctx).First(&doc, "id = ?", docID).Error; err != nil {
		log.Printf("[Doc %s] loading for processing: %v", docID, err)
		return
	}
	if err := s.runPipeline(ctx, &doc); err != nil {
		log.Printf("[Doc %s] processing failed: %v", docID, err)
		if uerr := s.update(ctx, docID, map[string]any{"status": models.StatusFailed, "error_message": err.Error()}); uerr != nil {
			log.Printf("[Doc %s] recording failure: %v", docID, uerr)
		}
	}
}

func (s *DocumentService) runPipeline(ctx context.Context, doc *models.Document) error {
	if err := s.update(ctx, doc.ID, map[string]any{"status": models.StatusExtracting, "error_message": "", "chunks_embedded": 0}); err != nil {
		return err
	}
	data, err := s.files.Read(ctx, doc.StoragePath)
	if err != nil {
		return fmt.Errorf("reading stored PDF: %w", err)
	}
	extraction, err := extractPDF(data)
	if err != nil {
		return fmt.Errorf("could not read this PDF (it may be corrupted or encrypted): %w", err)
	}
	if err := s.update(ctx, doc.ID, map[string]any{"page_count": extraction.TotalPages, "status": models.StatusChunking}); err != nil {
		return err
	}

	chunks := pdf.ChunkDocument(doc.ID, doc.UserID, doc.Filename, extraction.Pages, pdf.DefaultChunkerConfig())
	if len(chunks) == 0 {
		return errors.New("no extractable text found in this PDF; if it is scanned, it needs an OCR text layer")
	}
	if err := s.update(ctx, doc.ID, map[string]any{
		"status":          models.StatusEmbedding,
		"chunks_total":    len(chunks),
		"embedding_model": s.embedder.Model(),
	}); err != nil {
		return err
	}

	for start := 0; start < len(chunks); start += s.batchSize {
		end := min(start+s.batchSize, len(chunks))
		texts := make([]string, end-start)
		for i := range texts {
			texts[i] = chunks[start+i].ChunkText
		}
		vecs, err := s.embedder.EmbedDocuments(ctx, texts)
		if err != nil {
			return fmt.Errorf("embedding failed: %w", err)
		}
		for i, v := range vecs {
			chunks[start+i].Embedding = pgvector.NewVector(v)
		}
		if err := s.update(ctx, doc.ID, map[string]any{"chunks_embedded": end}); err != nil {
			return err
		}
	}

	if err := s.update(ctx, doc.ID, map[string]any{"status": models.StatusIndexing}); err != nil {
		return err
	}
	if err := s.vectors.ReplaceChunks(ctx, doc.ID, chunks); err != nil {
		return fmt.Errorf("saving chunks: %w", err)
	}
	if err := s.update(ctx, doc.ID, map[string]any{"status": models.StatusReady}); err != nil {
		return err
	}
	log.Printf("[Doc %s] READY (%d pages, %d chunks)", doc.ID, extraction.TotalPages, len(chunks))
	return nil
}

func (s *DocumentService) update(ctx context.Context, docID string, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	if err := s.db.WithContext(ctx).Model(&models.Document{}).Where("id = ?", docID).Updates(fields).Error; err != nil {
		return fmt.Errorf("updating document: %w", err)
	}
	return nil
}

func (s *DocumentService) List(ctx context.Context, userID string) ([]models.Document, error) {
	var docs []models.Document
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&docs).Error
	return docs, err
}

// Get returns ErrNotFound for missing documents and for documents owned by someone else.
func (s *DocumentService) Get(ctx context.Context, userID, docID string) (*models.Document, error) {
	if _, err := uuid.Parse(docID); err != nil {
		return nil, ErrNotFound
	}
	var doc models.Document
	err := s.db.WithContext(ctx).First(&doc, "id = ? AND user_id = ?", docID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetReady is Get plus a check that processing finished.
func (s *DocumentService) GetReady(ctx context.Context, userID, docID string) (*models.Document, error) {
	doc, err := s.Get(ctx, userID, docID)
	if err != nil {
		return nil, err
	}
	if doc.Status != models.StatusReady {
		return nil, ErrNotReady
	}
	return doc, nil
}

func (s *DocumentService) FileURL(ctx context.Context, userID, docID string) (string, error) {
	doc, err := s.Get(ctx, userID, docID)
	if err != nil {
		return "", err
	}
	return s.files.SignedURL(ctx, doc.StoragePath, 10*time.Minute)
}

// Delete removes the record (chunks, chats and quizzes cascade), then the stored file.
func (s *DocumentService) Delete(ctx context.Context, userID, docID string) error {
	doc, err := s.Get(ctx, userID, docID)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&models.Document{}, "id = ? AND user_id = ?", docID, userID).Error; err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}
	if err := s.files.Delete(ctx, doc.StoragePath); err != nil {
		return fmt.Errorf("document deleted but its file could not be removed: %w", err)
	}
	return nil
}

// extractPDF writes the bytes to a temp file because the PDF library reads from disk.
func extractPDF(data []byte) (result *pdf.ExtractionResult, err error) {
	// The PDF library can panic on malformed files; treat that as a bad PDF, not a crash.
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, fmt.Errorf("malformed PDF: %v", r)
		}
	}()
	tmp, err := os.CreateTemp("", "pagepilot-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	return pdf.ExtractText(tmp.Name())
}

func cleanFilename(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return "document.pdf"
	}
	if len(base) > 200 {
		base = base[:200]
	}
	return base
}
