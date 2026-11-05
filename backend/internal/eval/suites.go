package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/services"
)

// Suites in run order. Retrieval runs first: it needs no LLM, so it still reports when Groq is down.
var SuiteNames = []string{"rag", "tools", "answer", "quiz", "flashcards"}

// minJudgeScore is the lowest judge score (1-5) a case may get on any criterion and still pass.
const minJudgeScore = 3

func (r *Runner) Run(ctx context.Context, suite string) (SuiteResult, error) {
	fmt.Fprintf(r.Out, "\n%-11s ", suite)
	switch suite {
	case "rag":
		return r.runRAG(ctx), nil
	case "tools":
		return r.runTools(ctx), nil
	case "answer":
		return r.runAnswers(ctx), nil
	case "quiz":
		return r.runQuiz(ctx), nil
	case "flashcards":
		return r.runFlashcards(ctx), nil
	}
	return SuiteResult{}, fmt.Errorf("unknown suite %q (choose from %s)", suite, strings.Join(SuiteNames, ", "))
}

// Retrieval: the real RetrieveMany over every corpus document, scored against evidence strings.

func (r *Runner) runRAG(ctx context.Context) SuiteResult {
	cases := r.runCases(len(r.Data.RAG), func(i int) CaseResult {
		c := r.Data.RAG[i]
		return r.traced(ctx, "rag", c.ID, []string{"recall_at_k", "mrr"}, func(ctx context.Context, _ string) CaseResult {
			// Evidence split across two chunks can never be retrieved whole; that is a dataset problem.
			for _, e := range c.Evidence {
				if !slices.ContainsFunc(r.chunks[c.Doc], func(chunk string) bool { return Contains(chunk, e) }) {
					return CaseResult{Outcome: Error, Error: fmt.Sprintf("dataset: evidence %q is not inside a single indexed chunk", e)}
				}
			}
			results, err := r.RAG.RetrieveMany(ctx, c.Question, r.user, r.allDocIDs(), r.TopK)
			if err != nil {
				return ErrorResult(c.ID, "", err)
			}
			texts := make([]string, len(results))
			for i, res := range results {
				texts[i] = res.ChunkText
			}
			s := ScoreRetrieval(texts, c.Evidence, r.TopK)
			topDoc := 0.0
			if len(results) > 0 && results[0].DocumentID == r.docIDs[c.Doc] {
				topDoc = 1
			}
			res := CaseResult{Outcome: Pass, Metrics: map[string]float64{
				"recall_at_k": s.RecallAtK, "precision_at_k": s.PrecisionAtK, "mrr": s.MRR, "ndcg_at_k": s.NDCGAtK,
				"hit_at_k": boolf(s.FirstHitRank > 0), "top1_doc": topDoc,
			}}
			if s.FirstHitRank == 0 {
				res.Outcome = Fail
				res.Problems = []string{fmt.Sprintf("no chunk with the evidence in the top %d", r.TopK)}
			}
			return res
		})
	})
	s := NewSuite("rag", cases)
	for _, m := range []string{"recall_at_k", "precision_at_k", "mrr", "ndcg_at_k", "hit_at_k", "top1_doc"} {
		if v, ok := MeanMetric(cases, m); ok {
			s.Aggregates[m] = v
		}
	}
	s.Notes = append(s.Notes, fmt.Sprintf("k = %d, searching all %d corpus documents at once", r.TopK, len(r.docIDs)))
	return s
}

// Tool selection: the real ADK agent and model; tool calls are read from its execute_tool spans.

func (r *Runner) runTools(ctx context.Context) SuiteResult {
	cases := r.runCases(len(r.Data.Tools), func(i int) CaseResult {
		c := r.Data.Tools[i]
		if c.Requires == "web" && !r.WebSearch {
			r.mark(Skipped)
			return CaseResult{ID: c.ID, Outcome: Skipped, Problems: []string{"web search is not configured"}}
		}
		return r.traced(ctx, "tools", c.ID, []string{"correct"}, func(ctx context.Context, traceID string) CaseResult {
			conv, err := r.conversation(ctx, c.Docs)
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			if _, err := r.ToolAgent.Respond(ctx, services.ReplyRequest{UserID: r.user, Conversation: conv, Message: c.Message}, nil); err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			return scoreTools(r.toolsCalled(traceID), c)
		})
	})
	zeroFailed(cases, "correct")
	s := NewSuite("tools", cases)
	if v, ok := MeanMetric(cases, "correct"); ok {
		s.Aggregates["accuracy"] = v
	}
	for _, m := range []string{"missing", "wrong", "unnecessary"} {
		total := 0.0
		for _, c := range cases {
			total += c.Metrics[m]
		}
		s.Aggregates[m+"_total"] = total
	}
	return s
}

// scoreTools compares the tools called with a case: missing (expected, not called), wrong (forbidden,
// called) and unnecessary (neither expected nor allowed). Only missing or wrong calls fail a case.
func scoreTools(called []string, c ToolCase) CaseResult {
	in := func(list []string, t string) bool { return slices.Contains(list, t) }
	var missing, wrong, extra []string
	for _, t := range c.Expect {
		if !in(called, t) {
			missing = append(missing, t)
		}
	}
	for _, t := range dedupe(called) {
		switch {
		case in(c.Forbid, t):
			wrong = append(wrong, t)
		case !in(c.Expect, t) && !in(c.Allow, t):
			extra = append(extra, t)
		}
	}
	res := CaseResult{Outcome: Pass, Metrics: map[string]float64{
		"correct": boolf(len(missing) == 0 && len(wrong) == 0),
		"missing": float64(len(missing)), "wrong": float64(len(wrong)), "unnecessary": float64(len(extra)),
	}, Details: map[string][]string{"called": called}}
	if len(missing) > 0 {
		res.Problems = append(res.Problems, "did not call "+strings.Join(missing, ", "))
	}
	if len(wrong) > 0 {
		res.Problems = append(res.Problems, "called "+strings.Join(wrong, ", ")+", which it should not")
	}
	if len(res.Problems) > 0 {
		res.Outcome = Fail
	}
	return res
}

// Answers and citations: the agent answers a document question with every corpus document attached.

func (r *Runner) runAnswers(ctx context.Context) SuiteResult {
	criteria := rubrics["answer"].criteria
	scoreNames := append([]string{"citations_ok"}, prefixed("judge_", criteria)...)
	var answerCases []RAGCase
	for _, c := range r.Data.RAG {
		if c.Answer {
			answerCases = append(answerCases, c)
		}
	}
	cases := r.runCases(len(answerCases), func(i int) CaseResult {
		c := answerCases[i]
		return r.traced(ctx, "answer", c.ID, scoreNames, func(ctx context.Context, traceID string) CaseResult {
			conv, err := r.conversation(ctx, sortedKeys(r.Data.Corpus))
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			reply, err := r.Agent.Respond(ctx, services.ReplyRequest{UserID: r.user, Conversation: conv, Message: c.Question}, nil)
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			problems := CheckCitations(reply.Content, reply.Citations, r.corpusIDs(), r.docIDs[c.Doc])
			res := CaseResult{Outcome: Pass, Problems: problems, Metrics: map[string]float64{
				"citations_ok": boolf(len(problems) == 0), "citation_count": float64(len(reply.Citations)),
			}, Details: map[string]any{"answer": reply.Content, "citations": len(reply.Citations)}}

			if r.Judge != nil {
				v, err := r.Judge.Grade(ctx, "answer", map[string]string{
					"QUESTION": c.Question, "EVIDENCE": strings.Join(c.Evidence, "\n"),
					"SOURCE": r.Data.Corpus[c.Doc], "ANSWER": reply.Content, "CITED": r.citedPassages(ctx, reply.Citations),
				})
				if err != nil {
					return ErrorResult(c.ID, traceID, err)
				}
				addVerdict(&res, v)
			}
			if len(res.Problems) > 0 {
				res.Outcome = Fail
			}
			return res
		})
	})
	zeroFailed(cases, "citations_ok")
	s := NewSuite("answer", cases)
	for _, m := range append([]string{"citations_ok", "citation_count"}, prefixed("judge_", criteria)...) {
		if v, ok := MeanMetric(cases, m); ok {
			s.Aggregates[m] = v
		}
	}
	return s
}

// citedPassages gives the judge the full section behind each document citation.
func (r *Runner) citedPassages(ctx context.Context, citations []models.Citation) string {
	if len(citations) == 0 {
		return "(no citations)"
	}
	var b strings.Builder
	for _, c := range citations {
		text := c.Snippet
		if c.Kind != models.CitationWeb && c.DocumentID != "" {
			if sec, err := r.Docs.Section(ctx, r.user, c.DocumentID, c.PageNumber); err == nil {
				text = sec.Text
			}
		}
		fmt.Fprintf(&b, "[%d] %s\n\n", c.N, text)
	}
	return b.String()
}

// Quizzes: the real quiz service on one document and topic.

func (r *Runner) runQuiz(ctx context.Context) SuiteResult {
	criteria := rubrics["quiz"].criteria
	scoreNames := append([]string{"structure_ok"}, prefixed("judge_", criteria)...)
	cases := r.runCases(len(r.Data.Quiz), func(i int) CaseResult {
		c := r.Data.Quiz[i]
		return r.traced(ctx, "quiz", c.ID, scoreNames, func(ctx context.Context, traceID string) CaseResult {
			session, err := r.Quiz.GenerateQuiz(ctx, r.user, services.GenerateQuizRequest{
				DocumentID: r.docIDs[c.Doc], Topic: c.Topic, Difficulty: c.Difficulty, NumQuestions: c.Count,
			})
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			questions, err := r.quizQuestions(ctx, session.SessionID)
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			problems := CheckQuiz(questions, c.Count)
			res := CaseResult{Outcome: Pass, Problems: problems, Metrics: map[string]float64{"structure_ok": boolf(len(problems) == 0)}}
			if r.Judge != nil {
				items, _ := json.MarshalIndent(questions, "", " ")
				v, err := r.Judge.Grade(ctx, "quiz", map[string]string{"TOPIC": c.Topic, "SOURCE": r.Data.Corpus[c.Doc], "ITEMS": string(items)})
				if err != nil {
					return ErrorResult(c.ID, traceID, err)
				}
				addVerdict(&res, v)
			}
			if len(res.Problems) > 0 {
				res.Outcome = Fail
			}
			return res
		})
	})
	zeroFailed(cases, "structure_ok")
	s := NewSuite("quiz", cases)
	for _, m := range append([]string{"structure_ok"}, prefixed("judge_", criteria)...) {
		if v, ok := MeanMetric(cases, m); ok {
			s.Aggregates[m] = v
		}
	}
	return s
}

func (r *Runner) quizQuestions(ctx context.Context, sessionID string) ([]QuizQuestion, error) {
	var rows []models.QuizQuestion
	if err := r.DB.WithContext(ctx).Where("session_id = ?", sessionID).Order("question_order").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("loading quiz questions: %w", err)
	}
	out := make([]QuizQuestion, len(rows))
	for i, q := range rows {
		out[i] = QuizQuestion{Type: q.QuestionType, Text: q.QuestionText, Correct: q.CorrectOption, Explanation: q.Explanation}
		if q.OptionsJSON != "" {
			_ = json.Unmarshal([]byte(q.OptionsJSON), &out[i].Options) // malformed options fail CheckQuiz
		}
		if q.ReferenceAnswer != nil {
			out[i].Reference = *q.ReferenceAnswer
		}
	}
	return out, nil
}

// Flashcards: the real flashcard service on one document and topic.

func (r *Runner) runFlashcards(ctx context.Context) SuiteResult {
	criteria := rubrics["flashcards"].criteria
	scoreNames := append([]string{"structure_ok"}, prefixed("judge_", criteria)...)
	cases := r.runCases(len(r.Data.Flashcards), func(i int) CaseResult {
		c := r.Data.Flashcards[i]
		return r.traced(ctx, "flashcards", c.ID, scoreNames, func(ctx context.Context, traceID string) CaseResult {
			set, err := r.Cards.GenerateFlashcards(ctx, r.user, services.GenerateFlashcardsRequest{
				DocumentID: r.docIDs[c.Doc], Topic: c.Topic, Count: c.Count,
			})
			if err != nil {
				return ErrorResult(c.ID, traceID, err)
			}
			cards := make([]Card, len(set.Cards))
			for i, card := range set.Cards {
				cards[i] = Card{Front: card.Front, Back: card.Back}
			}
			problems := CheckFlashcards(cards, c.Count)
			res := CaseResult{Outcome: Pass, Problems: problems, Metrics: map[string]float64{"structure_ok": boolf(len(problems) == 0)}}
			if r.Judge != nil {
				items, _ := json.MarshalIndent(cards, "", " ")
				v, err := r.Judge.Grade(ctx, "flashcards", map[string]string{"TOPIC": c.Topic, "SOURCE": r.Data.Corpus[c.Doc], "ITEMS": string(items)})
				if err != nil {
					return ErrorResult(c.ID, traceID, err)
				}
				addVerdict(&res, v)
			}
			if len(res.Problems) > 0 {
				res.Outcome = Fail
			}
			return res
		})
	})
	zeroFailed(cases, "structure_ok")
	s := NewSuite("flashcards", cases)
	for _, m := range append([]string{"structure_ok"}, prefixed("judge_", criteria)...) {
		if v, ok := MeanMetric(cases, m); ok {
			s.Aggregates[m] = v
		}
	}
	return s
}

// addVerdict records judge scores as judge_* metrics; a score below minJudgeScore fails the case.
func addVerdict(res *CaseResult, v *Verdict) {
	for criterion, score := range v.Scores {
		res.Metrics["judge_"+criterion] = score
		if score < minJudgeScore {
			res.Problems = append(res.Problems, fmt.Sprintf("judge scored %s %.0f/5: %s", criterion, score, v.Reasons[criterion]))
		}
	}
	slices.Sort(res.Problems)
}

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

func dedupe(items []string) []string {
	var out []string
	for _, it := range items {
		if !slices.Contains(out, it) {
			out = append(out, it)
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
