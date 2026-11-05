package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pagepilot/backend/internal/agents"
	"github.com/pagepilot/backend/internal/api"
	"github.com/pagepilot/backend/internal/auth"
	"github.com/pagepilot/backend/internal/config"
	"github.com/pagepilot/backend/internal/database"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/observability"
	"github.com/pagepilot/backend/internal/ocr"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
	"github.com/pagepilot/backend/internal/websearch"
)

// hostOf returns just the host of a URL, so logs don't show account ids in paths.
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return "custom endpoint"
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Startup failed: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	telemetryCfg := observability.Config{
		Environment:       cfg.Environment,
		Version:           cfg.Version,
		LangfusePublicKey: cfg.LangfusePublicKey,
		LangfuseSecretKey: cfg.LangfuseSecretKey,
		LangfuseBaseURL:   cfg.LangfuseBaseURL,
		SampleRatio:       cfg.TraceSampleRatio,
	}
	shutdownTracing, err := observability.Setup(ctx, telemetryCfg)
	if err != nil {
		// Tracing is optional; the API runs without it.
		log.Printf("Tracing: off (%v)", err)
	} else if telemetryCfg.Enabled() {
		log.Printf("Tracing: on (environment %s, sampling %.2f, prompt capture %v)", cfg.Environment, cfg.TraceSampleRatio, observability.CaptureContent())
	} else {
		log.Println("Tracing: off (no LANGFUSE_* or OTEL_EXPORTER_OTLP_* settings)")
	}

	flushSentry, err := observability.InitSentry(observability.SentryConfig{
		DSN: cfg.SentryDSN, Environment: cfg.Environment, Release: cfg.Version,
	})
	switch {
	case err != nil:
		// Error reporting is optional; a bad DSN must not stop the API.
		log.Printf("Sentry: off (%v)", err)
	case cfg.SentryDSN != "":
		log.Printf("Sentry: on (environment %s)", cfg.Environment)
	default:
		log.Println("Sentry: off (no SENTRY_DSN)")
	}
	defer flushSentry(2 * time.Second)

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	log.Println("Postgres + pgvector: ok")

	files := storage.NewSupabaseStore(cfg.SupabaseURL, cfg.SupabaseSecretKey, cfg.StorageBucket)
	if err := files.Check(ctx); err != nil {
		return err
	}
	log.Printf("Supabase Storage bucket %q: ok", cfg.StorageBucket)

	verifier, err := auth.NewVerifier(ctx, cfg.SupabaseURL, cfg.SupabaseJWTSecret)
	if err != nil {
		return err
	}
	log.Println("Supabase Auth signing keys: ok")

	embedder, err := embeddings.New(ctx, cfg)
	if err != nil {
		return err
	}
	if err := database.CheckEmbeddingDim(ctx, db, embedder.Dimension()); err != nil {
		return err
	}
	if cfg.EmbeddingProvider == "gemini" {
		log.Printf("Embeddings: Gemini %s (%d dims, limits %d RPM / %d TPM / %d RPD)", embedder.Model(), embedder.Dimension(), cfg.EmbeddingRPM, cfg.EmbeddingTPM, cfg.EmbeddingRPD)
	} else {
		log.Printf("Embeddings: %s via %s (%d dims)", embedder.Model(), hostOf(cfg.EmbeddingBaseURL), embedder.Dimension())
	}

	vectors := vectorstore.NewPgVectorStore(db)
	groq := rag.NewGroqClient(cfg)
	engine := rag.NewEngine(embedder, vectors, groq)

	docService := services.NewDocumentService(db, files, embedder, vectors, cfg.EmbeddingBatchSize)
	recognizer, err := ocr.New(ctx, cfg)
	if err != nil {
		return err
	}
	if recognizer != nil {
		docService.UseOCR(recognizer, cfg.OCRMaxPages)
		log.Printf("OCR: %s (limits %d RPM / %d TPM / %d RPD, up to %d scanned pages per document)", recognizer.Model(), cfg.OCRRPM, cfg.OCRTPM, cfg.OCRRPD, cfg.OCRMaxPages)
	} else {
		log.Println("OCR: off (scanned pages are not read)")
	}
	quizService := services.NewQuizService(db, docService, engine, groq)
	flashcardService := services.NewFlashcardService(db, docService, engine, groq)
	summaryService := services.NewSummaryService(db, docService, engine, groq)
	progressService := services.NewProgressService(db, docService)

	searcher := websearch.New(cfg)
	if searcher != nil {
		log.Printf("Web search: Tavily (up to %d results per search)", cfg.WebSearchMaxResults)
	} else {
		log.Println("Web search: off")
	}

	llm, err := agents.NewGroqModel(ctx, cfg.GroqAPIKey, cfg.GroqModel)
	if err != nil {
		return fmt.Errorf("creating ADK model: %w", err)
	}
	responder, err := agents.NewEngine(llm, agents.Services{
		RAG: engine, Quiz: quizService, Cards: flashcardService, Summaries: summaryService, Progress: progressService,
		Web: searcher,
	})
	if err != nil {
		return err
	}
	log.Printf("Chat agent: ADK (model %s)", cfg.GroqModel)
	chatService := services.NewChatService(db, docService, responder, groq)

	if err := docService.ResumeUnfinished(ctx); err != nil {
		return err
	}

	healthChecks := map[string]func(context.Context) error{
		"database": func(ctx context.Context) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		},
		"storage": files.Check,
	}
	// A nil *LangfuseClient must not become a non-nil interface, or feedback would call into it.
	var scorer services.FeedbackScorer
	if lf := observability.NewLangfuseClient(telemetryCfg); lf != nil {
		scorer = lf
	}
	feedbackService := services.NewFeedbackService(db, scorer)
	apiServer := api.NewAPI(docService, chatService, quizService, flashcardService, summaryService, progressService, feedbackService, healthChecks)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.APIPort),
		Handler:           api.SetupRouter(apiServer, verifier, cfg.FrontendOrigins),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // large uploads
		WriteTimeout:      3 * time.Minute, // chat streaming clears this per request
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("PagePilot API listening on http://localhost:%s (CORS: %v)", cfg.APIPort, cfg.FrontendOrigins)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	// Flush queued spans after in-flight requests finish, within the same deadline.
	if terr := shutdownTracing(shutdownCtx); terr != nil {
		log.Printf("Flushing traces: %v", terr)
	}
	return err
}
