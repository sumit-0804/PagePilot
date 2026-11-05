package agents

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/option"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/observability"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/websearch"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

const (
	appName   = "pagepilot"
	agentName = "pagepilot_assistant"
	// Bounds tool loops: a few searches, maybe one study tool, and the answer.
	maxLLMCalls  = 8
	historyTurns = 10
)

// Services are the Go services the agent's tools call; the agent decides when.
type Services struct {
	RAG       *rag.Engine
	Quiz      *services.QuizService
	Cards     *services.FlashcardService
	Summaries *services.SummaryService
	Progress  *services.ProgressService
	Web       websearch.Searcher // nil turns the web_search tool off
	// BeforeToolCallbacks run after the status report; one returning a result skips the tool (evals use this).
	BeforeToolCallbacks []llmagent.BeforeToolCallback
}

// NewGroqModel talks to Groq through ADK's OpenAI-compatible Chat Completions model. extra options
// come last, so they can replace the defaults (the eval passes its pacing HTTP client).
func NewGroqModel(ctx context.Context, apiKey, modelName string, extra ...option.RequestOption) (model.LLM, error) {
	// Rate limits (429) are retried with Groq's Retry-After instead of failing the reply.
	opts := []option.RequestOption{option.WithMaxRetries(4)}
	if strings.Contains(modelName, "gpt-oss") {
		// The reasoning text is never shown, so don't have Groq send it.
		opts = append(opts, option.WithJSONSet("include_reasoning", false))
	}
	return openaimodel.NewModel(ctx, modelName, &openaimodel.ClientConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.groq.com/openai/v1",
		API:     openaimodel.APIChatCompletions,
		Options: append(opts, extra...),
	})
}

// Engine answers chat messages with an ADK coordinator agent whose tools wrap the study services.
type Engine struct {
	runner   *runner.Runner
	sessions session.Service
}

func NewEngine(llm model.LLM, svc Services) (*Engine, error) {
	tools, err := newTools(svc)
	if err != nil {
		return nil, err
	}
	tutor, err := llmagent.New(llmagent.Config{
		Name:                  agentName,
		Description:           "Chat assistant and tutor: answers directly, searches attached documents and the web, and creates quizzes, flashcards and summaries.",
		Model:                 llm,
		InstructionProvider:   instruction(svc.Web != nil),
		GenerateContentConfig: &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.2)},
		Tools:                 tools,
		BeforeToolCallbacks:   append([]llmagent.BeforeToolCallback{reportStatus}, svc.BeforeToolCallbacks...),
		OnToolErrorCallbacks:  []llmagent.OnToolErrorCallback{logToolError},
	})
	if err != nil {
		return nil, fmt.Errorf("creating tutor agent: %w", err)
	}
	// Postgres holds the conversation; each request gets a short-lived ADK session rebuilt from it.
	sessions := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: appName, Agent: tutor, SessionService: sessions})
	if err != nil {
		return nil, fmt.Errorf("creating runner: %w", err)
	}
	return &Engine{runner: r, sessions: sessions}, nil
}

// turn is what one chat request's tools need (whose documents) and collect (numbered sources, study items).
type turn struct {
	userID         string
	guest          bool
	conversationID string
	style          string
	docs           []models.Document
	onStatus       func(string) error

	mu        sync.Mutex
	sources   []models.Citation
	studyItem *services.Reply
}

type turnKey struct{}

func turnFrom(ctx context.Context) (*turn, error) {
	t, ok := ctx.Value(turnKey{}).(*turn)
	if !ok {
		return nil, errors.New("no chat request in context")
	}
	return t, nil
}

// addSource numbers a source for this answer; the same page or URL keeps its first number.
func (t *turn) addSource(c models.Citation) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.sources {
		if s.Kind == c.Kind && s.URL == c.URL && s.DocumentID == c.DocumentID && s.PageNumber == c.PageNumber {
			return s.N
		}
	}
	c.N = len(t.sources) + 1
	t.sources = append(t.sources, c)
	return c.N
}

// logToolError records tool failures; the model still gets the error and decides how to recover.
func logToolError(ctx agent.Context, tl tool.Tool, _ map[string]any, err error) (map[string]any, error) {
	log.Printf("[agent] %s failed: %v", tl.Name(), err)
	if !services.IsUserError(err) {
		observability.Capture(ctx, err, observability.CategoryTool, map[string]string{"tool": tl.Name()})
	}
	return nil, nil
}

// reportStatus tells the user what the agent is doing before each tool runs.
func reportStatus(ctx agent.Context, tl tool.Tool, _ map[string]any) (map[string]any, error) {
	t, err := turnFrom(ctx)
	if err != nil || t.onStatus == nil {
		return nil, nil
	}
	if status, ok := statusFor[tl.Name()]; ok {
		if err := t.onStatus(status); err != nil {
			log.Printf("sending status: %v", err)
		}
	}
	return nil, nil
}

func (t *turn) setStudyItem(r *services.Reply) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.studyItem = r
}

func (e *Engine) Respond(ctx context.Context, req services.ReplyRequest, streamWriter func(string) error) (*services.Reply, error) {
	t := &turn{
		userID: req.UserID, guest: req.Guest, conversationID: req.Conversation.ID, style: req.Style,
		docs: req.Conversation.Documents, onStatus: req.OnStatus,
	}
	ctx = context.WithValue(ctx, turnKey{}, t)

	sessionID := uuid.NewString()
	created, err := e.sessions.Create(ctx, &session.CreateRequest{AppName: appName, UserID: req.UserID, SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("creating agent session: %w", err)
	}
	defer func() {
		if err := e.sessions.Delete(context.WithoutCancel(ctx), &session.DeleteRequest{AppName: appName, UserID: req.UserID, SessionID: sessionID}); err != nil {
			log.Printf("deleting agent session: %v", err)
		}
	}()
	if err := e.loadHistory(ctx, created.Session, req.History); err != nil {
		return nil, err
	}

	var final string
	msg := genai.NewContentFromText(req.Message, genai.RoleUser)
	runCfg := agent.RunConfig{StreamingMode: agent.StreamingModeSSE, MaxLLMCalls: maxLLMCalls}
	for ev, err := range e.runner.Run(ctx, req.UserID, sessionID, msg, runCfg) {
		if err != nil {
			return nil, fmt.Errorf("tutor agent failed: %w", err)
		}
		if ev.ErrorCode != "" || ev.ErrorMessage != "" {
			return nil, fmt.Errorf("model error %s: %s", ev.ErrorCode, ev.ErrorMessage)
		}
		text := textOf(ev.Content)
		if ev.Partial {
			if text != "" && streamWriter != nil {
				if err := streamWriter(text); err != nil {
					return nil, err
				}
			}
			continue
		}
		if ev.Author == agentName && ev.IsFinalResponse() && text != "" {
			final = text
		}
	}

	if t.studyItem != nil {
		if streamWriter != nil {
			if err := streamWriter(t.studyItem.Content); err != nil {
				return nil, err
			}
		}
		return t.studyItem, nil
	}
	if strings.TrimSpace(final) == "" {
		return nil, errors.New("the assistant didn't produce an answer, please try again")
	}
	answer := rag.NormalizeCitations(final)
	return &services.Reply{Content: answer, Type: "chat", Citations: rag.FilterSources(answer, t.sources)}, nil
}

// loadHistory replays recent chat turns from Postgres into the request's ADK session.
func (e *Engine) loadHistory(ctx context.Context, s session.Session, history []rag.ChatMessage) error {
	for _, m := range history[max(0, len(history)-historyTurns):] {
		ev := session.NewEvent(ctx, "history")
		if m.Role == "user" {
			ev.Author = "user"
			ev.Content = genai.NewContentFromText(m.Content, genai.RoleUser)
		} else {
			ev.Author = agentName
			ev.Content = genai.NewContentFromText(m.Content, genai.RoleModel)
		}
		if err := e.sessions.AppendEvent(ctx, s, ev); err != nil {
			return fmt.Errorf("loading chat history: %w", err)
		}
	}
	return nil
}

// textOf joins an event's visible text, skipping reasoning ("thought") parts.
func textOf(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range c.Parts {
		if p != nil && p.Text != "" && !p.Thought {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func instruction(webSearch bool) func(agent.ReadonlyContext) (string, error) {
	return func(ctx agent.ReadonlyContext) (string, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		b.WriteString(`You are PagePilot, a helpful chat assistant and study tutor.
Decide for each message what it needs: answer directly from your own knowledge when you can, search the attached documents when the question is about them, and use the web for recent or uncertain facts. You may call several tools and search again with different words before answering.

`)
		if len(t.docs) == 0 {
			b.WriteString("ATTACHED DOCUMENTS: none.\n")
		} else {
			b.WriteString("ATTACHED DOCUMENTS (id, name, format):\n")
			for _, d := range t.docs {
				fmt.Fprintf(&b, "- %s, %q, %s, %d %ss\n", d.ID, d.Filename, d.Format, d.PageCount, strings.ToLower(d.SectionLabel()))
			}
		}
		b.WriteString(`
TOOLS:
- search_documents: finds passages in the attached documents. You only see the chat text of earlier turns, never their passages, so search again in THIS turn before every reply that uses the documents, including follow-ups such as "explain it more simply". Pass a standalone query that names the subject (resolve "it" or "that" from the conversation).
`)
		if webSearch {
			b.WriteString("- web_search: searches the live web. Use it for news, current events, prices, versions, dates after your training, or facts you are unsure of.\n")
		}
		b.WriteString(`- create_quiz / create_flashcards: when the user wants to be quizzed or wants flashcards. Use an attached document's id when they mean the document; otherwise pass a topic taken from the message or conversation and leave document_id empty.
- summarize_document: when the user wants a summary or notes for an attached document.
- get_study_progress: when the user asks how they are doing on an attached document.
- list_documents: when you need the attached documents' details.

`)
		b.WriteString(rag.SourceRules + "\n" + rag.StyleRule(t.style))
		return b.String(), nil
	}
}
