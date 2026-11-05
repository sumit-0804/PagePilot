package eval

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pagepilot/backend/internal/rag"
)

//go:embed prompts/*.md
var promptFiles embed.FS

// A rubric is one versioned judge prompt and the criteria it must score.
type rubric struct {
	file     string
	criteria []string
}

// Bump a file's version (answer.v2.md, ...) when its wording or criteria change, so scores
// from different rubrics are not compared as if they were the same.
var rubrics = map[string]rubric{
	"answer":     {"prompts/answer.v1.md", []string{"correctness", "relevance", "completeness", "groundedness", "citation_support"}},
	"quiz":       {"prompts/quiz.v1.md", []string{"relevance", "answer_correctness", "distractor_quality", "explanation_consistency"}},
	"flashcards": {"prompts/flashcards.v1.md", []string{"consistency", "correctness", "coverage", "conciseness"}},
}

// Verdict is a judge's 1-5 score and reason for each criterion.
type Verdict struct {
	Scores  map[string]float64 `json:"scores"`
	Reasons map[string]string  `json:"reasons"`
}

// Judge grades generated content with an LLM, using the rubrics in prompts/.
type Judge struct {
	LLM   rag.LLMClient
	Model string
}

// PromptVersions names the rubric files in use, for the report.
func PromptVersions() map[string]string {
	out := map[string]string{}
	for kind, r := range rubrics {
		out[kind] = strings.TrimPrefix(r.file, "prompts/")
	}
	return out
}

// Grade fills the rubric for kind with vars ({{NAME}} placeholders) and returns the scores.
// A reply that is not valid JSON or leaves out a criterion is a judge failure, not a low score.
func (j *Judge) Grade(ctx context.Context, kind string, vars map[string]string) (*Verdict, error) {
	r, ok := rubrics[kind]
	if !ok {
		return nil, fmt.Errorf("no rubric for %q", kind)
	}
	tmpl, err := promptFiles.ReadFile(r.file)
	if err != nil {
		return nil, err
	}
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	prompt := strings.NewReplacer(pairs...).Replace(string(tmpl))

	raw, err := j.LLM.GenerateJSON(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You are a strict, fair grader. Reply with JSON only."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("judge call: %w", err)
	}
	return parseVerdict(raw, r.criteria)
}

func parseVerdict(raw string, criteria []string) (*Verdict, error) {
	var parsed map[string]struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	// Reasoning models may think aloud (with braces) before the JSON; keep only what follows.
	if _, after, ok := strings.Cut(raw, "</think>"); ok {
		raw = after
	}
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, &JudgeError{fmt.Sprintf("judge reply is not the expected JSON: %v", err)}
	}
	v := &Verdict{Scores: map[string]float64{}, Reasons: map[string]string{}}
	for _, c := range criteria {
		s, ok := parsed[c]
		if !ok || s.Score < 1 || s.Score > 5 {
			return nil, &JudgeError{fmt.Sprintf("judge left out %q or scored it outside 1-5", c)}
		}
		v.Scores[c] = s.Score
		v.Reasons[c] = s.Reason
	}
	return v, nil
}

// JudgeError means the judge itself misbehaved; the case is an error, not a quality failure.
type JudgeError struct{ msg string }

func (e *JudgeError) Error() string { return e.msg }
