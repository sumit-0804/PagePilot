package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/database"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
	"gorm.io/gorm"
)

// These tests run against the local Supabase database (bunx supabase start) with a fake LLM
// and the hash embedder, so they need no API keys.
var (
	testDB    *gorm.DB
	testDBErr error
)

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@127.0.0.1:54322/postgres"
	}
	testDB, testDBErr = database.Connect(url)
	os.Exit(m.Run())
}

func requireDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testDB == nil {
		t.Skipf("local Supabase database not reachable (run `bunx supabase start`): %v", testDBErr)
	}
	return testDB
}

// createUser inserts an auth user; deleting it at cleanup cascades to all of its data.
func createUser(t *testing.T, db *gorm.DB) string {
	t.Helper()
	id := uuid.NewString()
	err := db.Exec(`insert into auth.users (id, instance_id, aud, role, email, created_at, updated_at)
		values (?, '00000000-0000-0000-0000-000000000000', 'authenticated', 'authenticated', ?, now(), now())`,
		id, fmt.Sprintf("test-%s@pagepilot.test", id[:8])).Error
	if err != nil {
		t.Fatalf("creating test user: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("delete from auth.users where id = ?", id).Error; err != nil {
			t.Errorf("deleting test user: %v", err)
		}
	})
	return id
}

// fakeLLM answers by kind of call, told apart by the system prompt.
type fakeLLM struct {
	answer string // Generate and Stream
	json   string // quiz and flashcard generation
	grade  string // short-answer grading
	err    error

	generateCalls  int
	generatePrompt []string // user prompts sent to Generate
	lastSystem     string   // system prompt of the last Generate or Stream call
	lastJSONPrompt string   // user prompt of the last GenerateJSON call
}

func (f *fakeLLM) Generate(_ context.Context, msgs []rag.ChatMessage) (string, error) {
	f.generateCalls++
	f.generatePrompt = append(f.generatePrompt, msgs[len(msgs)-1].Content)
	f.lastSystem = msgs[0].Content
	return f.answer, f.err
}

func (f *fakeLLM) GenerateJSON(_ context.Context, msgs []rag.ChatMessage) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.lastJSONPrompt = msgs[len(msgs)-1].Content
	if strings.Contains(msgs[0].Content, "answer grader") {
		return f.grade, nil
	}
	return f.json, nil
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func (f *fakeLLM) Stream(_ context.Context, msgs []rag.ChatMessage, onChunk func(string) error) (string, error) {
	f.lastSystem = msgs[0].Content
	if f.err != nil {
		return "", f.err
	}
	for _, word := range strings.SplitAfter(f.answer, " ") {
		if err := onChunk(word); err != nil {
			return "", err
		}
	}
	return f.answer, nil
}

type harness struct {
	db        *gorm.DB
	llm       *fakeLLM
	engine    *rag.Engine
	docs      *DocumentService
	chat      *ChatService
	responder *stubResponder
	quiz      *QuizService
	cards     *FlashcardService
	summaries *SummaryService
	progress  *ProgressService
	files     *storage.LocalStore
}

func newHarness(t *testing.T) *harness {
	db := requireDB(t)
	emb := embeddings.NewBuiltinEmbedder(1536) // must match the vector(1536) column
	vectors := vectorstore.NewPgVectorStore(db)
	files := storage.NewLocalStore(t.TempDir())
	llm := &fakeLLM{}
	engine := rag.NewEngine(emb, vectors, llm)
	docs := NewDocumentService(db, files, emb, vectors, 5)
	quiz := NewQuizService(db, docs, engine, llm)
	cards := NewFlashcardService(db, docs, engine, llm)
	summaries := NewSummaryService(db, docs, engine, llm)
	responder := &stubResponder{reply: &Reply{Content: "OK", Type: "chat"}}
	return &harness{
		db:        db,
		llm:       llm,
		engine:    engine,
		docs:      docs,
		chat:      NewChatService(db, docs, responder),
		responder: responder,
		quiz:      quiz,
		cards:     cards,
		summaries: summaries,
		progress:  NewProgressService(db, docs),
		files:     files,
	}
}

func intPtr(n int) *int { return &n }

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// upload stores a PDF and waits for background processing to finish.
func (h *harness) upload(t *testing.T, userID string, data []byte) *models.Document {
	t.Helper()
	ctx := context.Background()
	doc, err := h.docs.Upload(ctx, userID, "Biology Handbook.pdf", data)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	return h.waitReady(t, userID, doc.ID)
}

// waitReady polls until background processing finishes.
func (h *harness) waitReady(t *testing.T, userID, docID string) *models.Document {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		current, err := h.docs.Get(ctx, userID, docID)
		if err != nil {
			t.Fatalf("polling status: %v", err)
		}
		if current.Status == models.StatusReady || current.Status == models.StatusFailed {
			return current
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("document %s did not finish processing", docID)
	return nil
}

func (h *harness) uploadReady(t *testing.T, userID string) *models.Document {
	t.Helper()
	doc := h.upload(t, userID, readFixture(t, "sample.pdf"))
	if doc.Status != models.StatusReady {
		t.Fatalf("expected READY, got %s: %s", doc.Status, doc.ErrorMessage)
	}
	return doc
}
