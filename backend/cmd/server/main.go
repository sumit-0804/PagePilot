package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"github.com/pagepilot/backend/internal/storage"
	"github.com/pagepilot/backend/internal/vectorstore"
)

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
	log.Printf("Embeddings: %s (%d dims, limits %d RPM / %d TPM / %d RPD)", embedder.Model(), embedder.Dimension(), cfg.EmbeddingRPM, cfg.EmbeddingTPM, cfg.EmbeddingRPD)

	vectors := vectorstore.NewPgVectorStore(db)
	groq := rag.NewGroqClient(cfg)
	engine := rag.NewEngine(embedder, vectors, groq)

	docService := services.NewDocumentService(db, files, embedder, vectors, cfg.EmbeddingBatchSize)
	quizService := services.NewQuizService(db, docService, engine, groq)
	flashcardService := services.NewFlashcardService(db, docService, engine, groq)
	summaryService := services.NewSummaryService(db, docService, engine, groq)
	progressService := services.NewProgressService(db, docService)

	var responder services.Responder
	switch cfg.ChatEngine {
	case "adk":
		llm, err := agents.NewGroqModel(ctx, cfg.GroqAPIKey, cfg.GroqModel)
		if err != nil {
			return fmt.Errorf("creating ADK model: %w", err)
		}
		responder, err = agents.NewEngine(llm, agents.Services{
			RAG: engine, Quiz: quizService, Cards: flashcardService, Summaries: summaryService, Progress: progressService,
		})
		if err != nil {
			return err
		}
	case "direct":
		responder = services.NewDirectResponder(groq, engine, quizService, flashcardService, summaryService)
	}
	log.Printf("Chat engine: %s (model %s)", cfg.ChatEngine, cfg.GroqModel)
	chatService := services.NewChatService(db, docService, responder)

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
	apiServer := api.NewAPI(docService, chatService, quizService, flashcardService, summaryService, progressService, healthChecks)

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
	return server.Shutdown(shutdownCtx)
}
