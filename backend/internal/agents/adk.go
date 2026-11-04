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
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/vectorstore"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const (
	appName   = "pagepilot"
	agentName = "study_tutor"
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
}

// NewGroqModel talks to Groq through ADK's OpenAI-compatible Chat Completions model.
func NewGroqModel(ctx context.Context, apiKey, modelName string) (model.LLM, error) {
	var opts []option.RequestOption
	if strings.Contains(modelName, "gpt-oss") {
		// The reasoning text is never shown, so don't have Groq send it.
		opts = append(opts, option.WithJSONSet("include_reasoning", false))
	}
	return openaimodel.NewModel(ctx, modelName, &openaimodel.ClientConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.groq.com/openai/v1",
		API:     openaimodel.APIChatCompletions,
		Options: opts,
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
		Description:           "Study tutor for one uploaded PDF: answers from the document, and creates quizzes, flashcards and summaries.",
		Model:                 llm,
		InstructionProvider:   instruction,
		GenerateContentConfig: &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.2)},
		Tools:                 tools,
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

// turn is what one chat request's tools need (whose document) and collect (pages, study items).
type turn struct {
	userID         string
	documentID     string
	conversationID string
	style          string

	mu        sync.Mutex
	retrieved []vectorstore.SearchResult
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

func (t *turn) addRetrieved(results []vectorstore.SearchResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.retrieved = append(t.retrieved, results...)
}

func (t *turn) setStudyItem(r *services.Reply) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.studyItem = r
}

func (e *Engine) Respond(ctx context.Context, req services.ReplyRequest, streamWriter func(string) error) (*services.Reply, error) {
	t := &turn{userID: req.UserID, documentID: req.Conversation.DocumentID, conversationID: req.Conversation.ID, style: req.Style}
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
		return nil, errors.New("the tutor didn't produce an answer, please try again")
	}
	answer := rag.NormalizeCitations(final)
	return &services.Reply{Content: answer, Type: "chat", Citations: rag.FilterCitations(answer, rag.CitationsFor(t.retrieved))}, nil
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

func instruction(ctx agent.ReadonlyContext) (string, error) {
	t, err := turnFrom(ctx)
	if err != nil {
		return "", err
	}
	return `You are PagePilot, a study tutor for one PDF the student uploaded.

TOOLS:
- search_document: finds passages in the PDF. You only see the chat text of earlier turns, never their passages, so call search_document in THIS turn before every reply that uses the document, including follow-ups such as "explain it more simply", "give another example" or "go deeper". Pass a standalone query that names the subject (use the conversation to resolve "it" or "that"). If the passages don't cover the question, search again with different words. Cite each passage with its "cite" value, exactly as given.
- create_quiz: when the student wants to be quizzed or tested ("test my understanding", "quiz me").
- create_flashcards: when the student wants flashcards.
- summarize_document: when the student wants a summary, notes, revision notes, definitions, key concepts, formulas or exam notes for the document.
- get_study_progress: when the student asks how they are doing, what to review, or what their weak areas are. Explain the result in plain words.
For a quiz, flashcards or a summary, take the topic from the message or the conversation; leave it empty for the whole document.

` + rag.GroundingRules + "\n" + rag.StyleRule(t.style), nil
}
