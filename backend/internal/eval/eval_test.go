package eval

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/embeddings"
	"github.com/pagepilot/backend/internal/models"
)

// These tests need no database or API keys, so they run on every pull request.

func TestDatasetsAreValid(t *testing.T) {
	d, err := Load("../../evaluation")
	if err != nil {
		t.Fatal(err)
	}
	answers := 0
	for _, c := range d.RAG {
		if c.Answer {
			answers++
		}
	}
	got := fmt.Sprint(len(d.Corpus), len(d.RAG), answers, len(d.Tools), len(d.Quiz), len(d.Flashcards))
	if got != "5 50 15 20 10 10" {
		t.Errorf("corpus, rag, answer, tools, quiz, flashcards = %s; update this test if the datasets grew on purpose", got)
	}
	if len(d.Version) != 12 {
		t.Errorf("dataset version %q", d.Version)
	}
	for i := 0; i < 5; i++ {
		again, err := Load("../../evaluation")
		if err != nil || again.Version != d.Version {
			t.Fatalf("the dataset version must be the same on every load: %q then %q", d.Version, again.Version)
		}
	}
}

func TestValidateRejectsBadCases(t *testing.T) {
	d := &Data{
		Corpus: map[string]string{"doc": "The mitochondria produce ATP."},
		RAG: []RAGCase{
			{ID: "a", Doc: "doc", Question: "q", Evidence: []string{"produce   atp"}}, // case and spacing don't matter
			{ID: "b", Doc: "doc", Question: "q", Evidence: []string{"ribosomes make proteins"}},
			{ID: "b", Doc: "missing", Question: "q", Evidence: []string{"x"}},
		},
		Tools: []ToolCase{{ID: "t", Expect: []string{"make_coffee"}, Forbid: []string{"web_search"}, Allow: []string{"web_search"}}},
	}
	err := d.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{`"ribosomes make proteins" is not in doc`, `duplicate case id "b"`, `unknown corpus document "missing"`, `unknown tool "make_coffee"`, "web_search is in both"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing error %q in:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "produce   atp") {
		t.Error("evidence should match ignoring case and whitespace")
	}
}

func TestScoreRetrieval(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	chunks := []string{"unrelated", "Mitochondria produce ATP", "also unrelated", "ribosomes make proteins"}

	s := ScoreRetrieval(chunks, []string{"produce ATP"}, 3)
	if s.FirstHitRank != 2 || !near(s.MRR, 0.5) || s.RecallAtK != 1 || !near(s.PrecisionAtK, 1.0/3) {
		t.Errorf("one evidence at rank 2: %+v", s)
	}
	if want := 1 / math.Log2(3); !near(s.NDCGAtK, want) {
		t.Errorf("nDCG = %v, want %v", s.NDCGAtK, want)
	}

	s = ScoreRetrieval(chunks, []string{"produce ATP", "make proteins"}, 3)
	if s.RecallAtK != 0.5 {
		t.Errorf("second evidence is at rank 4, outside k=3: recall %v", s.RecallAtK)
	}

	s = ScoreRetrieval(chunks, []string{"photosynthesis"}, 3)
	if s.FirstHitRank != 0 || s.MRR != 0 || s.RecallAtK != 0 || s.NDCGAtK != 0 {
		t.Errorf("no hit: %+v", s)
	}

	s = ScoreRetrieval([]string{"produce ATP here", "and produce ATP there"}, []string{"produce ATP"}, 5)
	if s.NDCGAtK != 1 {
		t.Errorf("nDCG must be capped at 1 when overlapping chunks repeat the evidence: %v", s.NDCGAtK)
	}
}

func TestCheckCitations(t *testing.T) {
	corpus := map[string]bool{"d1": true, "d2": true}
	cites := []models.Citation{{N: 1, Kind: models.CitationDocument, DocumentID: "d1"}}

	if p := CheckCitations("Cells come from cells [1].", cites, corpus, "d1"); len(p) != 0 {
		t.Errorf("a good answer: %v", p)
	}
	checks := []struct {
		name, answer string
		cites        []models.Citation
		want         string
	}{
		{"invented marker", "Fact [1]. Another fact [2].", cites, "marker [2] has no matching source"},
		{"model's dagger format", "Fact 【3†L1-L2】.", cites, "marker [3] has no matching source"},
		{"no citations", "Cells come from cells.", nil, "no citations"},
		{"wrong document", "Fact [1].", []models.Citation{{N: 1, Kind: models.CitationDocument, DocumentID: "d2"}}, "does not cite the document"},
		{"unknown document", "Fact [1].", []models.Citation{{N: 1, Kind: models.CitationDocument, DocumentID: "zzz"}}, "not one of the attached documents"},
		{"web source", "Fact [1].", []models.Citation{{N: 1, Kind: models.CitationWeb, URL: "https://x"}}, "is a web page"},
		{"empty", "  ", cites, "empty answer"},
	}
	for _, c := range checks {
		if p := strings.Join(CheckCitations(c.answer, c.cites, corpus, "d1"), "; "); !strings.Contains(p, c.want) {
			t.Errorf("%s: problems %q, want %q", c.name, p, c.want)
		}
	}
}

func TestCheckQuiz(t *testing.T) {
	one, three := 1, 3
	good := []QuizQuestion{
		{Type: "mcq", Text: "What makes ATP?", Options: []string{"Ribosome", "Mitochondrion", "Golgi"}, Correct: &one, Explanation: "Mitochondria do."},
		{Type: "short_answer", Text: "Define osmosis.", Reference: "Diffusion of water.", Explanation: "From the text."},
	}
	if p := CheckQuiz(good, 2); len(p) != 0 {
		t.Errorf("good quiz: %v", p)
	}
	bad := []QuizQuestion{
		{Type: "mcq", Text: "What makes ATP?", Options: []string{"A", "a"}, Correct: &three},
		{Type: "mcq", Text: "what makes  ATP?", Options: []string{"A", "B"}, Correct: &one, Explanation: "x"},
		{Type: "essay", Text: "Discuss.", Explanation: "x"},
	}
	p := strings.Join(CheckQuiz(bad, 4), "; ")
	for _, want := range []string{"asked for 4 questions, got 3", "no valid correct option", "repeated option", "no explanation", "repeats an earlier question", `unknown type "essay"`} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in %q", want, p)
		}
	}
}

func TestCheckFlashcards(t *testing.T) {
	good := []Card{{Front: "What is osmosis?", Back: "Diffusion of water across a membrane."}}
	if p := CheckFlashcards(good, 1); len(p) != 0 {
		t.Errorf("good cards: %v", p)
	}
	bad := []Card{
		{Front: "Osmosis", Back: "osmosis"},
		{Front: "", Back: "x"},
		{Front: "OSMOSIS", Back: "different"},
		{Front: "Long", Back: strings.Repeat("x", 601)},
	}
	p := strings.Join(CheckFlashcards(bad, 5), "; ")
	for _, want := range []string{"asked for 5 cards, got 4", "same text on both sides", "empty side", "repeats an earlier card", "back is 601 characters"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in %q", want, p)
		}
	}
}

func TestScoreTools(t *testing.T) {
	c := ToolCase{Expect: []string{"create_quiz"}, Allow: []string{"search_documents"}, Forbid: []string{"create_flashcards"}}
	cases := []struct {
		called              []string
		correct             float64
		missing, wrong, ext float64
	}{
		{[]string{"search_documents", "create_quiz"}, 1, 0, 0, 0},
		{[]string{"create_quiz", "list_documents"}, 1, 0, 0, 1}, // unnecessary, but not a failure
		{[]string{"search_documents"}, 0, 1, 0, 0},
		{[]string{"create_quiz", "create_flashcards", "create_flashcards"}, 0, 0, 1, 0},
	}
	for _, tc := range cases {
		r := scoreTools(tc.called, c)
		m := r.Metrics
		if m["correct"] != tc.correct || m["missing"] != tc.missing || m["wrong"] != tc.wrong || m["unnecessary"] != tc.ext {
			t.Errorf("called %v: metrics %v", tc.called, m)
		}
		if (r.Outcome == Pass) != (tc.correct == 1) {
			t.Errorf("called %v: outcome %s", tc.called, r.Outcome)
		}
	}
}

func TestParseVerdict(t *testing.T) {
	criteria := []string{"correctness", "relevance"}
	v, err := parseVerdict(`{"correctness":{"score":4,"reason":"ok"},"relevance":{"score":5,"reason":"on topic"}}`, criteria)
	if err != nil || v.Scores["correctness"] != 4 || v.Reasons["relevance"] != "on topic" {
		t.Fatalf("verdict %+v, err %v", v, err)
	}
	for _, raw := range []string{`not json`, `{"correctness":{"score":4}}`, `{"correctness":{"score":9},"relevance":{"score":3}}`} {
		_, err := parseVerdict(raw, criteria)
		var je *JudgeError
		if !errors.As(err, &je) {
			t.Errorf("%s: want a JudgeError, got %v", raw, err)
		}
	}
}

func TestIsInfra(t *testing.T) {
	infra := []error{
		errors.New("groq returned status 429: rate limit reached"),
		errors.New(`POST "https://api.groq.com/openai/v1/chat/completions": 503 Service Unavailable`),
		fmt.Errorf("embedding: %w", embeddings.ErrDailyQuota),
		&JudgeError{"bad json"},
		errors.New("read tcp: connection reset by peer"),
	}
	for _, err := range infra {
		if !IsInfra(err) {
			t.Errorf("%v should be infrastructure", err)
		}
	}
	quality := []error{
		errors.New("the AI returned a quiz in an unreadable format, please try again"),
		errors.New("document has no content"),
		errors.New("expected 4500 tokens"), // numbers alone are not status codes
	}
	for _, err := range quality {
		if IsInfra(err) {
			t.Errorf("%v should be a quality failure", err)
		}
	}
}

func TestFinishAppliesThresholdsAndBaseline(t *testing.T) {
	th := &Thresholds{Floors: map[string]map[string]float64{"rag": {"recall_at_k": 0.8}}, MaxJudgeDrop: 0.5, MaxErrorRate: 0.2}
	suite := func(name string, agg map[string]float64, pass, errs int) SuiteResult {
		var cases []CaseResult
		for i := 0; i < pass; i++ {
			cases = append(cases, CaseResult{Outcome: Pass})
		}
		for i := 0; i < errs; i++ {
			cases = append(cases, CaseResult{Outcome: Error})
		}
		s := NewSuite(name, cases)
		s.Aggregates = agg
		return s
	}
	base := &Report{DatasetVersion: "v1", Suites: []SuiteResult{
		suite("answer", map[string]float64{"judge_correctness": 4.5, "citations_ok": 1}, 10, 0),
	}}

	ok := &Report{DatasetVersion: "v1", Suites: []SuiteResult{
		suite("rag", map[string]float64{"recall_at_k": 0.9}, 10, 0),
		suite("answer", map[string]float64{"judge_correctness": 4.2, "citations_ok": 0.5}, 10, 0),
	}}
	ok.Finish(th, base)
	if ok.ExitCode != 0 || ok.Suites[1].Status != StatusOK {
		t.Errorf("a small judge dip and a deterministic drop above any floor are fine: %+v", ok.Suites)
	}

	low := &Report{DatasetVersion: "v1", Suites: []SuiteResult{suite("rag", map[string]float64{"recall_at_k": 0.7}, 10, 0)}}
	low.Finish(th, base)
	if low.ExitCode != 1 || low.Suites[0].Status != StatusBelowThreshold {
		t.Errorf("below the floor: %+v", low.Suites[0])
	}

	regressed := &Report{DatasetVersion: "v1", Suites: []SuiteResult{suite("answer", map[string]float64{"judge_correctness": 3.8}, 10, 0)}}
	regressed.Finish(th, base)
	if regressed.ExitCode != 1 || regressed.Suites[0].Status != StatusRegression {
		t.Errorf("a judge drop of 0.7: %+v", regressed.Suites[0])
	}

	otherData := &Report{DatasetVersion: "v2", Suites: []SuiteResult{suite("answer", map[string]float64{"judge_correctness": 3.8}, 10, 0)}}
	otherData.Finish(th, base)
	if otherData.ExitCode != 0 || otherData.LikeForLike {
		t.Errorf("a changed dataset is compared but never a regression: %+v", otherData)
	}

	flaky := &Report{DatasetVersion: "v1", Suites: []SuiteResult{suite("rag", map[string]float64{"recall_at_k": 0.1}, 7, 3)}}
	flaky.Finish(th, base)
	if flaky.ExitCode != 2 || flaky.Suites[0].Status != StatusInconclusive {
		t.Errorf("30%% infrastructure errors must be inconclusive, not a failure: %+v", flaky.Suites[0])
	}
}
