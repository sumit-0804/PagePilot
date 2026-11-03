package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

const weakQuizScorePct = 60

type ProgressService struct {
	db   *gorm.DB
	docs *DocumentService
}

func NewProgressService(db *gorm.DB, docs *DocumentService) *ProgressService {
	return &ProgressService{db: db, docs: docs}
}

// WeakArea is a page the user keeps missing quiz questions about.
type WeakArea struct {
	Page            int    `json:"page"`
	Missed          int    `json:"missed"`
	Attempted       int    `json:"attempted"`
	ExampleQuestion string `json:"example_question"`
}

type FlashcardProgress struct {
	Total    int `json:"total"`
	Known    int `json:"known"`
	Practice int `json:"practice"`
	New      int `json:"new"`
}

type ProgressResponse struct {
	DocumentID        string            `json:"document_id"`
	QuizzesCompleted  int               `json:"quizzes_completed"`
	QuizzesInProgress int               `json:"quizzes_in_progress"`
	QuestionsAnswered int               `json:"questions_answered"`
	CorrectAnswers    int               `json:"correct_answers"`
	AverageScorePct   float64           `json:"average_score_pct"` // over completed quizzes
	Flashcards        FlashcardProgress `json:"flashcards"`
	WeakAreas         []WeakArea        `json:"weak_areas"`
	WeakTopics        []string          `json:"weak_topics"`
	Recommendations   []string          `json:"recommendations"`
	LastStudiedAt     *time.Time        `json:"last_studied_at,omitempty"`
}

// Get computes progress from quiz answers and flashcards, so it can never drift from them.
func (s *ProgressService) Get(ctx context.Context, userID, docID string) (*ProgressResponse, error) {
	if _, err := s.docs.Get(ctx, userID, docID); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	resp := &ProgressResponse{DocumentID: docID, WeakAreas: []WeakArea{}, WeakTopics: []string{}}

	var sessions []struct {
		Topic          string
		Score          int
		TotalQuestions int
		Completed      bool
		UpdatedAt      time.Time
	}
	if err := db.Table("quiz_sessions").Select("topic, score, total_questions, completed, updated_at").
		Where("user_id = ? AND document_id = ?", userID, docID).Order("updated_at").Scan(&sessions).Error; err != nil {
		return nil, fmt.Errorf("loading quizzes: %w", err)
	}
	var scoreSum float64
	for _, q := range sessions {
		resp.LastStudiedAt = &q.UpdatedAt
		if !q.Completed {
			resp.QuizzesInProgress++
			continue
		}
		resp.QuizzesCompleted++
		pct := 100 * float64(q.Score) / float64(max(q.TotalQuestions, 1))
		scoreSum += pct
		if pct < weakQuizScorePct && q.Topic != "" {
			resp.WeakTopics = append(resp.WeakTopics, q.Topic)
		}
	}
	if resp.QuizzesCompleted > 0 {
		resp.AverageScorePct = math.Round(scoreSum/float64(resp.QuizzesCompleted)*10) / 10
	}

	var answered struct{ Total, Correct int }
	if err := db.Raw(`select count(*) as total, count(*) filter (where a.is_correct) as correct
		from quiz_answers a join quiz_sessions s on s.id = a.session_id
		where s.user_id = ? and s.document_id = ?`, userID, docID).Scan(&answered).Error; err != nil {
		return nil, fmt.Errorf("counting answers: %w", err)
	}
	resp.QuestionsAnswered, resp.CorrectAnswers = answered.Total, answered.Correct

	if err := db.Raw(`select q.citation_page as page,
			count(*) filter (where not a.is_correct) as missed,
			count(*) as attempted,
			min(q.question_text) filter (where not a.is_correct) as example_question
		from quiz_answers a
		join quiz_questions q on q.id = a.question_id
		join quiz_sessions s on s.id = a.session_id
		where s.user_id = ? and s.document_id = ? and q.citation_page is not null
		group by q.citation_page
		having count(*) filter (where not a.is_correct) > 0
		order by missed desc, page
		limit 5`, userID, docID).Scan(&resp.WeakAreas).Error; err != nil {
		return nil, fmt.Errorf("finding weak areas: %w", err)
	}

	var cards []struct {
		Status string
		Count  int
	}
	if err := db.Raw(`select f.status, count(*) as count from flashcards f
		join flashcard_sets fs on fs.id = f.set_id
		where fs.user_id = ? and fs.document_id = ? group by f.status`, userID, docID).Scan(&cards).Error; err != nil {
		return nil, fmt.Errorf("counting flashcards: %w", err)
	}
	for _, c := range cards {
		resp.Flashcards.Total += c.Count
		switch c.Status {
		case "known":
			resp.Flashcards.Known = c.Count
		case "practice":
			resp.Flashcards.Practice = c.Count
		case "new":
			resp.Flashcards.New = c.Count
		}
	}

	resp.Recommendations = recommend(resp)
	return resp, nil
}

func recommend(p *ProgressResponse) []string {
	recs := []string{}
	for _, w := range p.WeakAreas {
		recs = append(recs, fmt.Sprintf("Review page %d: you missed %d of %d questions about it.", w.Page, w.Missed, w.Attempted))
	}
	for _, t := range p.WeakTopics {
		recs = append(recs, fmt.Sprintf("Retake a quiz on %q, you scored under %d%% last time.", t, weakQuizScorePct))
	}
	if p.Flashcards.Practice > 0 {
		recs = append(recs, fmt.Sprintf("Practice the %d flashcards marked Need Practice.", p.Flashcards.Practice))
	}
	if p.QuizzesInProgress > 0 {
		recs = append(recs, fmt.Sprintf("Finish your %d unfinished quiz(zes).", p.QuizzesInProgress))
	}
	switch {
	case p.QuizzesCompleted == 0:
		recs = append(recs, "Take a quiz to check your understanding.")
	case p.AverageScorePct >= 80 && len(p.WeakAreas) == 0:
		recs = append(recs, "You're doing well: try a hard quiz or short-answer questions.")
	}
	if p.Flashcards.Total == 0 {
		recs = append(recs, "Create flashcards to practise active recall.")
	}
	return recs
}
