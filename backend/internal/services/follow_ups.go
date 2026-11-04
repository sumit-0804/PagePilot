package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pagepilot/backend/internal/rag"
)

// FollowUp is a next message offered under an answer; Kind lets the UI pick an icon and explanation style.
type FollowUp struct {
	Text string `json:"text"`
	Kind string `json:"kind"` // simpler, example, deeper, quiz, flashcards or question
}

var followUpKinds = []string{"simpler", "example", "deeper", "quiz", "flashcards", "question"}

const (
	maxFollowUps     = 4
	maxFollowUpChars = 60
)

// suggestFollowUps asks the model for next steps that fit this exchange.
func suggestFollowUps(ctx context.Context, llm rag.LLMClient, question, answer string) ([]FollowUp, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`A user asked a study assistant a question and got an answer. Suggest %d short follow-up messages the user is likely to send next, written as the user would type them.

RULES:
1. Each one is specific to this topic (name the subject), at most 8 words, no trailing period.
2. kind is one of: simpler (explain it more simply), example (another example), deeper (more detail), quiz (quiz me on it), flashcards (make flashcards), question (a new related question).
3. Mix kinds: at most one quiz and one flashcards; include at least one related question.
4. Output a JSON object: {"follow_ups": [{"text": "...", "kind": "..."}]}

QUESTION:
%s

ANSWER:
%s`, maxFollowUps, rag.Snippet(question, 1000), rag.Snippet(answer, 3000))

	raw, err := llm.GenerateJSON(ctx, []rag.ChatMessage{
		{Role: "system", Content: "You suggest follow-up messages for a chat. Respond ONLY with a JSON object."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		FollowUps []FollowUp `json:"follow_ups"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("unreadable follow-ups: %w", err)
	}

	var out []FollowUp
	for _, f := range parsed.FollowUps {
		f.Text = strings.TrimSuffix(strings.TrimSpace(f.Text), ".")
		if f.Text == "" || utf8.RuneCountInString(f.Text) > maxFollowUpChars {
			continue
		}
		if !slices.Contains(followUpKinds, f.Kind) {
			f.Kind = "question"
		}
		out = append(out, f)
		if len(out) == maxFollowUps {
			break
		}
	}
	return out, nil
}
