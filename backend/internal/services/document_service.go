package services

import (
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
	"github.com/pagepilot/backend/internal/extract"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/ocr"
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
	// Shown to users as-is, so they read as sentences.
	ErrUnsupportedFile = errors.New("This file type isn't supported. Upload a PDF, Word (.docx), PowerPoint (.pptx), text, Markdown, CSV or HTML file.")
	ErrEmptyFile       = errors.New("This file is empty.")
	ErrTooLarge        = fmt.Errorf("This file is larger than %d MB.", MaxUploadBytes>>20)
	ErrNotReady        = errors.New("document is not ready yet")
)

var mimeTypes = map[extract.Format]string{
	extract.PDF:  "application/pdf",
	extract.DOCX: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	extract.PPTX: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	extract.TXT:  "text/plain",
	extract.MD:   "text/markdown",
	extract.CSV:  "text/csv",
	extract.HTML: "text/html",
}

// A page with less text than this is treated as scanned and read with OCR (when OCR is on).
const minPageTextChars = 40

type DocumentService struct {
	db          *gorm.DB
	files       storage.FileStore
	embedder    embeddings.Embedder
	vectors     vectorstore.VectorStore
	batchSize   int
	ocr         ocr.Recognizer // nil when OCR is off
	ocrMaxPages int
}

func NewDocumentService(db *gorm.DB, files storage.FileStore, embedder embeddings.Embedder, vectors vectorstore.VectorStore, batchSize int) *DocumentService {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &DocumentService{db: db, files: files, embedder: embedder, vectors: vectors, batchSize: batchSize}
}

// UseOCR turns on reading scanned pages, up to maxPages per document.
func (s *DocumentService) UseOCR(r ocr.Recognizer, maxPages int) {
	s.ocr, s.ocrMaxPages = r, maxPages
}

// Upload validates and stores a document, then processes it in the background.
// An identical file the user already has READY is returned instead of being processed again.
func (s *DocumentService) Upload(ctx context.Context, userID, filename string, data []byte) (*models.Document, error) {
	if len(data) == 0 {
		return nil, ErrEmptyFile
	}
	if len(data) > MaxUploadBytes {
		return nil, ErrTooLarge
	}
	format, err := extract.DetectFormat(data[:min(len(data), 4096)], filename)
	if err != nil {
		return nil, ErrUnsupportedFile
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	var existing models.Document
	err = s.db.WithContext(ctx).
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
		StoragePath: fmt.Sprintf("%s/%s.%s", userID, docID, format),
		FileSize:    int64(len(data)),
		ContentHash: hash,
		Status:      models.StatusUploaded,
		Format:      string(format),
		MimeType:    mimeTypes[format],
	}

	if err := s.files.Save(ctx, doc.StoragePath, data, doc.MimeType); err != nil {
		return nil, fmt.Errorf("storing file: %w", err)
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
		if uerr := s.update(ctx, docID, map[string]any{"status": models.StatusFailed, "error_message": failureMessage(err)}); uerr != nil {
			log.Printf("[Doc %s] recording failure: %v", docID, uerr)
		}
	}
}

// processingError carries the sentence shown to the user; the wrapped cause goes only to the log.
type processingError struct {
	message string
	cause   error
}

func (e *processingError) Error() string { return e.message + ": " + e.cause.Error() }
func (e *processingError) Unwrap() error { return e.cause }

func failWith(message string, cause error) error {
	return &processingError{message: message, cause: cause}
}

func failureMessage(err error) string {
	if errors.Is(err, embeddings.ErrDailyQuota) {
		return "The daily limit for indexing documents has been reached. Upload this file again after the limit resets at midnight Pacific time."
	}
	if errors.Is(err, ocr.ErrDailyQuota) {
		return "The daily limit for reading scanned pages has been reached. Upload this PDF again after the limit resets at midnight Pacific time."
	}
	var pe *processingError
	if errors.As(err, &pe) {
		return pe.message
	}
	return "Something went wrong while processing this file. Try uploading it again."
}

func isPDF(doc *models.Document) bool {
	return doc.Format == "" || doc.Format == string(extract.PDF)
}

func (s *DocumentService) runPipeline(ctx context.Context, doc *models.Document) error {
	if err := s.update(ctx, doc.ID, map[string]any{"status": models.StatusExtracting, "error_message": "", "chunks_embedded": 0, "ocr_pages": 0}); err != nil {
		return err
	}
	data, err := s.files.Read(ctx, doc.StoragePath)
	if err != nil {
		return fmt.Errorf("reading stored file: %w", err)
	}
	var pages []pdf.ExtractedPage
	if isPDF(doc) {
		pages, err = s.readPDF(ctx, doc, data)
	} else {
		pages, err = s.readOther(ctx, doc, data)
	}
	if err != nil {
		return err
	}

	chunks := pdf.ChunkDocument(doc.ID, doc.UserID, doc.Filename, pages, pdf.DefaultChunkerConfig())
	if len(chunks) == 0 {
		switch {
		case !isPDF(doc):
			return failWith("No text was found in this file.", errors.New("no extractable text"))
		case s.ocr != nil:
			return failWith("No readable text was found in this PDF, even after reading its scanned pages.", errors.New("no text after OCR"))
		}
		return failWith("No text was found in this PDF. If it's a scanned document, run it through OCR to add a text layer, then upload it again.",
			errors.New("no extractable text"))
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
		batchStarted := time.Now()
		vecs, err := s.embedder.EmbedDocuments(ctx, texts)
		if err != nil {
			return failWith("Indexing this file failed. Try uploading it again in a few minutes.", err)
		}
		log.Printf("[Doc %s] embedded passages %d-%d of %d in %s", doc.ID, start+1, end, len(chunks), time.Since(batchStarted).Round(time.Millisecond))
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
	log.Printf("[Doc %s] READY (%d sections, %d chunks)", doc.ID, len(pages), len(chunks))
	return nil
}

// readPDF extracts a PDF's pages and reads scanned ones with OCR.
func (s *DocumentService) readPDF(ctx context.Context, doc *models.Document, data []byte) ([]pdf.ExtractedPage, error) {
	started := time.Now()
	extraction, err := extractPDF(data)
	if err != nil {
		return nil, failWith("This PDF couldn't be read. It may be damaged or password-protected.", err)
	}
	log.Printf("[Doc %s] extracted %d pages in %s", doc.ID, extraction.TotalPages, time.Since(started).Round(time.Millisecond))
	if err := s.update(ctx, doc.ID, map[string]any{"page_count": extraction.TotalPages}); err != nil {
		return nil, err
	}
	ocrPages, err := s.readScannedPages(ctx, doc.ID, data, extraction.Pages)
	if err != nil {
		return nil, err
	}
	if err := s.update(ctx, doc.ID, map[string]any{"ocr_pages": ocrPages, "status": models.StatusChunking}); err != nil {
		return nil, err
	}
	return extraction.Pages, nil
}

// readOther splits a Word, PowerPoint or text document into sections, stored like PDF pages.
func (s *DocumentService) readOther(ctx context.Context, doc *models.Document, data []byte) ([]pdf.ExtractedPage, error) {
	sections, err := extractSections(extract.Format(doc.Format), data)
	if err != nil {
		if errors.Is(err, extract.ErrNoText) {
			return nil, failWith("No text was found in this file.", err)
		}
		return nil, failWith("This file couldn't be read. It may be damaged or in an unexpected format.", err)
	}
	pages := make([]pdf.ExtractedPage, len(sections))
	for i, sec := range sections {
		pages[i] = pdf.ExtractedPage{PageNumber: sec.Number, Text: sec.Text}
	}
	log.Printf("[Doc %s] extracted %d %s sections", doc.ID, len(sections), doc.Format)
	if err := s.update(ctx, doc.ID, map[string]any{"page_count": len(sections), "status": models.StatusChunking}); err != nil {
		return nil, err
	}
	return pages, nil
}

// Section returns the text of one section of a non-PDF document, for the in-app viewer.
func (s *DocumentService) Section(ctx context.Context, userID, docID string, number int) (*extract.Section, error) {
	doc, err := s.GetReady(ctx, userID, docID)
	if err != nil {
		return nil, err
	}
	if isPDF(doc) {
		return nil, fmt.Errorf("%w: PDFs are shown in the PDF viewer", ErrInvalidInput)
	}
	data, err := s.files.Read(ctx, doc.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("reading stored file: %w", err)
	}
	sections, err := extractSections(extract.Format(doc.Format), data)
	if err != nil {
		return nil, fmt.Errorf("extracting sections: %w", err)
	}
	for i := range sections {
		if sections[i].Number == number {
			return &sections[i], nil
		}
	}
	return nil, ErrNotFound
}

func extractSections(format extract.Format, data []byte) ([]extract.Section, error) {
	if format != extract.DOCX && format != extract.PPTX {
		return extract.ExtractText(data, format)
	}
	// The zip reader needs random access, so Office files go through a temp file.
	tmp, err := os.CreateTemp("", "pagepilot-*."+string(format))
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
	return extract.ExtractOffice(tmp.Name(), format)
}

// readScannedPages fills in the text of pages that have (almost) no text layer and returns how many it read.
func (s *DocumentService) readScannedPages(ctx context.Context, docID string, data []byte, pages []pdf.ExtractedPage) (int, error) {
	if s.ocr == nil {
		return 0, nil
	}
	var scanned []int // indexes into pages
	for i, p := range pages {
		if len([]rune(strings.TrimSpace(p.Text))) < minPageTextChars {
			scanned = append(scanned, i)
		}
	}
	if len(scanned) == 0 {
		return 0, nil
	}
	if len(scanned) > s.ocrMaxPages {
		return 0, failWith(fmt.Sprintf("This PDF has %d scanned pages, and PagePilot reads up to %d scanned pages per document. Split it into smaller PDFs and upload them separately.", len(scanned), s.ocrMaxPages),
			fmt.Errorf("%d scanned pages, OCR_MAX_PAGES is %d", len(scanned), s.ocrMaxPages))
	}

	numbers := make([]int, len(scanned))
	for i, idx := range scanned {
		numbers[i] = pages[idx].PageNumber
	}
	started := time.Now()
	texts, err := s.ocr.Recognize(ctx, data, numbers)
	if err != nil {
		return 0, failWith("Reading the scanned pages of this PDF failed. Try uploading it again in a few minutes.", err)
	}
	read := 0
	for i, idx := range scanned {
		if text := strings.TrimSpace(texts[i]); text != "" {
			pages[idx].Text = text
			read++
		}
	}
	log.Printf("[Doc %s] read %d of %d scanned pages with %s in %s", docID, read, len(scanned), s.ocr.Model(), time.Since(started).Round(time.Millisecond))
	return read, nil
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
