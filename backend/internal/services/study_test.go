package services

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestChatCitesOnlyRetrievedPages(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)
	conv, err := h.chat.CreateConversation(ctx, user, doc.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	question := "Robert Hooke first described cells in 1665 observing cork"
	retrieved, err := h.engine.Retrieve(ctx, question, user, doc.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	var retrievedPages []int
	for _, r := range retrieved {
		retrievedPages = append(retrievedPages, r.PageNumber)
	}
	if !slices.Contains(retrievedPages, 2) {
		t.Fatalf("expected page 2 to be retrieved, got %v", retrievedPages)
	}

	h.llm.answer = "Robert Hooke described cells in 1665 【Page 2】. Unrelated claim [Page 99]."
	result, err := h.chat.SendMessage(ctx, user, conv.ID, question, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Citations) != 1 || result.Citations[0].PageNumber != 2 {
		t.Fatalf("expected only page 2 cited, got %+v", result.Citations)
	}
	if result.AssistantMessage.Content != "Robert Hooke described cells in 1665 [Page 2]. Unrelated claim [Page 99]." {
		t.Fatalf("saved answer should have normalized brackets, got %q", result.AssistantMessage.Content)
	}

	h.llm.answer = "The document does not say."
	var streamed []string
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "What is the capital of Australia?", func(tok string) error {
		streamed = append(streamed, tok)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Citations) != 0 || len(streamed) < 2 {
		t.Fatalf("uncited streamed answer: citations=%d tokens=%d", len(result.Citations), len(streamed))
	}

	got, err := h.chat.GetConversation(ctx, user, conv.ID)
	if err != nil || len(got.Messages) != 4 {
		t.Fatalf("expected 4 stored messages, got %d (err %v)", len(got.Messages), err)
	}

	h.llm.err = errors.New("groq returned status 503")
	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "Another question", nil); err == nil {
		t.Fatal("an LLM failure must surface as an error")
	}

	other := createUser(t, h.db)
	if _, err := h.chat.SendMessage(ctx, other, conv.ID, "hi", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's conversation: expected ErrNotFound, got %v", err)
	}
}

const quizJSON = `{"questions": [
  {"question": "Who described cells?", "options": ["Hooke", "Darwin", "Mendel", "Crick"], "correct_option": 0,
   "explanation": "Hooke, 1665.", "citation_page": 2, "citation_excerpt": "Robert Hooke first described cells"},
  {"question": "What does DNA pair with?", "options": ["A-T", "A-G", "C-T", "G-A"], "correct_option": 0,
   "explanation": "Base pairing.", "citation_page": 99},
  {"question": "Malformed", "options": ["only", "three", "options"], "correct_option": 0}
]}`

func TestQuizFlow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)

	h.llm.json = "not json"
	if _, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{DocumentID: doc.ID}); err == nil {
		t.Fatal("unreadable LLM output must fail instead of inventing questions")
	}

	h.llm.json = quizJSON
	quiz, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{DocumentID: doc.ID, NumQuestions: 3})
	if err != nil {
		t.Fatal(err)
	}
	if quiz.TotalQuestions != 2 {
		t.Fatalf("malformed question should be skipped, got %d questions", quiz.TotalQuestions)
	}
	q1 := quiz.CurrentQuestion
	if q1 == nil || q1.CorrectOption != nil || q1.Explanation != "" || q1.CitationPage != nil {
		t.Fatalf("answer details must stay hidden before submitting: %+v", q1)
	}

	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: 7}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("out-of-range option: expected ErrInvalidInput, got %v", err)
	}
	other := createUser(t, h.db)
	if _, err := h.quiz.SubmitAnswer(ctx, other, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's quiz: expected ErrNotFound, got %v", err)
	}

	fb, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !fb.IsCorrect || fb.CurrentScore != 1 || fb.CitationPage == nil || *fb.CitationPage != 2 || fb.NextQuestion == nil {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
	if fb.NextQuestion.CorrectOption != nil {
		t.Fatal("next question must not reveal its answer")
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: 1}); err == nil {
		t.Fatal("answering a question twice must fail")
	}

	fb, err = h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: fb.NextQuestion.ID, SelectedOption: 2})
	if err != nil {
		t.Fatal(err)
	}
	if fb.IsCorrect || !fb.Completed || fb.CitationPage != nil {
		t.Fatalf("second answer: want wrong, completed and the invented page 99 dropped, got %+v", fb)
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: fb.QuestionID}); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("finished quiz: expected ErrAlreadyAnswered, got %v", err)
	}

	resumed, err := h.quiz.GetSession(ctx, user, quiz.SessionID)
	if err != nil || !resumed.Completed || resumed.Score != 1 || resumed.AllQuestions[0].CorrectOption == nil {
		t.Fatalf("resumed session: %+v (err %v)", resumed, err)
	}
}

func TestQuizRejectsForeignConversation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	alice, bob := createUser(t, h.db), createUser(t, h.db)
	aliceDoc, bobDoc := h.uploadReady(t, alice), h.uploadReady(t, bob)
	bobConv, err := h.chat.CreateConversation(ctx, bob, bobDoc.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	h.llm.json = quizJSON
	_, err = h.quiz.GenerateQuiz(ctx, alice, GenerateQuizRequest{DocumentID: aliceDoc.ID, ConversationID: bobConv.ID})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("linking another user's conversation: expected ErrNotFound, got %v", err)
	}
}

func TestFlashcards(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)

	h.llm.json = `{"cards": [
	  {"front": "What is the powerhouse of the cell?", "back": "Mitochondria", "citation_page": 4},
	  {"front": "Who wrote On the Origin of Species?", "back": "Darwin", "citation_page": 99},
	  {"front": "", "back": "empty front is skipped", "citation_page": 1}
	]}`
	set, err := h.cards.GenerateFlashcards(ctx, user, GenerateFlashcardsRequest{DocumentID: doc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(set.Cards))
	}
	if set.Cards[0].CitationPage == nil || *set.Cards[0].CitationPage != 4 || set.Cards[1].CitationPage != nil {
		t.Fatal("card pages must be kept only when they were among the excerpts")
	}

	cardID := set.Cards[0].ID
	if err := h.cards.UpdateCardStatus(ctx, user, cardID, "mastered"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid status: expected ErrInvalidInput, got %v", err)
	}
	other := createUser(t, h.db)
	if err := h.cards.UpdateCardStatus(ctx, other, cardID, "known"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's card: expected ErrNotFound, got %v", err)
	}
	if err := h.cards.UpdateCardStatus(ctx, user, cardID, "known"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.cards.GetFlashcardSet(ctx, other, set.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's set: expected ErrNotFound, got %v", err)
	}
}
