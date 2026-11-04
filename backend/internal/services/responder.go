package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
)

// ReplyRequest is one chat message to answer, within a conversation the user owns.
type ReplyRequest struct {
	UserID       string
	Guest        bool
	Conversation *models.Conversation // with its attached Documents
	Message      string
	Style        string // requested explanation style; "" lets the responder decide
	History      []rag.ChatMessage
	OnStatus     func(status string) error // optional progress updates, e.g. "Searching the web"
}

// Reply is the assistant's answer, or a quiz, flashcard set or summary made for the user.
type Reply struct {
	Content   string
	Type      string // "chat", "quiz", "flashcards", "summary"
	Metadata  string // JSON linking the message to the quiz, flashcard set or summary
	Citations []models.Citation
}

// Responder produces the reply to a chat message, streaming text to streamWriter when it is given.
type Responder interface {
	Respond(ctx context.Context, req ReplyRequest, streamWriter func(string) error) (*Reply, error)
}

// Shared wording for study items, so every responder presents them the same way.

func QuizReply(quiz *QuizSessionResponse, topic string) *Reply {
	text := fmt.Sprintf("Let's test your understanding of %s. %d questions, one at a time, and you'll get feedback after each answer.",
		subject(topic), quiz.TotalQuestions)
	return &Reply{Content: text, Type: "quiz", Metadata: metadataJSON(map[string]string{"quiz_session_id": quiz.SessionID})}
}

func FlashcardsReply(set *models.FlashcardSet, topic string) *Reply {
	text := fmt.Sprintf("Here are %d flashcards on %s. Flip each card, then mark it Known or Need Practice.", len(set.Cards), subject(topic))
	return &Reply{Content: text, Type: "flashcards", Metadata: metadataJSON(map[string]string{"flashcard_set_id": set.ID})}
}

func SummaryReply(sum *SummaryResponse) *Reply {
	return &Reply{
		Content:   sum.Summary,
		Type:      "summary",
		Metadata:  metadataJSON(map[string]string{"summary_type": sum.Type, "topic": sum.Topic}),
		Citations: sum.Citations,
	}
}

func subject(topic string) string {
	if topic == "" {
		return "this material"
	}
	return topic
}

func metadataJSON(fields map[string]string) string {
	b, _ := json.Marshal(fields) // a map of strings always marshals
	return string(b)
}
