package eval

import (
	"context"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/observability"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// Env is the real application wiring the suites run against.
type Env struct {
	DB        *gorm.DB
	Docs      *services.DocumentService
	RAG       *rag.Engine
	Quiz      *services.QuizService
	Cards     *services.FlashcardService
	Chat      *services.ChatService
	Agent     services.Responder
	ToolAgent services.Responder            // the agent with content-generating tools stubbed out
	Judge     *Judge                        // nil when judging is off
	Langfuse  *observability.LangfuseClient // nil when Langfuse is not configured
	Spans     *tracetest.SpanRecorder       // sees every span, so tool calls and token usage can be read back
	WebSearch bool                          // whether the agent has the web_search tool
	RunID     string
	TopK      int
	// Concurrency is how many cases run at once; the model clients retry rate limits.
	Concurrency int
	// Limit runs only the first N cases of each suite (0 = all), for cheap smoke runs.
	Limit int
	// MaxTokens stops starting LLM cases once any one model has used this many tokens in the run (0 = no cap).
	MaxTokens int
	Out       io.Writer
}

// Runner runs suites as a throwaway user who owns the ingested corpus.
type Runner struct {
	Env
	Data   *Data
	user   string
	docIDs map[string]string   // corpus key -> document id
	chunks map[string][]string // corpus key -> indexed chunk texts
	outMu  sync.Mutex
}

func NewRunner(env Env, data *Data) *Runner {
	return &Runner{Env: env, Data: data, docIDs: map[string]string{}, chunks: map[string][]string{}}
}

// Setup creates the eval user and ingests the corpus through the real document pipeline.
func (r *Runner) Setup(ctx context.Context) error {
	// Users left by runs that were killed before Cleanup.
	if err := r.DB.WithContext(ctx).Exec("delete from auth.users where email like 'eval-%@pagepilot.test'").Error; err != nil {
		return fmt.Errorf("removing old eval users: %w", err)
	}
	r.user = uuid.NewString()
	err := r.DB.WithContext(ctx).Exec(`insert into auth.users (id, instance_id, aud, role, email, created_at, updated_at)
		values (?, '00000000-0000-0000-0000-000000000000', 'authenticated', 'authenticated', ?, now(), now())`,
		r.user, "eval-"+r.user[:8]+"@pagepilot.test").Error
	if err != nil {
		return fmt.Errorf("creating eval user: %w", err)
	}

	keys := make([]string, 0, len(r.Data.Corpus))
	for k := range r.Data.Corpus {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		doc, err := r.Docs.Upload(ctx, r.user, key+".md", []byte(r.Data.Corpus[key]))
		if err != nil {
			return fmt.Errorf("uploading %s: %w", key, err)
		}
		r.docIDs[key] = doc.ID
	}
	for _, key := range keys {
		if err := r.waitReady(ctx, key); err != nil {
			return err
		}
		var texts []string
		if err := r.DB.WithContext(ctx).Model(&models.DocumentChunk{}).
			Where("document_id = ?", r.docIDs[key]).Order("chunk_index").Pluck("chunk_text", &texts).Error; err != nil {
			return fmt.Errorf("reading chunks of %s: %w", key, err)
		}
		r.chunks[key] = texts
	}
	fmt.Fprintf(r.Out, "Corpus ready: %d documents\n", len(keys))
	return nil
}

func (r *Runner) waitReady(ctx context.Context, key string) error {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		doc, err := r.Docs.Get(ctx, r.user, r.docIDs[key])
		if err != nil {
			return err
		}
		switch {
		case doc.Status == models.StatusReady:
			return nil
		case doc.Status == models.StatusFailed:
			return fmt.Errorf("indexing %s failed: %s", key, doc.ErrorMessage)
		case time.Now().After(deadline):
			return fmt.Errorf("indexing %s did not finish in time (status %s)", key, doc.Status)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Cleanup deletes the eval user; everything it made goes with it.
func (r *Runner) Cleanup(ctx context.Context) {
	if r.user == "" {
		return
	}
	if err := r.DB.WithContext(ctx).Exec("delete from auth.users where id = ?", r.user).Error; err != nil {
		log.Printf("eval: deleting eval user %s: %v", r.user, err)
	}
}

func (r *Runner) allDocIDs() []string {
	ids := make([]string, 0, len(r.docIDs))
	for _, id := range r.docIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Runner) corpusIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range r.docIDs {
		out[id] = true
	}
	return out
}

// traced runs one case as its own trace (exported to Langfuse when configured), then sends the
// case's chosen metrics to it as scores.
func (r *Runner) traced(ctx context.Context, suite, caseID string, scoreNames []string, run func(ctx context.Context, traceID string) CaseResult) CaseResult {
	if suite != "rag" && r.overBudget() {
		res := CaseResult{ID: caseID, Outcome: Skipped, Problems: []string{fmt.Sprintf("a model reached the run's budget of %d tokens", r.MaxTokens)}}
		r.mark(res.Outcome)
		return res
	}
	ctx = observability.WithTraceAttributes(ctx,
		observability.AttrSessionID.String("eval-"+r.RunID),
		observability.AttrTraceTags.StringSlice([]string{"eval", "suite:" + suite}),
		attribute.String("langfuse.trace.metadata.case_id", caseID),
	)
	ctx, span := observability.StartRoot(ctx, trace.SpanContext{}, "eval."+suite, observability.TypeChain)
	traceID := span.SpanContext().TraceID().String()
	res := run(ctx, traceID)
	res.ID, res.TraceID = caseID, traceID
	span.SetAttributes(attribute.String("langfuse.trace.metadata.outcome", string(res.Outcome)))
	span.End()

	if r.Langfuse != nil && (res.Outcome == Pass || res.Outcome == Fail) {
		for _, name := range scoreNames {
			v, ok := res.Metrics[name]
			if !ok {
				continue
			}
			if err := r.Langfuse.ScoreTrace(ctx, traceID, "eval_"+name, v, ""); err != nil {
				log.Printf("eval: sending %s score to Langfuse: %v", name, err)
			}
		}
	}
	r.mark(res.Outcome)
	return res
}

// mark prints one progress character per finished case.
func (r *Runner) mark(o Outcome) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	fmt.Fprint(r.Out, map[Outcome]string{Pass: ".", Fail: "F", Error: "E", Skipped: "s"}[o])
}

// runCases runs fn for cases 0..n-1, at most Concurrency at a time, keeping results in case order.
func (r *Runner) runCases(n int, fn func(i int) CaseResult) []CaseResult {
	if r.Limit > 0 {
		n = min(n, r.Limit)
	}
	out := make([]CaseResult, n)
	slots := make(chan struct{}, max(1, r.Concurrency))
	var wg sync.WaitGroup
	for i := range n {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			out[i] = fn(i)
		}()
	}
	wg.Wait()
	return out
}

// TokensUsed sums the input and output tokens each model reported in this run. Groq's daily limits
// are per model, so the budget is too.
func (r *Runner) TokensUsed() map[string]int {
	used := map[string]int{}
	for _, s := range r.Spans.Ended() {
		model, tokens := "unknown", 0
		for _, kv := range s.Attributes() {
			switch kv.Key {
			case observability.AttrGenAIRequestModel:
				model = kv.Value.AsString()
			case observability.AttrGenAIInputTokens, observability.AttrGenAIOutputTokens:
				tokens += int(kv.Value.AsInt64())
			}
		}
		if tokens > 0 {
			used[model] += tokens
		}
	}
	return used
}

func (r *Runner) overBudget() bool {
	if r.MaxTokens <= 0 {
		return false
	}
	for _, n := range r.TokensUsed() {
		if n >= r.MaxTokens {
			return true
		}
	}
	return false
}

// toolsCalled reads the agent's tool calls from ADK's execute_tool spans in one trace.
func (r *Runner) toolsCalled(traceID string) []string {
	var tools []string
	for _, s := range r.Spans.Ended() {
		if s.SpanContext().TraceID().String() == traceID {
			if name, ok := strings.CutPrefix(s.Name(), "execute_tool "); ok {
				tools = append(tools, name)
			}
		}
	}
	return tools
}

// conversation creates a chat with the given corpus documents attached, loaded as the agent sees it.
func (r *Runner) conversation(ctx context.Context, keys []string) (*models.Conversation, error) {
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, r.docIDs[k])
	}
	conv, err := r.Chat.CreateConversation(ctx, r.user, ids, "eval")
	if err != nil {
		return nil, err
	}
	return r.Chat.GetConversation(ctx, r.user, conv.ID)
}
