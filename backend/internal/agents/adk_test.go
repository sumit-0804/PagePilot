package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/database"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
	"github.com/pagepilot/backend/internal/websearch"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

// These tests drive the real ADK runner with a scripted model, and real services on the local
// Supabase database (bunx supabase start). No API keys are needed.

// scriptedModel plays one scripted step per model call and records each request.
type scriptedModel struct {
	mu       sync.Mutex
	steps    []func(req *model.LLMRequest) ([]*model.LLMResponse, error)
	requests []*model.LLMRequest
}

func (m *scriptedModel) Name() string { return "scripted" }

func (m *scriptedModel) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.mu.Lock()
		i := len(m.requests)
		m.requests = append(m.requests, req)
		m.mu.Unlock()
		if i >= len(m.steps) {
			yield(nil, fmt.Errorf("unexpected model call %d", i+1))
			return
		}
		responses, err := m.steps[i](req)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, r := range responses {
			if !yield(r, nil) {
				return
			}
		}
	}
}

func (m *scriptedModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func callTool(name string, args map[string]any) func(*model.LLMRequest) ([]*model.LLMResponse, error) {
	return func(*model.LLMRequest) ([]*model.LLMResponse, error) {
		part := genai.NewPartFromFunctionCall(name, args)
		part.FunctionCall.ID = "call-" + name
		return []*model.LLMResponse{{Content: genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)}}, nil
	}
}

// answer streams text in chunks, then sends the complete text, as a streaming model does.
func answer(chunks ...string) func(*model.LLMRequest) ([]*model.LLMResponse, error) {
	return func(*model.LLMRequest) ([]*model.LLMResponse, error) {
		var out []*model.LLMResponse
		for _, c := range chunks {
			out = append(out, &model.LLMResponse{Content: genai.NewContentFromText(c, genai.RoleModel), Partial: true})
		}
		full := genai.NewContentFromText(strings.Join(chunks, ""), genai.RoleModel)
		return append(out, &model.LLMResponse{Content: full, TurnComplete: true}), nil
	}
}

// jsonLLM stands in for Groq inside the quiz and flashcard services.
type jsonLLM struct{ json string }

func (f jsonLLM) Generate(context.Context, []rag.ChatMessage) (string, error)     { return "", nil }
func (f jsonLLM) GenerateJSON(context.Context, []rag.ChatMessage) (string, error) { return f.json, nil }
func (f jsonLLM) Stream(context.Context, []rag.ChatMessage, func(string) error) (string, error) {
	return "", nil
}

type fixture struct {
	db   *gorm.DB
	svc  Services
	conv *models.Conversation
	user string
}

func setup(t *testing.T, serviceLLM rag.LLMClient) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgresql://postgres:postgres@127.0.0.1:54322/postgres"
	}
	db, err := database.Connect(url)
	if err != nil {
		t.Skipf("local Supabase database not reachable (run `bunx supabase start`): %v", err)
	}

	user := uuid.NewString()
	if err := db.Exec(`insert into auth.users (id, instance_id, aud, role, email, created_at, updated_at)
		values (?, '00000000-0000-0000-0000-000000000000', 'authenticated', 'authenticated', ?, now(), now())`,
		user, "agent-"+user[:8]+"@pagepilot.test").Error; err != nil {
		t.Fatalf("creating test user: %v", err)
	}
	t.Cleanup(func() { db.Exec("delete from auth.users where id = ?", user) })

	emb := embeddings.NewBuiltinEmbedder(1536)
	vectors := vectorstore.NewPgVectorStore(db)
	engine := rag.NewEngine(emb, vectors, serviceLLM)
	docs := services.NewDocumentService(db, storage.NewLocalStore(t.TempDir()), emb, vectors, 5)

	pdf, err := os.ReadFile("../services/testdata/sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := docs.Upload(context.Background(), user, "Biology Handbook.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(30 * time.Second); ; {
		got, err := docs.Get(context.Background(), user, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == models.StatusReady {
			break
		}
		if got.Status == models.StatusFailed || time.Now().After(deadline) {
			t.Fatalf("document not ready: %s %s", got.Status, got.ErrorMessage)
		}
		time.Sleep(100 * time.Millisecond)
	}

	conv := &models.Conversation{ID: uuid.NewString(), UserID: user, Title: "test"}
	if err := db.Create(conv).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ConversationDocument{ConversationID: conv.ID, DocumentID: doc.ID, UserID: user}).Error; err != nil {
		t.Fatal(err)
	}
	ready, err := docs.Get(context.Background(), user, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	conv.Documents = []models.Document{*ready}
	return &fixture{
		db:   db,
		conv: conv,
		user: user,
		svc: Services{
			RAG:       engine,
			Quiz:      services.NewQuizService(db, docs, engine, serviceLLM),
			Cards:     services.NewFlashcardService(db, docs, engine, serviceLLM),
			Summaries: services.NewSummaryService(db, docs, engine, serviceLLM),
			Progress:  services.NewProgressService(db, docs),
		},
	}
}

func (f *fixture) engine(t *testing.T, m *scriptedModel) *Engine {
	t.Helper()
	e, err := NewEngine(m, f.svc)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (f *fixture) request(message, style string, history ...rag.ChatMessage) services.ReplyRequest {
	return services.ReplyRequest{UserID: f.user, Conversation: f.conv, Message: message, Style: style, History: history}
}

// functionResponse finds the result the runner sent back to the model for a tool call.
func functionResponse(req *model.LLMRequest, name string) map[string]any {
	for _, c := range req.Contents {
		for _, p := range c.Parts {
			if p.FunctionResponse != nil && p.FunctionResponse.Name == name {
				return p.FunctionResponse.Response
			}
		}
	}
	return nil
}

// citeFor finds the marker the search tool gave a page, e.g. "[2]".
func citeFor(t *testing.T, searched map[string]any, location string) string {
	t.Helper()
	excerpts, _ := searched["excerpts"].([]any)
	for _, e := range excerpts {
		ex, _ := e.(map[string]any)
		if ex["location"] == location {
			return ex["cite"].(string)
		}
	}
	raw, _ := json.Marshal(searched)
	t.Fatalf("no excerpt for %s in %s", location, raw)
	return ""
}

func TestAgentAnswersFromSearchedPages(t *testing.T) {
	f := setup(t, jsonLLM{})
	var searched map[string]any
	var statuses []string
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		callTool("search_documents", map[string]any{"query": "Robert Hooke first described cells in 1665 observing cork"}),
		func(req *model.LLMRequest) ([]*model.LLMResponse, error) {
			searched = functionResponse(req, "search_documents")
			n := strings.Trim(citeFor(t, searched, "Page 2"), "[]")
			return answer("Robert Hooke described cells in 1665 ", "【"+n+"†L1-L4】. ", "Invented claim [99].")(req)
		},
	}}

	req := f.request("Explain who first described cells", "simple")
	req.OnStatus = func(s string) error { statuses = append(statuses, s); return nil }
	var streamed []string
	reply, err := f.engine(t, m).Respond(context.Background(), req, func(tok string) error {
		streamed = append(streamed, tok)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	cite := citeFor(t, searched, "Page 2")
	if reply.Type != "chat" || len(reply.Citations) != 1 || reply.Citations[0].PageNumber != 2 ||
		reply.Citations[0].Kind != models.CitationDocument || reply.Citations[0].Label != "Page" {
		t.Fatalf("only the retrieved page 2 may be cited: %+v", reply)
	}
	if !strings.Contains(reply.Content, cite) {
		t.Fatalf("saved answer should have normalized the %s marker: %q", cite, reply.Content)
	}
	if len(streamed) < 3 {
		t.Fatalf("expected streamed chunks, got %v", streamed)
	}
	if len(statuses) != 1 || statuses[0] != "Searching your documents" {
		t.Fatalf("expected a search status, got %v", statuses)
	}
	system := textOf(m.requests[0].Config.SystemInstruction)
	if !strings.Contains(system, rag.ExplanationStyles["simple"]) || !strings.Contains(system, "Biology Handbook.pdf") {
		t.Fatal("the requested style and attached documents should be in the system instruction")
	}
	if strings.Contains(system, "web_search") {
		t.Fatal("web search is off in this engine, so the prompt must not offer it")
	}
}

type fakeSearcher struct{}

func (fakeSearcher) Search(context.Context, string) (*websearch.Result, error) {
	return &websearch.Result{
		Answer:  "Go 1.27 is the latest release.",
		Sources: []websearch.Source{{Title: "Go releases", URL: "https://go.dev/doc/devel/release"}},
	}, nil
}

func TestAgentSearchesTheWebWithoutDocuments(t *testing.T) {
	f := setup(t, jsonLLM{})
	f.svc.Web = fakeSearcher{}
	f.conv.Documents = nil
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		callTool("web_search", map[string]any{"query": "latest Go release"}),
		func(req *model.LLMRequest) ([]*model.LLMResponse, error) {
			if functionResponse(req, "web_search") == nil {
				return nil, errors.New("web result missing from the model input")
			}
			return answer("The latest Go is 1.27 [1].")(req)
		},
	}}
	reply, err := f.engine(t, m).Respond(context.Background(), f.request("What's the latest Go version?", ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Citations) != 1 || reply.Citations[0].Kind != models.CitationWeb || reply.Citations[0].URL != "https://go.dev/doc/devel/release" {
		t.Fatalf("expected one web citation: %+v", reply.Citations)
	}
	if !strings.Contains(textOf(m.requests[0].Config.SystemInstruction), "ATTACHED DOCUMENTS: none") {
		t.Fatal("the prompt should say no documents are attached")
	}
}

func TestAgentReplaysHistory(t *testing.T) {
	f := setup(t, jsonLLM{})
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){answer("Sure.")}}
	history := []rag.ChatMessage{
		{Role: "user", Content: "Tell me about photosynthesis"},
		{Role: "assistant", Content: "Photosynthesis happens in chloroplasts [Page 5]."},
	}
	if _, err := f.engine(t, m).Respond(context.Background(), f.request("Explain it more simply", "", history...), nil); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, c := range m.requests[0].Contents {
		seen = append(seen, c.Role+": "+textOf(c))
	}
	joined := strings.Join(seen, "\n")
	for _, want := range []string{"user: Tell me about photosynthesis", "model: Photosynthesis happens in chloroplasts", "user: Explain it more simply"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("model input should contain %q, got:\n%s", want, joined)
		}
	}
}

func TestAgentCreatesQuizAndEndsTurn(t *testing.T) {
	f := setup(t, jsonLLM{json: `{"questions": [
	  {"question": "Who described cells?", "options": ["Hooke", "Darwin", "Mendel", "Crick"], "correct_option": 0, "citation_page": 2},
	  {"question": "Where is ATP made?", "options": ["Mitochondria", "Ribosome", "Nucleus", "Wall"], "correct_option": 0, "citation_page": 4}
	]}`})
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		callTool("create_quiz", map[string]any{"document_id": f.conv.Documents[0].ID, "topic": "cells", "num_questions": 2}),
	}}

	var streamed []string
	reply, err := f.engine(t, m).Respond(context.Background(), f.request("Test my understanding", ""), func(tok string) error {
		streamed = append(streamed, tok)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Type != "quiz" || !strings.Contains(reply.Content, "cells") || len(streamed) != 1 {
		t.Fatalf("expected a quiz reply streamed once: %+v (%d tokens)", reply, len(streamed))
	}
	if m.calls() != 1 {
		t.Fatalf("the quiz should end the turn without another model call, got %d calls", m.calls())
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(reply.Metadata), &meta); err != nil {
		t.Fatal(err)
	}
	var session models.QuizSession
	if err := f.db.First(&session, "id = ?", meta["quiz_session_id"]).Error; err != nil {
		t.Fatalf("quiz session not saved: %v", err)
	}
	if session.UserID != f.user || session.ConversationID == nil || *session.ConversationID != f.conv.ID || session.TotalQuestions != 2 {
		t.Fatalf("quiz saved for the wrong user or conversation: %+v", session)
	}
}

func TestAgentExplainsProgress(t *testing.T) {
	f := setup(t, jsonLLM{})
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		callTool("get_study_progress", map[string]any{}),
		func(req *model.LLMRequest) ([]*model.LLMResponse, error) {
			if functionResponse(req, "get_study_progress") == nil {
				return nil, errors.New("progress result missing from the model input")
			}
			return answer("You haven't taken a quiz yet.")(req)
		},
	}}
	reply, err := f.engine(t, m).Respond(context.Background(), f.request("How am I doing?", ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Type != "chat" || len(reply.Citations) != 0 || m.calls() != 2 {
		t.Fatalf("progress answer: %+v after %d calls", reply, m.calls())
	}
}

func TestAgentModelErrorSurfaces(t *testing.T) {
	f := setup(t, jsonLLM{})
	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		func(*model.LLMRequest) ([]*model.LLMResponse, error) {
			return nil, errors.New("groq returned status 503")
		},
	}}
	if _, err := f.engine(t, m).Respond(context.Background(), f.request("Hello", ""), nil); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("a model failure must surface as an error, got %v", err)
	}
}
