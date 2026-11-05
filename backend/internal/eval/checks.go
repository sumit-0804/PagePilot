package eval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
)

// Matches citation markers such as [3], [1][4] and [2, 5].
var (
	markerRe = regexp.MustCompile(`\[(\d+(?:\s*[,;]\s*\d+)*)\]`)
	digitRe  = regexp.MustCompile(`\d+`)
)

// CheckCitations checks an answer to a document question: it cites something, every marker points
// at a returned source, every source is a real corpus document, and the expected document is cited.
// It returns the problems found; none means the answer passes.
func CheckCitations(answer string, citations []models.Citation, corpusIDs map[string]bool, wantDocID string) []string {
	var problems []string
	if strings.TrimSpace(answer) == "" {
		return []string{"empty answer"}
	}
	if len(citations) == 0 {
		problems = append(problems, "no citations for a question about the documents")
	}

	sources := map[int]bool{}
	citedWanted := false
	for _, c := range citations {
		sources[c.N] = true
		if c.Kind == models.CitationWeb {
			problems = append(problems, fmt.Sprintf("source [%d] is a web page, not a document", c.N))
			continue
		}
		if !corpusIDs[c.DocumentID] {
			problems = append(problems, fmt.Sprintf("source [%d] is not one of the attached documents", c.N))
		}
		if c.DocumentID == wantDocID {
			citedWanted = true
		}
	}
	if len(citations) > 0 && !citedWanted {
		problems = append(problems, "does not cite the document that holds the answer")
	}

	for _, m := range markerRe.FindAllStringSubmatch(rag.NormalizeCitations(answer), -1) {
		for _, part := range digitRe.FindAllString(m[1], -1) {
			if n, _ := strconv.Atoi(part); !sources[n] {
				problems = append(problems, fmt.Sprintf("marker [%d] has no matching source", n))
			}
		}
	}
	return problems
}

// QuizQuestion is the part of a generated question the checks look at.
type QuizQuestion struct {
	Type        string   `json:"type"`
	Text        string   `json:"text"`
	Options     []string `json:"options,omitempty"`
	Correct     *int     `json:"correct,omitempty"`
	Reference   string   `json:"reference,omitempty"`
	Explanation string   `json:"explanation"`
}

// CheckQuiz checks a generated quiz's structure: the asked-for size, answerable questions,
// a valid correct option, distinct options and questions, and an explanation for each.
func CheckQuiz(questions []QuizQuestion, wantCount int) []string {
	var problems []string
	if len(questions) != wantCount {
		problems = append(problems, fmt.Sprintf("asked for %d questions, got %d", wantCount, len(questions)))
	}
	seen := map[string]bool{}
	for i, q := range questions {
		label := fmt.Sprintf("question %d", i+1)
		key := Normalize(q.Text)
		switch {
		case key == "":
			problems = append(problems, label+" has no text")
		case seen[key]:
			problems = append(problems, label+" repeats an earlier question")
		}
		seen[key] = true

		switch q.Type {
		case "mcq", "true_false":
			if len(q.Options) < 2 {
				problems = append(problems, label+" has fewer than two options")
			}
			if q.Correct == nil || *q.Correct < 0 || *q.Correct >= len(q.Options) {
				problems = append(problems, label+" has no valid correct option")
			}
			options := map[string]bool{}
			for _, o := range q.Options {
				k := Normalize(o)
				if k == "" || options[k] {
					problems = append(problems, label+" has an empty or repeated option")
					break
				}
				options[k] = true
			}
		case "short_answer":
			if strings.TrimSpace(q.Reference) == "" {
				problems = append(problems, label+" has no reference answer")
			}
		default:
			problems = append(problems, fmt.Sprintf("%s has unknown type %q", label, q.Type))
		}
		if strings.TrimSpace(q.Explanation) == "" {
			problems = append(problems, label+" has no explanation")
		}
	}
	return problems
}

// Card is one generated flashcard.
type Card struct {
	Front string `json:"front"`
	Back  string `json:"back"`
}

const (
	maxFrontChars = 200
	maxBackChars  = 600
)

// CheckFlashcards checks generated cards: the asked-for count, both sides filled in, concise,
// not just repeating the front, and no duplicate fronts.
func CheckFlashcards(cards []Card, wantCount int) []string {
	var problems []string
	if len(cards) != wantCount {
		problems = append(problems, fmt.Sprintf("asked for %d cards, got %d", wantCount, len(cards)))
	}
	seen := map[string]bool{}
	for i, c := range cards {
		label := fmt.Sprintf("card %d", i+1)
		front, back := Normalize(c.Front), Normalize(c.Back)
		switch {
		case front == "" || back == "":
			problems = append(problems, label+" has an empty side")
		case front == back:
			problems = append(problems, label+" has the same text on both sides")
		case seen[front]:
			problems = append(problems, label+" repeats an earlier card")
		}
		seen[front] = true
		if n := len([]rune(c.Front)); n > maxFrontChars {
			problems = append(problems, fmt.Sprintf("%s front is %d characters (max %d)", label, n, maxFrontChars))
		}
		if n := len([]rune(c.Back)); n > maxBackChars {
			problems = append(problems, fmt.Sprintf("%s back is %d characters (max %d)", label, n, maxBackChars))
		}
	}
	return problems
}
