package services

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/pagepilot/backend/internal/models"
)

// stubResponder stands in for the agent; the agent itself is tested in package agents.
type stubResponder struct {
	reply *Reply
	err   error
	last  ReplyRequest
}

func (s *stubResponder) Respond(_ context.Context, req ReplyRequest, streamWriter func(string) error) (*Reply, error) {
	s.last = req
	if s.err != nil {
		return nil, s.err
	}
	if streamWriter != nil {
		if err := streamWriter(s.reply.Content); err != nil {
			return nil, err
		}
	}
	reply := *s.reply // a fresh reply per message, as the agent returns
	return &reply, nil
}

func TestChatWithAndWithoutDocuments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)

	general, err := h.chat.CreateConversation(ctx, user, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.responder.reply = &Reply{Content: "Paris.", Type: "chat"}
	result, err := h.chat.SendMessage(ctx, user, general.ID, "Capital of France?", SendOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantMessage.Content != "Paris." || len(h.responder.last.Conversation.Documents) != 0 {
		t.Fatalf("general chat: %+v, docs %v", result.AssistantMessage, h.responder.last.Conversation.Documents)
	}

	conv, err := h.chat.CreateConversation(ctx, user, []string{doc.ID, doc.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	cite := models.Citation{N: 1, Kind: models.CitationDocument, DocumentID: doc.ID, PageNumber: 2, Label: "Page", Snippet: "Hooke"}
	h.responder.reply = &Reply{Content: "Hooke described cells [1].", Type: "chat", Citations: []models.Citation{cite}}
	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "Who described cells?", SendOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if docs := h.responder.last.Conversation.Documents; len(docs) != 1 || docs[0].ID != doc.ID {
		t.Fatalf("the responder should see the one attached document, got %+v", docs)
	}
	got, err := h.chat.GetConversation(ctx, user, conv.ID)
	if err != nil || len(got.Messages) != 2 || got.Messages[1].CitationsJSON == "" || got.Title != "Who described cells?" {
		t.Fatalf("stored conversation: %+v (err %v)", got, err)
	}

	h.responder.err = errors.New("groq returned status 503")
	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "Again", SendOptions{}, nil); err == nil {
		t.Fatal("a responder failure must surface as an error")
	}
	h.responder.err = nil

	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "x", SendOptions{Style: "poetic"}, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown style: expected ErrInvalidInput, got %v", err)
	}
	other := createUser(t, h.db)
	if _, err := h.chat.SendMessage(ctx, other, conv.ID, "hi", SendOptions{}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's conversation: expected ErrNotFound, got %v", err)
	}
}

func TestAnswersGetTopicFollowUps(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user, guest := createUser(t, h.db), createUser(t, h.db)

	h.llm.json = `{"follow_ups": [
		{"text": "Why does water boil lower on Everest?", "kind": "question"},
		{"text": "Quiz me on boiling points.", "kind": "quiz"},
		{"text": "Explain vapor pressure simply", "kind": "made-up"},
		{"text": "", "kind": "deeper"},
		{"text": "This suggestion is far too long to fit on a small chip under the answer", "kind": "deeper"},
		{"text": "Give another boiling example", "kind": "example"},
		{"text": "One too many", "kind": "question"}
	]}`
	h.responder.reply = &Reply{Content: "Water boils at 100 °C at sea level.", Type: "chat"}
	conv, err := h.chat.CreateConversation(ctx, user, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.chat.SendMessage(ctx, user, conv.ID, "Boiling point of water?", SendOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		FollowUps []FollowUp `json:"follow_ups"`
	}
	if err := json.Unmarshal([]byte(result.AssistantMessage.MetadataJSON), &meta); err != nil {
		t.Fatalf("metadata %q: %v", result.AssistantMessage.MetadataJSON, err)
	}
	want := []FollowUp{
		{Text: "Why does water boil lower on Everest?", Kind: "question"},
		{Text: "Quiz me on boiling points", Kind: "quiz"},
		{Text: "Explain vapor pressure simply", Kind: "question"},
		{Text: "Give another boiling example", Kind: "example"},
	}
	if !slices.Equal(meta.FollowUps, want) {
		t.Fatalf("follow-ups = %+v", meta.FollowUps)
	}
	if !contains(h.llm.lastJSONPrompt, "Boiling point of water?") || !contains(h.llm.lastJSONPrompt, "100 °C") {
		t.Fatal("the follow-up prompt should include the question and the answer")
	}

	h.llm.json = "not json"
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "And on Everest?", SendOptions{}, nil)
	if err != nil || result.AssistantMessage.MetadataJSON != "" {
		t.Fatalf("a follow-up failure must not fail the answer: %q (err %v)", result.AssistantMessage.MetadataJSON, err)
	}

	h.llm.json = `{"follow_ups": [{"text": "Anything", "kind": "question"}]}`
	guestConv, err := h.chat.CreateConversation(ctx, guest, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err = h.chat.SendMessage(ctx, guest, guestConv.ID, "Hi", SendOptions{Guest: true}, nil)
	if err != nil || result.AssistantMessage.MetadataJSON != "" {
		t.Fatalf("guests can't send a follow-up, so none should be made: %q (err %v)", result.AssistantMessage.MetadataJSON, err)
	}
}

func TestAttachAndDetachDocuments(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	alice, bob := createUser(t, h.db), createUser(t, h.db)
	doc, bobDoc := h.uploadReady(t, alice), h.uploadReady(t, bob)

	conv, err := h.chat.CreateConversation(ctx, alice, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.chat.AttachDocument(ctx, alice, conv.ID, bobDoc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attaching another user's document: expected ErrNotFound, got %v", err)
	}
	got, err := h.chat.AttachDocument(ctx, alice, conv.ID, doc.ID)
	if err != nil || len(got.Documents) != 1 {
		t.Fatalf("attach: %+v (err %v)", got, err)
	}
	if got, err = h.chat.AttachDocument(ctx, alice, conv.ID, doc.ID); err != nil || len(got.Documents) != 1 {
		t.Fatalf("attaching twice should be a no-op: %+v (err %v)", got, err)
	}
	if got, err = h.chat.DetachDocument(ctx, alice, conv.ID, doc.ID); err != nil || len(got.Documents) != 0 {
		t.Fatalf("detach: %+v (err %v)", got, err)
	}
	if _, err := h.chat.DetachDocument(ctx, alice, conv.ID, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("detaching twice: expected ErrNotFound, got %v", err)
	}
	if _, err := h.docs.Get(ctx, alice, doc.ID); err != nil {
		t.Fatalf("detaching must keep the document: %v", err)
	}
}

func TestGuestGetsOneMessageThenClaimMovesChat(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	guest, account := createUser(t, h.db), createUser(t, h.db)

	conv, err := h.chat.CreateConversation(ctx, guest, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h.responder.reply = &Reply{Content: "Hi!", Type: "chat"}
	if _, err := h.chat.SendMessage(ctx, guest, conv.ID, "Hello", SendOptions{Guest: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !h.responder.last.Guest {
		t.Fatal("the responder should know the caller is a guest")
	}
	if _, err := h.chat.SendMessage(ctx, guest, conv.ID, "More", SendOptions{Guest: true}, nil); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("second guest message: expected ErrLoginRequired, got %v", err)
	}
	if err := h.chat.CheckGuestAllowance(ctx, guest); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("allowance after one message: %v", err)
	}

	h.llm.json = `{"cards": [{"front": "Capital of France?", "back": "Paris", "citation_page": 0}]}`
	set, err := h.cards.GenerateFlashcards(ctx, guest, GenerateFlashcardsRequest{ConversationID: conv.ID, Topic: "capitals"})
	if err != nil {
		t.Fatal(err)
	}

	if err := h.chat.ClaimGuestData(ctx, guest, account); err != nil {
		t.Fatal(err)
	}
	if _, err := h.chat.GetConversation(ctx, account, conv.ID); err != nil {
		t.Fatalf("claimed conversation should belong to the account: %v", err)
	}
	if _, err := h.chat.GetConversation(ctx, guest, conv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the guest should no longer own it: %v", err)
	}
	if _, err := h.cards.GetFlashcardSet(ctx, account, set.ID); err != nil {
		t.Fatalf("claimed flashcards: %v", err)
	}
	if _, err := h.chat.SendMessage(ctx, account, conv.ID, "More", SendOptions{}, nil); err != nil {
		t.Fatalf("an account has no message limit: %v", err)
	}
}

func TestStudyItemsFromGeneralKnowledge(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)

	if _, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no document and no topic: expected ErrInvalidInput, got %v", err)
	}
	h.llm.json = quizJSON
	quiz, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{Topic: "cell biology"})
	if err != nil {
		t.Fatal(err)
	}
	if quiz.DocumentID != "" || quiz.TotalQuestions != 2 {
		t.Fatalf("general-knowledge quiz: %+v", quiz)
	}
	for _, q := range quiz.AllQuestions {
		if q.CitationPage != nil {
			t.Fatalf("without a document no page may be cited: %+v", q)
		}
	}
	if !contains(h.llm.lastJSONPrompt, "general knowledge") {
		t.Fatal("the prompt should ask for general knowledge when there is no document")
	}
}
