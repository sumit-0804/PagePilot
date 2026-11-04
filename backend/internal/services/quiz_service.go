package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAlreadyAnswered = errors.New("question has already been answered")

const (
	typeMCQ         = "mcq"
	typeTrueFalse   = "true_false"
	typeShortAnswer = "short_answer"

	maxAnswerChars = 2000
)

var questionTypes = []string{typeMCQ, typeTrueFalse, typeShortAnswer}

type QuizService struct {
	db        *gorm.DB
	docs      *DocumentService
	ragEngine *rag.Engine
	llmClient rag.LLMClient
}

func NewQuizService(db *gorm.DB, docs *DocumentService, ragEngine *rag.Engine, llm rag.LLMClient) *QuizService {
	return &QuizService{db: db, docs: docs, ragEngine: ragEngine, llmClient: llm}
}

type GenerateQuizRequest struct {
	DocumentID     string   `json:"document_id"`
	ConversationID string   `json:"conversation_id"`
	Topic          string   `json:"topic"`
	Difficulty     string   `json:"difficulty"` // "easy", "medium", "hard"
	NumQuestions   int      `json:"num_questions"`
	QuestionTypes  []string `json:"question_types"` // "mcq", "true_false", "short_answer"; default mcq
}

type generatedQuestion struct {
	Type            string   `json:"type"`
	Question        string   `json:"question"`
	Options         []string `json:"options"`
	CorrectOption   *int     `json:"correct_option"`
	ReferenceAnswer string   `json:"reference_answer"`
	Explanation     string   `json:"explanation"`
	CitationPage    int      `json:"citation_page"`
	CitationExcerpt string   `json:"citation_excerpt"`
}

type QuizQuestionDTO struct {
	ID              string   `json:"id"`
	QuestionOrder   int      `json:"question_order"`
	QuestionType    string   `json:"question_type"`
	QuestionText    string   `json:"question_text"`
	Options         []string `json:"options"`
	IsAnswered      bool     `json:"is_answered"`
	SelectedOption  *int     `json:"selected_option,omitempty"`
	AnswerText      string   `json:"answer_text,omitempty"`
	IsCorrect       bool     `json:"is_correct,omitempty"`
	CorrectOption   *int     `json:"correct_option,omitempty"`   // hidden until answered
	ReferenceAnswer string   `json:"reference_answer,omitempty"` // hidden until answered
	Explanation     string   `json:"explanation,omitempty"`      // hidden until answered
	Feedback        string   `json:"feedback,omitempty"`
	CitationPage    *int     `json:"citation_page,omitempty"`
	CitationExcerpt string   `json:"citation_excerpt,omitempty"`
}

type QuizSessionResponse struct {
	SessionID       string            `json:"session_id"`
	DocumentID      string            `json:"document_id,omitempty"`
	Topic           string            `json:"topic"`
	Difficulty      string            `json:"difficulty"`
	TotalQuestions  int               `json:"total_questions"`
	CurrentIndex    int               `json:"current_index"`
	Score           int               `json:"score"`
	Completed       bool              `json:"completed"`
	CurrentQuestion *QuizQuestionDTO  `json:"current_question,omitempty"`
	AllQuestions    []QuizQuestionDTO `json:"all_questions,omitempty"`
}

// GenerateQuiz builds a quiz from a document, or from general knowledge on a topic when DocumentID is empty.
func (s *QuizService) GenerateQuiz(ctx context.Context, userID string, req GenerateQuizRequest) (*QuizSessionResponse, error) {
	if req.NumQuestions <= 0 || req.NumQuestions > 20 {
		req.NumQuestions = 5
	}
	switch req.Difficulty {
	case "easy", "medium", "hard":
	default:
		req.Difficulty = "medium"
	}
	types := []string{typeMCQ}
	if len(req.QuestionTypes) > 0 {
		types = nil
		for _, t := range req.QuestionTypes {
			if !slices.Contains(questionTypes, t) {
				return nil, fmt.Errorf("%w: question_types may only contain %s", ErrInvalidInput, strings.Join(questionTypes, ", "))
			}
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}

	convID, err := ownedConversationID(ctx, s.db, userID, req.DocumentID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	excerpts, err := studySource(ctx, s.db, s.docs, s.ragEngine, userID, req.DocumentID, req.Topic, 12)
	if err != nil {
		return nil, err
	}
	pages := excerptPages(excerpts)

	prompt := fmt.Sprintf(`Generate an interactive study quiz with %d questions based strictly on the source below.
Topic: %s
Difficulty: %s
Allowed question types: %s (mix them if more than one is allowed)

RULES:
1. Every question MUST be answerable from the source.
2. "mcq": exactly 4 options; correct_option is the index 0-3.
3. "true_false": options must be ["True", "False"]; correct_option is 0 for True, 1 for False.
4. "short_answer": options is []; correct_option is null; reference_answer is a concise model answer from the source.
5. citation_page must be the page number shown in the excerpt that contains the answer.
6. explanation says why the answer is right (and, for mcq, why the other options are wrong). Refer to options by their wording, never by number or letter.
7. Output MUST be a JSON object matching this schema:
{
  "questions": [
    {
      "type": "mcq",
      "question": "Question text here?",
      "options": ["Option A", "Option B", "Option C", "Option D"],
      "correct_option": 1,
      "reference_answer": "",
      "explanation": "Explanation here",
      "citation_page": 4,
      "citation_excerpt": "Relevant sentence from text"
    }
  ]
}

%s`, req.NumQuestions, req.Topic, req.Difficulty, strings.Join(types, ", "), sourceBlock(excerpts))

	llmResp, err := s.llmClient.GenerateJSON(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are an expert exam creator. Respond ONLY with a JSON object."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("quiz generation failed: %w", err)
	}

	var generated struct {
		Questions []generatedQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(llmResp), &generated); err != nil {
		return nil, fmt.Errorf("the AI returned a quiz in an unreadable format, please try again: %w", err)
	}

	var questions []models.QuizQuestion
	for _, g := range generated.Questions {
		q, ok := buildQuestion(g, types, pages)
		if !ok {
			continue // skip malformed questions rather than store something ungradeable
		}
		q.QuestionOrder = len(questions)
		questions = append(questions, q)
	}
	if len(questions) == 0 {
		return nil, errors.New("the AI did not return any usable questions, please try again")
	}

	session := &models.QuizSession{
		ID:             uuid.NewString(),
		UserID:         userID,
		DocumentID:     optionalID(req.DocumentID),
		ConversationID: convID,
		Topic:          req.Topic,
		Difficulty:     req.Difficulty,
		TotalQuestions: len(questions),
	}
	for i := range questions {
		questions[i].SessionID = session.ID
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		return tx.Create(&questions).Error
	})
	if err != nil {
		return nil, fmt.Errorf("saving quiz: %w", err)
	}
	return s.GetSession(ctx, userID, session.ID)
}

// buildQuestion validates one generated question against its type's rules.
func buildQuestion(g generatedQuestion, allowed []string, pages map[int]bool) (models.QuizQuestion, bool) {
	if g.Type == "" {
		g.Type = typeMCQ
	}
	if strings.TrimSpace(g.Question) == "" || !slices.Contains(allowed, g.Type) {
		return models.QuizQuestion{}, false
	}

	q := models.QuizQuestion{
		ID:           uuid.NewString(),
		QuestionType: g.Type,
		QuestionText: g.Question,
		Explanation:  g.Explanation,
		CitationPage: verifiedPage(g.CitationPage, pages),
	}
	switch g.Type {
	case typeMCQ:
		if len(g.Options) != 4 || g.CorrectOption == nil || *g.CorrectOption < 0 || *g.CorrectOption > 3 {
			return models.QuizQuestion{}, false
		}
	case typeTrueFalse:
		if g.CorrectOption == nil || (*g.CorrectOption != 0 && *g.CorrectOption != 1) {
			return models.QuizQuestion{}, false
		}
		g.Options = []string{"True", "False"}
	case typeShortAnswer:
		if strings.TrimSpace(g.ReferenceAnswer) == "" {
			return models.QuizQuestion{}, false
		}
		g.Options, g.CorrectOption = []string{}, nil
		q.ReferenceAnswer = &g.ReferenceAnswer
	}

	opts, err := json.Marshal(g.Options)
	if err != nil {
		return models.QuizQuestion{}, false
	}
	q.OptionsJSON = string(opts)
	q.CorrectOption = g.CorrectOption
	if g.CitationExcerpt != "" {
		q.CitationExcerpt = &g.CitationExcerpt
	}
	return q, true
}

func (s *QuizService) loadSession(ctx context.Context, db *gorm.DB, userID, sessionID string) (*models.QuizSession, error) {
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrNotFound
	}
	var session models.QuizSession
	err := db.WithContext(ctx).
		Preload("Questions", func(db *gorm.DB) *gorm.DB { return db.Order("question_order ASC") }).
		Preload("Questions.Answer").
		First(&session, "id = ? AND user_id = ?", sessionID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *QuizService) GetSession(ctx context.Context, userID, sessionID string) (*QuizSessionResponse, error) {
	session, err := s.loadSession(ctx, s.db, userID, sessionID)
	if err != nil {
		return nil, err
	}

	resp := &QuizSessionResponse{
		SessionID:      session.ID,
		DocumentID:     derefString(session.DocumentID),
		Topic:          session.Topic,
		Difficulty:     session.Difficulty,
		TotalQuestions: session.TotalQuestions,
		CurrentIndex:   session.CurrentIndex,
		Score:          session.Score,
		Completed:      session.Completed,
	}
	for i, q := range session.Questions {
		dto, err := questionDTO(q)
		if err != nil {
			return nil, err
		}
		resp.AllQuestions = append(resp.AllQuestions, dto)
		if i == session.CurrentIndex && !session.Completed {
			resp.CurrentQuestion = &resp.AllQuestions[len(resp.AllQuestions)-1]
		}
	}
	return resp, nil
}

// questionDTO reveals the correct answer and explanation only after the question was answered.
func questionDTO(q models.QuizQuestion) (QuizQuestionDTO, error) {
	var opts []string
	if err := json.Unmarshal([]byte(q.OptionsJSON), &opts); err != nil {
		return QuizQuestionDTO{}, fmt.Errorf("reading options of question %s: %w", q.ID, err)
	}
	dto := QuizQuestionDTO{
		ID:            q.ID,
		QuestionOrder: q.QuestionOrder,
		QuestionType:  q.QuestionType,
		QuestionText:  q.QuestionText,
		Options:       opts,
	}
	if q.Answer != nil {
		dto.IsAnswered = true
		dto.SelectedOption = q.Answer.SelectedOption
		dto.AnswerText = deref(q.Answer.AnswerText)
		dto.Feedback = deref(q.Answer.Feedback)
		dto.IsCorrect = q.Answer.IsCorrect
		dto.CorrectOption = q.CorrectOption
		dto.ReferenceAnswer = deref(q.ReferenceAnswer)
		dto.Explanation = q.Explanation
		dto.CitationPage = q.CitationPage
		dto.CitationExcerpt = deref(q.CitationExcerpt)
	}
	return dto, nil
}

type AnswerSubmissionRequest struct {
	QuestionID     string `json:"question_id"`
	SelectedOption *int   `json:"selected_option"` // mcq and true_false
	AnswerText     string `json:"answer_text"`     // short_answer
}

type AnswerFeedbackResponse struct {
	QuestionID      string           `json:"question_id"`
	QuestionType    string           `json:"question_type"`
	IsCorrect       bool             `json:"is_correct"`
	CorrectOption   *int             `json:"correct_option"`
	SelectedOption  *int             `json:"selected_option,omitempty"`
	AnswerText      string           `json:"answer_text,omitempty"`
	ReferenceAnswer string           `json:"reference_answer,omitempty"`
	Feedback        string           `json:"feedback,omitempty"`
	Explanation     string           `json:"explanation"`
	CitationPage    *int             `json:"citation_page,omitempty"`
	CitationExcerpt string           `json:"citation_excerpt,omitempty"`
	CurrentScore    int              `json:"current_score"`
	TotalQuestions  int              `json:"total_questions"`
	CurrentIndex    int              `json:"current_index"`
	Completed       bool             `json:"completed"`
	NextQuestion    *QuizQuestionDTO `json:"next_question,omitempty"`
}

// SubmitAnswer grades the current question on the server and advances the quiz.
// Short answers are graded by the LLM first, outside the transaction, so no row stays locked meanwhile.
func (s *QuizService) SubmitAnswer(ctx context.Context, userID, sessionID string, req AnswerSubmissionRequest) (*AnswerFeedbackResponse, error) {
	session, err := s.loadSession(ctx, s.db, userID, sessionID)
	if err != nil {
		return nil, err
	}
	question, err := currentQuestion(session, req.QuestionID)
	if err != nil {
		return nil, err
	}

	answer := models.QuizAnswer{
		ID:         uuid.NewString(),
		QuestionID: question.ID,
		SessionID:  session.ID,
		UserID:     userID,
		AnsweredAt: time.Now(),
	}
	switch question.QuestionType {
	case typeShortAnswer:
		text := strings.TrimSpace(req.AnswerText)
		if text == "" || len(text) > maxAnswerChars {
			return nil, fmt.Errorf("%w: answer_text must be 1-%d characters", ErrInvalidInput, maxAnswerChars)
		}
		correct, feedback, err := s.gradeShortAnswer(ctx, question, text)
		if err != nil {
			return nil, err
		}
		answer.AnswerText, answer.IsCorrect, answer.Feedback = &text, correct, &feedback
	default:
		var opts []string
		if err := json.Unmarshal([]byte(question.OptionsJSON), &opts); err != nil {
			return nil, err
		}
		if req.SelectedOption == nil || *req.SelectedOption < 0 || *req.SelectedOption >= len(opts) {
			return nil, fmt.Errorf("%w: selected_option must be between 0 and %d", ErrInvalidInput, len(opts)-1)
		}
		answer.SelectedOption = req.SelectedOption
		answer.IsCorrect = question.CorrectOption != nil && *question.CorrectOption == *req.SelectedOption
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.loadSession(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), userID, sessionID)
		if err != nil {
			return err
		}
		// Another request may have answered while this one was grading.
		if _, err := currentQuestion(locked, question.ID); err != nil {
			return err
		}
		if err := tx.Create(&answer).Error; err != nil {
			return fmt.Errorf("saving answer: %w", err)
		}
		if answer.IsCorrect {
			locked.Score++
		}
		locked.CurrentIndex++
		locked.Completed = locked.CurrentIndex >= locked.TotalQuestions
		session = locked
		return tx.Model(&models.QuizSession{}).Where("id = ?", locked.ID).Updates(map[string]any{
			"score":         locked.Score,
			"current_index": locked.CurrentIndex,
			"completed":     locked.Completed,
			"updated_at":    time.Now(),
		}).Error
	})
	if err != nil {
		return nil, err
	}

	feedback := &AnswerFeedbackResponse{
		QuestionID:      question.ID,
		QuestionType:    question.QuestionType,
		IsCorrect:       answer.IsCorrect,
		CorrectOption:   question.CorrectOption,
		SelectedOption:  answer.SelectedOption,
		AnswerText:      deref(answer.AnswerText),
		ReferenceAnswer: deref(question.ReferenceAnswer),
		Feedback:        deref(answer.Feedback),
		Explanation:     question.Explanation,
		CitationPage:    question.CitationPage,
		CitationExcerpt: deref(question.CitationExcerpt),
		CurrentScore:    session.Score,
		TotalQuestions:  session.TotalQuestions,
		CurrentIndex:    session.CurrentIndex,
		Completed:       session.Completed,
	}
	if !session.Completed {
		next, err := questionDTO(session.Questions[session.CurrentIndex])
		if err != nil {
			return nil, err
		}
		feedback.NextQuestion = &next
	}
	return feedback, nil
}

// currentQuestion returns the session's current question if it is the one being answered.
func currentQuestion(session *models.QuizSession, questionID string) (models.QuizQuestion, error) {
	if session.Completed || session.CurrentIndex >= len(session.Questions) {
		return models.QuizQuestion{}, ErrAlreadyAnswered
	}
	q := session.Questions[session.CurrentIndex]
	if q.ID != questionID {
		return models.QuizQuestion{}, fmt.Errorf("%w: only the current question can be answered", ErrInvalidInput)
	}
	if q.Answer != nil {
		return models.QuizQuestion{}, ErrAlreadyAnswered
	}
	return q, nil
}

func (s *QuizService) gradeShortAnswer(ctx context.Context, q models.QuizQuestion, answer string) (bool, string, error) {
	prompt := fmt.Sprintf(`Grade the student's answer against the reference answer.
Mark it correct if it captures the key idea of the reference answer, even if worded differently; ignore spelling and grammar.
Give one or two sentences of feedback addressed to the student.

QUESTION: %s
REFERENCE ANSWER: %s
EXPLANATION: %s
STUDENT ANSWER: %s

Respond with a JSON object: {"is_correct": true, "feedback": "..."}`, q.QuestionText, deref(q.ReferenceAnswer), q.Explanation, answer)

	resp, err := s.llmClient.GenerateJSON(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are a fair answer grader for study quizzes. Respond ONLY with a JSON object."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return false, "", fmt.Errorf("grading failed: %w", err)
	}
	var graded struct {
		IsCorrect *bool  `json:"is_correct"`
		Feedback  string `json:"feedback"`
	}
	if err := json.Unmarshal([]byte(resp), &graded); err != nil || graded.IsCorrect == nil {
		return false, "", errors.New("the AI could not grade this answer, please try again")
	}
	return *graded.IsCorrect, graded.Feedback, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
