package api

import (
	"net/http"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/pagepilot/backend/internal/auth"
	"github.com/pagepilot/backend/internal/observability"
)

func SetupRouter(api *API, verifier *auth.Verifier, allowedOrigins []string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Inside Recoverer: Sentry reports a panic, then re-panics so Recoverer still answers 500.
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)

	// Auth uses bearer tokens, not cookies, so credentials are not needed.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders: []string{observability.TraceHeader},
		MaxAge:         300,
	}))

	r.Get("/health", api.HealthHandler)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", api.HealthHandler)

		// Guests (anonymous Supabase users) get through here; account-only routes add RequireAccount.
		r.Group(func(r chi.Router) {
			r.Use(verifier.Middleware)

			r.Route("/conversations", func(r chi.Router) {
				r.Post("/", api.CreateConversationHandler)
				r.Get("/", api.ListConversationsHandler)
				r.Get("/{id}", api.GetConversationHandler)
				r.Get("/{id}/messages", api.GetConversationMessagesHandler)
				r.Delete("/{id}", api.DeleteConversationHandler)
				r.With(auth.RequireAccount).Post("/{id}/documents", api.AttachDocumentHandler)
				r.With(auth.RequireAccount).Delete("/{id}/documents/{docId}", api.DetachDocumentHandler)
			})

			// Only the AI routes are traced (observability.HTTP), to keep within Langfuse's free quota.
			r.With(observability.HTTP("/api/chat")).Post("/chat", api.ChatHandler)

			r.Route("/quizzes", func(r chi.Router) {
				r.With(auth.RequireAccount, observability.HTTP("/api/quizzes/generate")).Post("/generate", api.GenerateQuizHandler)
				r.Get("/{id}", api.GetQuizHandler)
				r.Post("/{id}/answer", api.AnswerQuizHandler)
			})

			r.Route("/flashcards", func(r chi.Router) {
				r.With(auth.RequireAccount, observability.HTTP("/api/flashcards/generate")).Post("/generate", api.GenerateFlashcardsHandler)
				r.Get("/", api.ListFlashcardsHandler)
				r.Get("/{id}", api.GetFlashcardSetHandler)
				r.Patch("/cards/{cardId}", api.UpdateFlashcardStatusHandler)
			})

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAccount)

				r.Route("/documents", func(r chi.Router) {
					r.With(observability.HTTP("/api/documents/upload")).Post("/upload", api.UploadDocumentHandler)
					r.Get("/", api.ListDocumentsHandler)
					r.Get("/{id}", api.GetDocumentHandler)
					r.Get("/{id}/status", api.GetDocumentStatusHandler)
					r.Get("/{id}/file", api.DocumentFileHandler)
					r.Get("/{id}/sections/{n}", api.DocumentSectionHandler)
					r.Delete("/{id}", api.DeleteDocumentHandler)
				})

				r.With(observability.HTTP("/api/summaries/generate")).Post("/summaries/generate", api.GenerateSummaryHandler)
				r.Get("/progress", api.GetProgressHandler)
				r.Post("/account/claim", api.ClaimGuestHandler(verifier))
			})
		})
	})

	return r
}
