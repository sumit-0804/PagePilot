// Command eval runs PagePilot's AI evaluation suites against a local database and real models.
//
//	go run ./cmd/eval -suite all -judge
//
// Exit codes: 0 ok, 1 a suite is below a threshold or regressed, 2 inconclusive (infrastructure errors).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/agents"
	"github.com/pagepilot/backend/internal/config"
	"github.com/pagepilot/backend/internal/database"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/eval"
	"github.com/pagepilot/backend/internal/observability"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
	"github.com/pagepilot/backend/internal/websearch"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const localDB = "postgresql://postgres:postgres@127.0.0.1:54322/postgres"

func main() {
	code, err := run()
	if err != nil {
		log.Printf("eval: %v", err)
		os.Exit(2) // setup failed: inconclusive, not a quality result
	}
	os.Exit(code)
}

func run() (int, error) {
	suiteFlag := flag.String("suite", "all", "comma-separated suites to run: "+strings.Join(eval.SuiteNames, ", ")+", or all")
	judge := flag.Bool("judge", false, "grade answers, quizzes and flashcards with the LLM judge")
	dataDir := flag.String("data", "evaluation", "directory with corpus/, datasets/, thresholds.json and baselines/")
	topK := flag.Int("k", 5, "how many chunks retrieval returns")
	delay := flag.Duration("delay", 3*time.Second, "pause between LLM cases, to stay within rate limits")
	updateBaseline := flag.Bool("update-baseline", false, "save this run as the new baseline")
	allowRemote := flag.Bool("allow-remote-db", false, "allow EVAL_DATABASE_URL to point at a non-local database")
	noLangfuse := flag.Bool("no-langfuse", false, "don't send traces and scores to Langfuse even if it is configured")
	flag.Parse()

	suites, err := pickSuites(*suiteFlag)
	if err != nil {
		return 0, err
	}
	data, err := eval.Load(*dataDir)
	if err != nil {
		return 0, fmt.Errorf("loading datasets: %w", err)
	}
	thresholds, err := eval.LoadThresholds(filepath.Join(*dataDir, "thresholds.json"))
	if err != nil {
		return 0, err
	}
	baselinePath := filepath.Join(*dataDir, "baselines", "baseline.json")
	var baseline *eval.Report
	if b, err := eval.LoadReport(baselinePath); err == nil {
		baseline = b
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}

	cfg, err := loadConfig(*allowRemote)
	if err != nil {
		return 0, err
	}
	ctx := context.Background()

	// Every span goes to the recorder (tool calls are read from it) and, when configured, to Langfuse.
	spans := tracetest.NewSpanRecorder()
	tel := observability.Config{Environment: "eval", Version: gitCommit(), SpanProcessors: []sdktrace.SpanProcessor{spans}}
	if !*noLangfuse {
		tel.LangfusePublicKey, tel.LangfuseSecretKey, tel.LangfuseBaseURL = cfg.LangfusePublicKey, cfg.LangfuseSecretKey, cfg.LangfuseBaseURL
	}
	shutdown, err := observability.Setup(ctx, tel)
	if err != nil {
		return 0, err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := shutdown(flushCtx); err != nil {
			log.Printf("eval: flushing traces: %v", err)
		}
	}()

	env, err := buildEnv(ctx, cfg, tel, spans, *judge, *topK, *delay)
	if err != nil {
		return 0, err
	}
	runner := eval.NewRunner(*env, data)
	report := &eval.Report{
		DatasetVersion: data.Version, StartedAt: time.Now().UTC(), GitCommit: tel.Version, Judge: *judge,
		Models: map[string]string{"agent": cfg.GroqModel, "embeddings": cfg.EmbeddingModel},
	}
	if *judge {
		report.Models["judge"] = env.Judge.Model
		report.PromptVersions = eval.PromptVersions()
	}

	fmt.Printf("PagePilot eval: dataset %s, suites %s, judge %v, Langfuse %v\n", data.Version, strings.Join(suites, ","), *judge, env.Langfuse != nil)
	if err := runner.Setup(ctx); err != nil {
		runner.Cleanup(ctx)
		return 0, err
	}
	defer runner.Cleanup(context.Background())

	for _, s := range suites {
		res, err := runner.Run(ctx, s)
		if err != nil {
			return 0, err
		}
		report.Suites = append(report.Suites, res)
	}
	report.FinishedAt = time.Now().UTC()
	report.Finish(thresholds, baseline)
	report.PrintSummary(os.Stdout)

	if err := saveReport(report, *dataDir); err != nil {
		return 0, err
	}
	if *updateBaseline {
		if err := report.Save(baselinePath); err != nil {
			return 0, err
		}
		fmt.Printf("Saved as the new baseline: %s\n", baselinePath)
	}
	return report.ExitCode, nil
}

func pickSuites(flagValue string) ([]string, error) {
	if flagValue == "all" {
		return eval.SuiteNames, nil
	}
	var out []string
	for _, s := range strings.Split(flagValue, ",") {
		s = strings.TrimSpace(s)
		if !slices.Contains(eval.SuiteNames, s) {
			return nil, fmt.Errorf("unknown suite %q (choose from %s or all)", s, strings.Join(eval.SuiteNames, ", "))
		}
		out = append(out, s)
	}
	return out, nil
}

// loadConfig reuses the API's settings for models and keys, but points the database at the eval
// database (local by default) and turns off the parts the eval doesn't use.
func loadConfig(allowRemote bool) (*config.Config, error) {
	dbURL := os.Getenv("EVAL_DATABASE_URL")
	if dbURL == "" {
		dbURL = localDB
	}
	if !allowRemote && !isLocal(dbURL) {
		return nil, errors.New("EVAL_DATABASE_URL is not a local database; the eval creates and deletes a test user, so pass -allow-remote-db only for a disposable database")
	}
	// godotenv never overrides variables that are already set, so these win over .env.
	os.Setenv("DATABASE_URL", dbURL)
	os.Setenv("OCR_PROVIDER", "none") // the corpus is Markdown
	// Files go to a temporary local store; Supabase Storage and Auth are never called.
	for k, v := range map[string]string{"SUPABASE_URL": "http://localhost.invalid", "SUPABASE_SECRET_KEY": "unused-by-eval"} {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	return config.Load()
}

func isLocal(dbURL string) bool {
	u, err := url.Parse(dbURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "postgres" // "postgres" is the CI service name
}

func buildEnv(ctx context.Context, cfg *config.Config, tel observability.Config, spans *tracetest.SpanRecorder, judge bool, topK int, delay time.Duration) (*eval.Env, error) {
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	embedder, err := embeddings.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	filesDir, err := os.MkdirTemp("", "pagepilot-eval-")
	if err != nil {
		return nil, err
	}
	vectors := vectorstore.NewPgVectorStore(db)
	groq := rag.NewGroqClient(cfg)
	engine := rag.NewEngine(embedder, vectors, groq)
	docs := services.NewDocumentService(db, storage.NewLocalStore(filesDir), embedder, vectors, cfg.EmbeddingBatchSize)
	quiz := services.NewQuizService(db, docs, engine, groq)
	cards := services.NewFlashcardService(db, docs, engine, groq)
	summaries := services.NewSummaryService(db, docs, engine, groq)
	progress := services.NewProgressService(db, docs)
	searcher := websearch.New(cfg)

	llm, err := agents.NewGroqModel(ctx, cfg.GroqAPIKey, cfg.GroqModel)
	if err != nil {
		return nil, err
	}
	svc := agents.Services{RAG: engine, Quiz: quiz, Cards: cards, Summaries: summaries, Progress: progress}
	if searcher != nil {
		svc.Web = searcher
	}
	agent, err := agents.NewEngine(llm, svc)
	if err != nil {
		return nil, err
	}

	env := &eval.Env{
		DB: db, Docs: docs, RAG: engine, Quiz: quiz, Cards: cards,
		Chat: services.NewChatService(db, docs, agent, groq), Agent: agent,
		Langfuse: observability.NewLangfuseClient(tel), Spans: spans, WebSearch: searcher != nil,
		RunID: uuid.NewString()[:8], TopK: topK, CaseDelay: delay, Out: os.Stdout,
	}
	if judge {
		judgeCfg := *cfg
		judgeCfg.GroqModel = orDefault(os.Getenv("EVAL_JUDGE_MODEL"), cfg.GroqModel)
		env.Judge = &eval.Judge{LLM: rag.NewGroqClient(&judgeCfg), Model: judgeCfg.GroqModel}
	}
	return env, nil
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func gitCommit() string {
	if sha := os.Getenv("GITHUB_SHA"); sha != "" {
		return sha[:min(12, len(sha))]
	}
	out, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func saveReport(r *eval.Report, dataDir string) error {
	dir := filepath.Join(dataDir, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := r.StartedAt.Format("20060102-150405") + ".json"
	for _, p := range []string{filepath.Join(dir, name), filepath.Join(dir, "latest.json")} {
		if err := r.Save(p); err != nil {
			return err
		}
	}
	fmt.Printf("Report: %s\n", filepath.Join(dir, name))
	return nil
}
