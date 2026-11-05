// Package eval measures PagePilot's AI quality: retrieval, tool choice, answers, citations,
// quizzes and flashcards. Datasets live in backend/evaluation; see its README.
package eval

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tools the agent can call; datasets may only name these.
var KnownTools = map[string]bool{
	"list_documents": true, "search_documents": true, "create_quiz": true, "create_flashcards": true,
	"summarize_document": true, "get_study_progress": true, "web_search": true,
}

type RAGCase struct {
	ID       string   `json:"id"`
	Doc      string   `json:"doc"`
	Question string   `json:"question"`
	Evidence []string `json:"evidence"`
	Answer   bool     `json:"answer"` // also run through the agent for the answer and citation suite
}

type ToolCase struct {
	ID       string   `json:"id"`
	Message  string   `json:"message"`
	Docs     []string `json:"docs"`
	Expect   []string `json:"expect"`
	Allow    []string `json:"allow"`
	Forbid   []string `json:"forbid"`
	Requires string   `json:"requires"` // "web" when the case needs web search
	Note     string   `json:"note"`
}

type QuizCase struct {
	ID         string `json:"id"`
	Doc        string `json:"doc"`
	Topic      string `json:"topic"`
	Count      int    `json:"count"`
	Difficulty string `json:"difficulty"`
}

type FlashcardCase struct {
	ID    string `json:"id"`
	Doc   string `json:"doc"`
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// Data is everything a run needs from backend/evaluation.
type Data struct {
	Version    string            // hash of all dataset and corpus files
	Corpus     map[string]string // document key (file name without .md) -> text
	RAG        []RAGCase
	Tools      []ToolCase
	Quiz       []QuizCase
	Flashcards []FlashcardCase
}

// Load reads and validates the corpus and datasets under dir.
func Load(dir string) (*Data, error) {
	d := &Data{Corpus: map[string]string{}}
	hash := sha256.New()

	files, err := filepath.Glob(filepath.Join(dir, "corpus", "*.md"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no corpus documents in %s", filepath.Join(dir, "corpus"))
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		hash.Write(b)
		d.Corpus[strings.TrimSuffix(filepath.Base(f), ".md")] = string(b)
	}

	for name, target := range map[string]any{"rag": &d.RAG, "tools": &d.Tools, "quiz": &d.Quiz, "flashcards": &d.Flashcards} {
		path := filepath.Join(dir, "datasets", name+".jsonl")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		hash.Write(b)
		if err := readJSONL(path, b, target); err != nil {
			return nil, err
		}
	}
	d.Version = hex.EncodeToString(hash.Sum(nil))[:12]
	return d, d.Validate()
}

func readJSONL(path string, b []byte, target any) error {
	var items []json.RawMessage
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 1; sc.Scan(); line++ {
		if text := strings.TrimSpace(sc.Text()); text != "" {
			if !json.Valid([]byte(text)) {
				return fmt.Errorf("%s:%d: invalid JSON", path, line)
			}
			items = append(items, json.RawMessage(text))
		}
	}
	raw, _ := json.Marshal(items)
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Validate checks the datasets against the corpus, so a case can't silently test nothing:
// unique ids, known documents and tools, and evidence that really appears in its document.
func (d *Data) Validate() error {
	var errs []error
	ids := map[string]bool{}
	id := func(s string) {
		if s == "" || ids[s] {
			errs = append(errs, fmt.Errorf("missing or duplicate case id %q", s))
		}
		ids[s] = true
	}
	doc := func(caseID, key string) {
		if _, ok := d.Corpus[key]; !ok {
			errs = append(errs, fmt.Errorf("%s: unknown corpus document %q", caseID, key))
		}
	}

	for _, c := range d.RAG {
		id(c.ID)
		doc(c.ID, c.Doc)
		if strings.TrimSpace(c.Question) == "" || len(c.Evidence) == 0 {
			errs = append(errs, fmt.Errorf("%s: needs a question and evidence", c.ID))
		}
		for _, e := range c.Evidence {
			if !Contains(d.Corpus[c.Doc], e) {
				errs = append(errs, fmt.Errorf("%s: evidence %q is not in %s", c.ID, e, c.Doc))
			}
		}
	}
	for _, c := range d.Tools {
		id(c.ID)
		for _, key := range c.Docs {
			doc(c.ID, key)
		}
		seen := map[string]string{}
		for kind, tools := range map[string][]string{"expect": c.Expect, "allow": c.Allow, "forbid": c.Forbid} {
			for _, t := range tools {
				if !KnownTools[t] {
					errs = append(errs, fmt.Errorf("%s: unknown tool %q", c.ID, t))
				}
				if prev, dup := seen[t]; dup {
					errs = append(errs, fmt.Errorf("%s: %s is in both %s and %s", c.ID, t, prev, kind))
				}
				seen[t] = kind
			}
		}
		if c.Requires != "" && c.Requires != "web" {
			errs = append(errs, fmt.Errorf("%s: unknown requirement %q", c.ID, c.Requires))
		}
	}
	for _, c := range d.Quiz {
		id(c.ID)
		doc(c.ID, c.Doc)
		if c.Count < 1 || c.Count > 20 || c.Topic == "" {
			errs = append(errs, fmt.Errorf("%s: needs a topic and 1-20 questions", c.ID))
		}
	}
	for _, c := range d.Flashcards {
		id(c.ID)
		doc(c.ID, c.Doc)
		if c.Count < 1 || c.Count > 30 || c.Topic == "" {
			errs = append(errs, fmt.Errorf("%s: needs a topic and 1-30 cards", c.ID))
		}
	}
	return errors.Join(errs...)
}

var spaces = regexp.MustCompile(`\s+`)

// Normalize lowercases text and collapses whitespace, so evidence matches however a chunker wrapped it.
func Normalize(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(strings.ToLower(s), " "))
}

// Contains reports whether text contains evidence, ignoring case and whitespace differences.
func Contains(text, evidence string) bool {
	return strings.Contains(Normalize(text), Normalize(evidence))
}
