package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAlreadyAnswered = errors.New("question has already been answered")

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
	DocumentID     string `json:"document_id"`
	ConversationID string `json:"conversation_id"`
	Topic          string `json:"topic"`
	Difficulty     string `json:"difficulty"` // "easy", "medium", "hard"
	NumQuestions   int    `json:"num_questions"`
}

type generatedQuestion struct {
	Question        string   `json:"question"`
	Options         []string `json:"options"`
	CorrectOption   int      `json:"correct_option"`
	Explanation     string   `json:"explanation"`
	CitationPage    int      `json:"citation_page"`
	CitationExcerpt string   `json:"citation_excerpt"`
}

type QuizQuestionDTO struct {
	ID              string   `json:"id"`
	QuestionOrder   int      `json:"question_order"`
	QuestionText    string   `json:"question_text"`
	Options         []string `json:"options"`
	IsAnswered      bool     `json:"is_answered"`
	SelectedOption  *int     `json:"selected_option,omitempty"`
	IsCorrect       bool     `json:"is_correct,omitempty"`
	CorrectOption   *int     `json:"correct_option,omitempty"` // hidden until answered
	Explanation     string   `json:"explanation,omitempty"`    // hidden until answered
	CitationPage    *int     `json:"citation_page,omitempty"`
	CitationExcerpt string   `json:"citation_excerpt,omitempty"`
}

type QuizSessionResponse struct {
	SessionID       string            `json:"session_id"`
	DocumentID      string            `json:"document_id"`
	Topic           string            `json:"topic"`
	Difficulty      string            `json:"difficulty"`
	TotalQuestions  int               `json:"total_questions"`
	CurrentIndex    int               `json:"current_index"`
	Score           int               `json:"score"`
	Completed       bool              `json:"completed"`
	CurrentQuestion *QuizQuestionDTO  `json:"current_question,omitempty"`
	AllQuestions    []QuizQuestionDTO `json:"all_questions,omitempty"`
}

func (s *QuizService) GenerateQuiz(ctx context.Context, userID string, req GenerateQuizRequest) (*QuizSessionResponse, error) {
	if _, err := s.docs.GetReady(ctx, userID, req.DocumentID); err != nil {
		return nil, err
	}
	if req.NumQuestions <= 0 || req.NumQuestions > 20 {
		req.NumQuestions = 5
	}
	switch req.Difficulty {
	case "easy", "medium", "hard":
	default:
		req.Difficulty = "medium"
	}

	convID, err := ownedConversationID(ctx, s.db, userID, req.DocumentID, req.ConversationID)
	if err != nil {
		return nil, err
	}
	excerpts, err := loadExcerpts(ctx, s.db, s.ragEngine, userID, req.DocumentID, req.Topic, 12)
	if err != nil {
		return nil, err
	}
	pages := excerptPages(excerpts)

	prompt := fmt.Sprintf(`Generate an interactive study quiz with %d multiple-choice questions based strictly on the document excerpts below.
Topic: %s
Difficulty: %s

CRITICAL GROUNDING RULES:
1. Every question MUST be answerable from the excerpts.
2. Provide exactly 4 options per question.
3. correct_option must be an integer index from 0 to 3 (0=A, 1=B, 2=C, 3=D).
4. citation_page must be the page number shown in the excerpt that contains the answer.
5. Provide a helpful explanation of why the correct option is right and the others are wrong.
6. Output MUST be a JSON object matching this schema:
{
  "questions": [
    {
      "question": "Question text here?",
      "options": ["Option A", "Option B", "Option C", "Option D"],
      "correct_option": 1,
      "explanation": "Explanation here",
      "citation_page": 4,
      "citation_excerpt": "Relevant sentence from text"
    }
  ]
}

EXCERPTS:
%s`, req.NumQuestions, req.Topic, req.Difficulty, formatExcerpts(excerpts))

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
		if g.Question == "" || len(g.Options) != 4 || g.CorrectOption < 0 || g.CorrectOption > 3 {
			continue // skip malformed questions rather than store something ungradeable
		}
		opts, err := json.Marshal(g.Options)
		if err != nil {
			return nil, err
		}
		correct := g.CorrectOption
		var excerpt *string
		if g.CitationExcerpt != "" {
			excerpt = &g.CitationExcerpt
		}
		questions = append(questions, models.QuizQuestion{
			ID:              uuid.NewString(),
			QuestionOrder:   len(questions),
			QuestionType:    "mcq",
			QuestionText:    g.Question,
			OptionsJSON:     string(opts),
			CorrectOption:   &correct,
			Explanation:     g.Explanation,
			CitationPage:    verifiedPage(g.CitationPage, pages),
			CitationExcerpt: excerpt,
		})
	}
	if len(questions) == 0 {
		return nil, errors.New("the AI did not return any usable questions, please try again")
	}

	session := &models.QuizSession{
		ID:             uuid.NewString(),
		UserID:         userID,
		DocumentID:     req.DocumentID,
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
		DocumentID:     session.DocumentID,
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
		QuestionText:  q.QuestionText,
		Options:       opts,
	}
	if q.Answer != nil {
		dto.IsAnswered = true
		dto.SelectedOption = q.Answer.SelectedOption
		dto.IsCorrect = q.Answer.IsCorrect
		dto.CorrectOption = q.CorrectOption
		dto.Explanation = q.Explanation
		dto.CitationPage = q.CitationPage
		if q.CitationExcerpt != nil {
			dto.CitationExcerpt = *q.CitationExcerpt
		}
	}
	return dto, nil
}

type AnswerSubmissionRequest struct {
	QuestionID     string `json:"question_id"`
	SelectedOption int    `json:"selected_option"`
}

type AnswerFeedbackResponse struct {
	QuestionID      string           `json:"question_id"`
	IsCorrect       bool             `json:"is_correct"`
	CorrectOption   *int             `json:"correct_option"`
	SelectedOption  int              `json:"selected_option"`
	Explanation     string           `json:"explanation"`
	CitationPage    *int             `json:"citation_page,omitempty"`
	CitationExcerpt string           `json:"citation_excerpt,omitempty"`
	CurrentScore    int              `json:"current_score"`
	TotalQuestions  int              `json:"total_questions"`
	CurrentIndex    int              `json:"current_index"`
	Completed       bool             `json:"completed"`
	NextQuestion    *QuizQuestionDTO `json:"next_question,omitempty"`
}

// SubmitAnswer grades the current question on the server and advances the quiz in one transaction.
func (s *QuizService) SubmitAnswer(ctx context.Context, userID, sessionID string, req AnswerSubmissionRequest) (*AnswerFeedbackResponse, error) {
	var feedback *AnswerFeedbackResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, err := s.loadSession(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), userID, sessionID)
		if err != nil {
			return err
		}
		if session.Completed || session.CurrentIndex >= len(session.Questions) {
			return ErrAlreadyAnswered
		}
		question := session.Questions[session.CurrentIndex]
		if question.ID != req.QuestionID {
			return fmt.Errorf("%w: only the current question can be answered", ErrInvalidInput)
		}
		if question.Answer != nil {
			return ErrAlreadyAnswered
		}
		var opts []string
		if err := json.Unmarshal([]byte(question.OptionsJSON), &opts); err != nil {
			return err
		}
		if req.SelectedOption < 0 || req.SelectedOption >= len(opts) {
			return fmt.Errorf("%w: selected_option must be between 0 and %d", ErrInvalidInput, len(opts)-1)
		}

		selected := req.SelectedOption
		isCorrect := question.CorrectOption != nil && *question.CorrectOption == selected
		if err := tx.Create(&models.QuizAnswer{
			ID:             uuid.NewString(),
			QuestionID:     question.ID,
			SessionID:      session.ID,
			UserID:         userID,
			SelectedOption: &selected,
			IsCorrect:      isCorrect,
			AnsweredAt:     time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("saving answer: %w", err)
		}

		if isCorrect {
			session.Score++
		}
		session.CurrentIndex++
		session.Completed = session.CurrentIndex >= session.TotalQuestions
		if err := tx.Model(&models.QuizSession{}).Where("id = ?", session.ID).Updates(map[string]any{
			"score":         session.Score,
			"current_index": session.CurrentIndex,
			"completed":     session.Completed,
			"updated_at":    time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("updating quiz progress: %w", err)
		}

		feedback = &AnswerFeedbackResponse{
			QuestionID:     question.ID,
			IsCorrect:      isCorrect,
			CorrectOption:  question.CorrectOption,
			SelectedOption: selected,
			Explanation:    question.Explanation,
			CitationPage:   question.CitationPage,
			CurrentScore:   session.Score,
			TotalQuestions: session.TotalQuestions,
			CurrentIndex:   session.CurrentIndex,
			Completed:      session.Completed,
		}
		if question.CitationExcerpt != nil {
			feedback.CitationExcerpt = *question.CitationExcerpt
		}
		if !session.Completed {
			next, err := questionDTO(session.Questions[session.CurrentIndex])
			if err != nil {
				return err
			}
			feedback.NextQuestion = &next
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return feedback, nil
}
