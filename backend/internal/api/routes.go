package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/pagepilot/backend/internal/auth"
)

func SetupRouter(api *API, verifier *auth.Verifier, allowedOrigins []string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth uses bearer tokens, not cookies, so credentials are not needed.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
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

			r.Post("/chat", api.ChatHandler)

			r.Route("/quizzes", func(r chi.Router) {
				r.With(auth.RequireAccount).Post("/generate", api.GenerateQuizHandler)
				r.Get("/{id}", api.GetQuizHandler)
				r.Post("/{id}/answer", api.AnswerQuizHandler)
			})

			r.Route("/flashcards", func(r chi.Router) {
				r.With(auth.RequireAccount).Post("/generate", api.GenerateFlashcardsHandler)
				r.Get("/", api.ListFlashcardsHandler)
				r.Get("/{id}", api.GetFlashcardSetHandler)
				r.Patch("/cards/{cardId}", api.UpdateFlashcardStatusHandler)
			})

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAccount)

				r.Route("/documents", func(r chi.Router) {
					r.Post("/upload", api.UploadDocumentHandler)
					r.Get("/", api.ListDocumentsHandler)
					r.Get("/{id}", api.GetDocumentHandler)
					r.Get("/{id}/status", api.GetDocumentStatusHandler)
					r.Get("/{id}/file", api.DocumentFileHandler)
					r.Get("/{id}/sections/{n}", api.DocumentSectionHandler)
					r.Delete("/{id}", api.DeleteDocumentHandler)
				})

				r.Post("/summaries/generate", api.GenerateSummaryHandler)
				r.Get("/progress", api.GetProgressHandler)
				r.Post("/account/claim", api.ClaimGuestHandler(verifier))
			})
		})
	})

	return r
}
