package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/observability"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type scoreCall struct {
	op, traceID, messageID, reason string
	rating                         int
}

// fakeScorer records the Langfuse calls the service makes in the background.
type fakeScorer struct{ calls chan scoreCall }

func (f *fakeScorer) Score(_ context.Context, traceID, messageID string, rating int, reason string) error {
	f.calls <- scoreCall{op: "score", traceID: traceID, messageID: messageID, rating: rating, reason: reason}
	return nil
}

func (f *fakeScorer) Remove(_ context.Context, messageID string) error {
	f.calls <- scoreCall{op: "remove", messageID: messageID}
	return nil
}

func (f *fakeScorer) next(t *testing.T) scoreCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("expected a Langfuse score call")
		return scoreCall{}
	}
}

// tracedAnswer sends one chat message inside a trace and returns the saved answer and the trace id.
func tracedAnswer(t *testing.T, h *harness, userID string) (*models.Message, string) {
	t.Helper()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	conv, err := h.chat.CreateConversation(context.Background(), userID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := observability.StartRoot(context.Background(), trace.SpanContext{}, "POST /api/chat", observability.TypeSpan)
	defer span.End()
	res, err := h.chat.SendMessage(ctx, userID, conv.ID, "What is a cell?", SendOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.AssistantMessage, span.SpanContext().TraceID().String()
}

func TestRatingAnAnswer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	scorer := &fakeScorer{calls: make(chan scoreCall, 4)}
	svc := NewFeedbackService(h.db, scorer)

	answer, traceID := tracedAnswer(t, h, user)
	if answer.TraceID == nil || *answer.TraceID != traceID {
		t.Fatalf("the answer should keep its trace id: %v", answer.TraceID)
	}

	if _, err := svc.Rate(ctx, user, answer.ID, models.Feedback{Rating: -1, Reason: "not_grounded"}); err != nil {
		t.Fatal(err)
	}
	if c := scorer.next(t); c.op != "score" || c.traceID != traceID || c.rating != -1 || c.reason != "not_grounded" {
		t.Errorf("Langfuse score call: %+v", c)
	}

	// Changing the rating replaces it (one row per answer) and clears the old reason.
	if _, err := svc.Rate(ctx, user, answer.ID, models.Feedback{Rating: 1}); err != nil {
		t.Fatal(err)
	}
	scorer.next(t)
	conv, err := h.chat.GetConversation(ctx, user, answer.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	var got *models.Feedback
	for _, m := range conv.Messages {
		if m.ID == answer.ID {
			got = m.Feedback
		} else if m.Feedback != nil {
			t.Errorf("only the rated answer should carry feedback, %s has %+v", m.Role, m.Feedback)
		}
	}
	if got == nil || got.Rating != 1 || got.Reason != "" {
		t.Fatalf("loaded feedback = %+v, want thumbs up with no reason", got)
	}

	if err := svc.Clear(ctx, user, answer.ID); err != nil {
		t.Fatal(err)
	}
	if c := scorer.next(t); c.op != "remove" || c.messageID != answer.ID {
		t.Errorf("Langfuse remove call: %+v", c)
	}
	var count int64
	h.db.Model(&models.MessageFeedback{}).Where("message_id = ?", answer.ID).Count(&count)
	if count != 0 {
		t.Errorf("feedback rows after clearing: %d", count)
	}
}

func TestFeedbackIsLimitedToOwnAnswers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	owner, stranger := createUser(t, h.db), createUser(t, h.db)
	svc := NewFeedbackService(h.db, nil)

	conv, err := h.chat.CreateConversation(ctx, owner, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.chat.SendMessage(ctx, owner, conv.ID, "Hello", SendOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, user, message string
		fb                  models.Feedback
		want                error
	}{
		{"someone else's answer", stranger, res.AssistantMessage.ID, models.Feedback{Rating: 1}, ErrNotFound},
		{"a user message", owner, res.UserMessage.ID, models.Feedback{Rating: 1}, ErrInvalidInput},
		{"a made-up id", owner, "not-a-uuid", models.Feedback{Rating: 1}, ErrNotFound},
		{"a zero rating", owner, res.AssistantMessage.ID, models.Feedback{Rating: 0}, ErrInvalidInput},
		{"an unknown reason", owner, res.AssistantMessage.ID, models.Feedback{Rating: -1, Reason: "rude"}, ErrInvalidInput},
	}
	for _, c := range cases {
		if _, err := svc.Rate(ctx, c.user, c.message, c.fb); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	if err := svc.Clear(ctx, stranger, res.AssistantMessage.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("clearing someone else's rating: %v", err)
	}

	// Without tracing the answer has no trace id, and rating still works without Langfuse.
	if res.AssistantMessage.TraceID != nil {
		t.Errorf("untraced answer has trace id %q", *res.AssistantMessage.TraceID)
	}
	if _, err := svc.Rate(ctx, owner, res.AssistantMessage.ID, models.Feedback{Rating: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestGuestFeedbackFollowsClaimedChat(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	guest, account := createUser(t, h.db), createUser(t, h.db)
	svc := NewFeedbackService(h.db, nil)

	conv, err := h.chat.CreateConversation(ctx, guest, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.chat.SendMessage(ctx, guest, conv.ID, "Hello", SendOptions{Guest: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rate(ctx, guest, res.AssistantMessage.ID, models.Feedback{Rating: 1}); err != nil {
		t.Fatal(err)
	}
	if err := h.chat.ClaimGuestData(ctx, guest, account); err != nil {
		t.Fatal(err)
	}

	claimed, err := h.chat.GetConversation(ctx, account, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fb := claimed.Messages[len(claimed.Messages)-1].Feedback; fb == nil || fb.Rating != 1 {
		t.Errorf("the guest's rating should move with the chat, got %+v", fb)
	}
}
