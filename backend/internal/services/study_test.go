package services

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
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
	result, err := h.chat.SendMessage(ctx, user, conv.ID, question, SendOptions{}, nil)
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
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "What is the capital of Australia?", SendOptions{}, func(tok string) error {
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
	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "Another question", SendOptions{}, nil); err == nil {
		t.Fatal("an LLM failure must surface as an error")
	}
	h.llm.err = nil

	other := createUser(t, h.db)
	if _, err := h.chat.SendMessage(ctx, other, conv.ID, "hi", SendOptions{}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's conversation: expected ErrNotFound, got %v", err)
	}
}

func TestChatFollowUpUsesRewrittenQueryAndStyle(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)
	conv, err := h.chat.CreateConversation(ctx, user, doc.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	// The follow-up alone says nothing about cells; only the rewritten query can retrieve page 2.
	h.llm.route = `{"intent": "question", "search_query": "Robert Hooke first described cells in 1665 observing cork", "style": "simple"}`
	h.llm.answer = "Hooke looked at cork and saw tiny boxes he called cells [Page 2]."
	result, err := h.chat.SendMessage(ctx, user, conv.ID, "Explain that more simply", SendOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Citations) != 1 || result.Citations[0].PageNumber != 2 {
		t.Fatalf("rewritten query should have retrieved page 2, got %+v", result.Citations)
	}
	if !strings.Contains(h.llm.lastSystem, rag.ExplanationStyles["simple"]) {
		t.Fatal("detected style should reach the prompt")
	}

	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "Explain again", SendOptions{Style: "exam"}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.llm.lastSystem, rag.ExplanationStyles["exam"]) {
		t.Fatal("a requested style should override the detected one")
	}
	if _, err := h.chat.SendMessage(ctx, user, conv.ID, "x", SendOptions{Style: "poetic"}, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown style: expected ErrInvalidInput, got %v", err)
	}

	h.llm.route = "this is not json"
	h.llm.answer = "Still answered."
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "What is a cell?", SendOptions{}, nil)
	if err != nil || result.AssistantMessage.Type != "chat" {
		t.Fatalf("a router failure should fall back to answering the question: %v", err)
	}
}

func TestChatCreatesStudyItems(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)
	conv, err := h.chat.CreateConversation(ctx, user, doc.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	metadata := func(m *models.Message) map[string]string {
		var out map[string]string
		if err := json.Unmarshal([]byte(m.MetadataJSON), &out); err != nil {
			t.Fatalf("message metadata: %v", err)
		}
		return out
	}

	h.llm.route = `{"intent": "quiz", "topic": "cells", "num_questions": 2}`
	h.llm.json = quizJSON
	var streamed []string
	result, err := h.chat.SendMessage(ctx, user, conv.ID, "Test my understanding", SendOptions{}, func(tok string) error {
		streamed = append(streamed, tok)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := result.AssistantMessage
	if msg.Type != "quiz" || len(streamed) != 1 || !strings.Contains(msg.Content, "cells") {
		t.Fatalf("expected a streamed quiz message about cells, got type=%s tokens=%d %q", msg.Type, len(streamed), msg.Content)
	}
	quiz, err := h.quiz.GetSession(ctx, user, metadata(msg)["quiz_session_id"])
	if err != nil || quiz.TotalQuestions != 2 {
		t.Fatalf("quiz from chat: %+v (err %v)", quiz, err)
	}
	var linked models.QuizSession
	h.db.First(&linked, "id = ?", quiz.SessionID)
	if linked.ConversationID == nil || *linked.ConversationID != conv.ID {
		t.Fatal("quiz should be linked to the conversation")
	}

	h.llm.route = `{"intent": "flashcards", "count": 2}`
	h.llm.json = `{"cards": [{"front": "Powerhouse of the cell?", "back": "Mitochondria", "citation_page": 4}]}`
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "Make flashcards", SendOptions{}, nil)
	if err != nil || result.AssistantMessage.Type != "flashcards" {
		t.Fatalf("flashcards from chat: %v", err)
	}
	if _, err := h.cards.GetFlashcardSet(ctx, user, metadata(result.AssistantMessage)["flashcard_set_id"]); err != nil {
		t.Fatalf("flashcard set from chat: %v", err)
	}

	h.llm.route = `{"intent": "summary", "summary_type": "revision"}`
	h.llm.answer = "## Revision notes\n- Cells were first described by Hooke [Page 2]."
	result, err = h.chat.SendMessage(ctx, user, conv.ID, "Give me revision notes", SendOptions{}, nil)
	if err != nil || result.AssistantMessage.Type != "summary" {
		t.Fatalf("summary from chat: %v", err)
	}
	if len(result.Citations) != 1 || result.Citations[0].PageNumber != 2 || metadata(result.AssistantMessage)["summary_type"] != "revision" {
		t.Fatalf("summary citations/metadata: %+v %s", result.Citations, result.AssistantMessage.MetadataJSON)
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
	if q1 == nil || q1.QuestionType != "mcq" || q1.CorrectOption != nil || q1.Explanation != "" || q1.CitationPage != nil {
		t.Fatalf("answer details must stay hidden before submitting: %+v", q1)
	}

	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: intPtr(7)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("out-of-range option: expected ErrInvalidInput, got %v", err)
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing option: expected ErrInvalidInput, got %v", err)
	}
	other := createUser(t, h.db)
	if _, err := h.quiz.SubmitAnswer(ctx, other, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: intPtr(0)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's quiz: expected ErrNotFound, got %v", err)
	}

	fb, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: intPtr(0)})
	if err != nil {
		t.Fatal(err)
	}
	if !fb.IsCorrect || fb.CurrentScore != 1 || fb.CitationPage == nil || *fb.CitationPage != 2 || fb.NextQuestion == nil {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
	if fb.NextQuestion.CorrectOption != nil {
		t.Fatal("next question must not reveal its answer")
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: q1.ID, SelectedOption: intPtr(1)}); err == nil {
		t.Fatal("answering a question twice must fail")
	}

	fb, err = h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: fb.NextQuestion.ID, SelectedOption: intPtr(2)})
	if err != nil {
		t.Fatal(err)
	}
	if fb.IsCorrect || !fb.Completed || fb.CitationPage != nil {
		t.Fatalf("second answer: want wrong, completed and the invented page 99 dropped, got %+v", fb)
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: fb.QuestionID, SelectedOption: intPtr(0)}); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("finished quiz: expected ErrAlreadyAnswered, got %v", err)
	}

	resumed, err := h.quiz.GetSession(ctx, user, quiz.SessionID)
	if err != nil || !resumed.Completed || resumed.Score != 1 || resumed.AllQuestions[0].CorrectOption == nil {
		t.Fatalf("resumed session: %+v (err %v)", resumed, err)
	}
}

func TestQuizTrueFalseAndShortAnswer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)

	if _, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{DocumentID: doc.ID, QuestionTypes: []string{"essay"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown question type: expected ErrInvalidInput, got %v", err)
	}

	h.llm.json = `{"questions": [
	  {"type": "true_false", "question": "Mitochondria produce ATP.", "options": ["Yes", "No"], "correct_option": 0,
	   "explanation": "They do.", "citation_page": 4},
	  {"type": "short_answer", "question": "What does cell theory state?", "options": ["ignored"], "correct_option": 2,
	   "reference_answer": "All living things are made of cells; the cell is the basic unit of life.", "explanation": "Cell theory.", "citation_page": 2},
	  {"type": "mcq", "question": "Not an allowed type here", "options": ["a", "b", "c", "d"], "correct_option": 0},
	  {"type": "short_answer", "question": "No reference answer", "reference_answer": ""}
	]}`
	quiz, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{DocumentID: doc.ID, QuestionTypes: []string{"true_false", "short_answer"}})
	if err != nil {
		t.Fatal(err)
	}
	if quiz.TotalQuestions != 2 {
		t.Fatalf("only the valid true/false and short-answer questions should be kept, got %d", quiz.TotalQuestions)
	}
	tf := quiz.AllQuestions[0]
	if tf.QuestionType != "true_false" || !slices.Equal(tf.Options, []string{"True", "False"}) {
		t.Fatalf("true/false options should be normalized: %+v", tf)
	}
	fb, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: tf.ID, SelectedOption: intPtr(0)})
	if err != nil || !fb.IsCorrect {
		t.Fatalf("true/false answer: %+v (err %v)", fb, err)
	}

	short := fb.NextQuestion
	if short == nil || short.QuestionType != "short_answer" || len(short.Options) != 0 || short.ReferenceAnswer != "" {
		t.Fatalf("short-answer question must not reveal its reference answer: %+v", short)
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: short.ID, AnswerText: "   "}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty short answer: expected ErrInvalidInput, got %v", err)
	}

	h.llm.grade = "not json"
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: short.ID, AnswerText: "Everything is cells"}); err == nil {
		t.Fatal("an ungradeable answer must fail, not be marked right or wrong")
	}

	h.llm.grade = `{"is_correct": true, "feedback": "Right: the cell is the basic unit of life."}`
	fb, err = h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: short.ID, AnswerText: "Living things are built from cells"})
	if err != nil {
		t.Fatal(err)
	}
	if !fb.IsCorrect || fb.Feedback == "" || fb.ReferenceAnswer == "" || !fb.Completed || fb.CurrentScore != 2 {
		t.Fatalf("graded short answer: %+v", fb)
	}
	resumed, err := h.quiz.GetSession(ctx, user, quiz.SessionID)
	if err != nil || resumed.AllQuestions[1].AnswerText == "" || resumed.AllQuestions[1].Feedback == "" {
		t.Fatalf("short answer and feedback should be stored: %+v (err %v)", resumed, err)
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

func TestProgressFindsWeakAreas(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)

	empty, err := h.progress.Get(ctx, user, doc.ID)
	if err != nil || empty.QuizzesCompleted != 0 || len(empty.Recommendations) == 0 {
		t.Fatalf("fresh document progress: %+v (err %v)", empty, err)
	}

	h.llm.json = `{"questions": [
	  {"question": "Who described cells?", "options": ["Hooke", "Darwin", "Mendel", "Crick"], "correct_option": 0, "citation_page": 2},
	  {"question": "Where is ATP made?", "options": ["Mitochondria", "Ribosome", "Nucleus", "Wall"], "correct_option": 0, "citation_page": 4}
	]}`
	quiz, err := h.quiz.GenerateQuiz(ctx, user, GenerateQuizRequest{DocumentID: doc.ID, Topic: "cells"})
	if err != nil {
		t.Fatal(err)
	}
	fb, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: quiz.CurrentQuestion.ID, SelectedOption: intPtr(1)}) // wrong
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.quiz.SubmitAnswer(ctx, user, quiz.SessionID, AnswerSubmissionRequest{QuestionID: fb.NextQuestion.ID, SelectedOption: intPtr(0)}); err != nil {
		t.Fatal(err)
	}

	h.llm.json = `{"cards": [{"front": "Q", "back": "A", "citation_page": 2}, {"front": "Q2", "back": "A2", "citation_page": 3}]}`
	set, err := h.cards.GenerateFlashcards(ctx, user, GenerateFlashcardsRequest{DocumentID: doc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.cards.UpdateCardStatus(ctx, user, set.Cards[0].ID, "practice"); err != nil {
		t.Fatal(err)
	}

	p, err := h.progress.Get(ctx, user, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.QuizzesCompleted != 1 || p.AverageScorePct != 50 || p.QuestionsAnswered != 2 || p.CorrectAnswers != 1 {
		t.Fatalf("quiz stats: %+v", p)
	}
	if len(p.WeakAreas) != 1 || p.WeakAreas[0].Page != 2 || p.WeakAreas[0].Missed != 1 {
		t.Fatalf("weak areas should be page 2 only: %+v", p.WeakAreas)
	}
	if !slices.Equal(p.WeakTopics, []string{"cells"}) {
		t.Fatalf("weak topics: %v", p.WeakTopics)
	}
	if p.Flashcards != (FlashcardProgress{Total: 2, Practice: 1, New: 1}) {
		t.Fatalf("flashcard progress: %+v", p.Flashcards)
	}
	if !strings.Contains(strings.Join(p.Recommendations, " "), "page 2") {
		t.Fatalf("recommendations should point at page 2: %v", p.Recommendations)
	}

	other := createUser(t, h.db)
	if _, err := h.progress.Get(ctx, other, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's progress: expected ErrNotFound, got %v", err)
	}
}

func TestSummariesAreHierarchicalAndCached(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	user := createUser(t, h.db)
	doc := h.uploadReady(t, user)
	h.llm.answer = "## Notes\n- Hooke described cells [Page 2]. Invented [Page 99]."

	sum, err := h.summaries.GenerateSummary(ctx, user, GenerateSummaryRequest{DocumentID: doc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if h.llm.generateCalls != 1 || sum.Cached || sum.Type != "complete" {
		t.Fatalf("a short document should be summarized in one call: calls=%d %+v", h.llm.generateCalls, sum)
	}
	if len(sum.Citations) != 1 || sum.Citations[0].PageNumber != 2 {
		t.Fatalf("only real cited pages should be kept: %+v", sum.Citations)
	}

	again, err := h.summaries.GenerateSummary(ctx, user, GenerateSummaryRequest{DocumentID: doc.ID})
	if err != nil || !again.Cached || h.llm.generateCalls != 1 || again.Summary != sum.Summary {
		t.Fatalf("second request should come from the cache: calls=%d %+v (err %v)", h.llm.generateCalls, again, err)
	}
	if _, err := h.summaries.GenerateSummary(ctx, user, GenerateSummaryRequest{DocumentID: doc.ID, Refresh: true}); err != nil || h.llm.generateCalls != 2 {
		t.Fatalf("refresh should regenerate: calls=%d (err %v)", h.llm.generateCalls, err)
	}

	// Force several sections so the whole document is covered piece by piece, then combined.
	h.summaries.sectionChars = 800
	h.llm.generateCalls, h.llm.generatePrompt = 0, nil
	if _, err := h.summaries.GenerateSummary(ctx, user, GenerateSummaryRequest{DocumentID: doc.ID, Type: "revision"}); err != nil {
		t.Fatal(err)
	}
	if h.llm.generateCalls < 3 {
		t.Fatalf("expected section calls plus a combining call, got %d", h.llm.generateCalls)
	}
	final := h.llm.generatePrompt[len(h.llm.generatePrompt)-1]
	if !strings.Contains(final, "SECTION 1") || !strings.Contains(final, "revision notes") {
		t.Fatal("the final call should combine the section notes using the revision instruction")
	}
	if !strings.Contains(h.llm.generatePrompt[0], "pages 1-") {
		t.Fatalf("the first section should start at page 1: %q", h.llm.generatePrompt[0][:80])
	}

	other := createUser(t, h.db)
	if _, err := h.summaries.GenerateSummary(ctx, other, GenerateSummaryRequest{DocumentID: doc.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's summary: expected ErrNotFound, got %v", err)
	}
}
