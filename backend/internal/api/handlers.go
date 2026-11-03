package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pagepilot/backend/internal/auth"
	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/services"
)

const maxJSONBody = 1 << 20

type API struct {
	docService       *services.DocumentService
	chatService      *services.ChatService
	quizService      *services.QuizService
	flashcardService *services.FlashcardService
	summaryService   *services.SummaryService
	progressService  *services.ProgressService
	healthChecks     map[string]func(ctx context.Context) error
}

func NewAPI(
	docSvc *services.DocumentService,
	chatSvc *services.ChatService,
	quizSvc *services.QuizService,
	fcSvc *services.FlashcardService,
	sumSvc *services.SummaryService,
	progressSvc *services.ProgressService,
	healthChecks map[string]func(ctx context.Context) error,
) *API {
	return &API{
		docService:       docSvc,
		chatService:      chatSvc,
		quizService:      quizSvc,
		flashcardService: fcSvc,
		summaryService:   sumSvc,
		progressService:  progressSvc,
		healthChecks:     healthChecks,
	}
}

// HealthHandler reports each dependency; 503 if any is down.
func (a *API) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := http.StatusOK
	checks := make(map[string]string, len(a.healthChecks))
	for name, check := range a.healthChecks {
		if err := check(ctx); err != nil {
			checks[name] = "error: " + err.Error()
			status = http.StatusServiceUnavailable
		} else {
			checks[name] = "ok"
		}
	}
	respondJSON(w, status, map[string]any{"status": map[bool]string{true: "healthy", false: "degraded"}[status == http.StatusOK], "checks": checks})
}

// Documents

func (a *API) UploadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, services.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.fail(w, services.ErrTooLarge)
			return
		}
		respondError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing 'file' in upload form")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, services.MaxUploadBytes+1))
	if err != nil {
		respondError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}

	doc, err := a.docService.Upload(r.Context(), auth.UserID(r.Context()), header.Filename, data)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusAccepted, doc)
}

func (a *API) ListDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	docs, err := a.docService.List(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, docs)
}

func (a *API) GetDocumentHandler(w http.ResponseWriter, r *http.Request) {
	doc, err := a.docService.Get(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func (a *API) GetDocumentStatusHandler(w http.ResponseWriter, r *http.Request) {
	doc, err := a.docService.Get(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"id":              doc.ID,
		"status":          doc.Status,
		"error_message":   doc.ErrorMessage,
		"page_count":      doc.PageCount,
		"chunks_total":    doc.ChunksTotal,
		"chunks_embedded": doc.ChunksEmbedded,
		"updated_at":      doc.UpdatedAt,
	})
}

// DocumentFileHandler returns a short-lived signed URL; the viewer loads the PDF straight from storage.
func (a *API) DocumentFileHandler(w http.ResponseWriter, r *http.Request) {
	url, err := a.docService.FileURL(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (a *API) DeleteDocumentHandler(w http.ResponseWriter, r *http.Request) {
	if err := a.docService.Delete(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id")); err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Document deleted"})
}

// Conversations

func (a *API) CreateConversationHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DocumentID string `json:"document_id"`
		Title      string `json:"title"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DocumentID == "" {
		respondError(w, http.StatusBadRequest, "document_id is required")
		return
	}
	conv, err := a.chatService.CreateConversation(r.Context(), auth.UserID(r.Context()), body.DocumentID, body.Title)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, conv)
}

func (a *API) ListConversationsHandler(w http.ResponseWriter, r *http.Request) {
	convs, err := a.chatService.ListConversations(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, convs)
}

func (a *API) GetConversationHandler(w http.ResponseWriter, r *http.Request) {
	conv, err := a.chatService.GetConversation(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, conv)
}

func (a *API) GetConversationMessagesHandler(w http.ResponseWriter, r *http.Request) {
	conv, err := a.chatService.GetConversation(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, conv.Messages)
}

func (a *API) DeleteConversationHandler(w http.ResponseWriter, r *http.Request) {
	if err := a.chatService.DeleteConversation(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id")); err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Conversation deleted"})
}

// Chat

func (a *API) ChatHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversation_id"`
		DocumentID     string `json:"document_id"`
		Message        string `json:"message"`
		Stream         bool   `json:"stream"`
		Style          string `json:"style"` // optional explanation style, e.g. "simple", "exam"
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	if body.Message == "" {
		respondError(w, http.StatusBadRequest, "message cannot be empty")
		return
	}
	if len(body.Message) > 8000 {
		respondError(w, http.StatusBadRequest, "message is too long (max 8000 characters)")
		return
	}

	ctx := r.Context()
	userID := auth.UserID(ctx)
	if body.ConversationID == "" {
		if body.DocumentID == "" {
			respondError(w, http.StatusBadRequest, "conversation_id or document_id is required")
			return
		}
		conv, err := a.chatService.CreateConversation(ctx, userID, body.DocumentID, "")
		if err != nil {
			a.fail(w, err)
			return
		}
		body.ConversationID = conv.ID
	}

	opts := services.SendOptions{Style: body.Style}
	if !body.Stream && r.Header.Get("Accept") != "text/event-stream" {
		result, err := a.chatService.SendMessage(ctx, userID, body.ConversationID, body.Message, opts, nil)
		if err != nil {
			a.fail(w, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"conversation_id":   body.ConversationID,
			"user_message":      result.UserMessage,
			"assistant_message": result.AssistantMessage,
			"citations":         result.Citations,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	// Streams can outlive the server-wide write timeout.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("chat: clearing write deadline: %v", err)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	send := func(payload map[string]any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	// Heartbeat comments keep proxies from closing a stream that is waiting on the model.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				mu.Lock()
				_, _ = fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
				mu.Unlock()
			}
		}
	}()

	result, err := a.chatService.SendMessage(ctx, userID, body.ConversationID, body.Message, opts, func(token string) error {
		return send(map[string]any{"type": "token", "content": token})
	})
	if err != nil {
		log.Printf("chat: %v", err)
		_ = send(map[string]any{"type": "error", "error": publicMessage(err)})
		return
	}
	_ = send(map[string]any{
		"type":              "done",
		"conversation_id":   body.ConversationID,
		"user_message":      result.UserMessage,
		"assistant_message": result.AssistantMessage,
		"citations":         result.Citations,
	})
}

// Quiz

func (a *API) GenerateQuizHandler(w http.ResponseWriter, r *http.Request) {
	var req services.GenerateQuizRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := a.quizService.GenerateQuiz(r.Context(), auth.UserID(r.Context()), req)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, resp)
}

func (a *API) GetQuizHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := a.quizService.GetSession(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (a *API) AnswerQuizHandler(w http.ResponseWriter, r *http.Request) {
	var req services.AnswerSubmissionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	feedback, err := a.quizService.SubmitAnswer(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"), req)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, feedback)
}

// Flashcards

func (a *API) GenerateFlashcardsHandler(w http.ResponseWriter, r *http.Request) {
	var req services.GenerateFlashcardsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	set, err := a.flashcardService.GenerateFlashcards(r.Context(), auth.UserID(r.Context()), req)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, set)
}

func (a *API) ListFlashcardsHandler(w http.ResponseWriter, r *http.Request) {
	sets, err := a.flashcardService.ListFlashcardSets(r.Context(), auth.UserID(r.Context()), r.URL.Query().Get("document_id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sets)
}

func (a *API) GetFlashcardSetHandler(w http.ResponseWriter, r *http.Request) {
	set, err := a.flashcardService.GetFlashcardSet(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, set)
}

func (a *API) UpdateFlashcardStatusHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"` // "new", "known", "practice"
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := a.flashcardService.UpdateCardStatus(r.Context(), auth.UserID(r.Context()), chi.URLParam(r, "cardId"), body.Status); err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Card status updated"})
}

// Summaries

func (a *API) GenerateSummaryHandler(w http.ResponseWriter, r *http.Request) {
	var req services.GenerateSummaryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Long documents are summarized section by section, which can outlast the server-wide write timeout.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("summary: clearing write deadline: %v", err)
	}
	resp, err := a.summaryService.GenerateSummary(r.Context(), auth.UserID(r.Context()), req)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// Progress

func (a *API) GetProgressHandler(w http.ResponseWriter, r *http.Request) {
	docID := r.URL.Query().Get("document_id")
	if docID == "" {
		respondError(w, http.StatusBadRequest, "document_id is required")
		return
	}
	progress, err := a.progressService.Get(r.Context(), auth.UserID(r.Context()), docID)
	if err != nil {
		a.fail(w, err)
		return
	}
	respondJSON(w, http.StatusOK, progress)
}

// Helpers

// fail maps service errors to status codes; unexpected errors are logged and returned as 500.
func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound):
		respondError(w, http.StatusNotFound, "not found")
	case errors.Is(err, services.ErrInvalidInput), errors.Is(err, services.ErrNotPDF), errors.Is(err, services.ErrEmptyFile):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, services.ErrTooLarge):
		respondError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, services.ErrNotReady), errors.Is(err, services.ErrAlreadyAnswered):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, embeddings.ErrDailyQuota):
		respondError(w, http.StatusTooManyRequests, err.Error())
	default:
		log.Printf("request failed: %v", err)
		respondError(w, http.StatusInternalServerError, publicMessage(err))
	}
}

// publicMessage keeps user-facing messages useful without dumping upstream response bodies.
func publicMessage(err error) string {
	switch {
	case errors.Is(err, services.ErrNotFound):
		return "not found"
	case errors.Is(err, embeddings.ErrDailyQuota):
		return err.Error()
	}
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && len(msg) > 200 {
		return msg[:i]
	}
	return msg
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("writing response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
