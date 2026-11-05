package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pagepilot/backend/internal/embeddings"
)

// Outcome of one case. Errors are infrastructure problems (rate limits, timeouts, a broken judge)
// and never count toward quality scores.
type Outcome string

const (
	Pass    Outcome = "pass"
	Fail    Outcome = "fail"
	Error   Outcome = "error"
	Skipped Outcome = "skipped"
)

type CaseResult struct {
	ID       string             `json:"id"`
	Outcome  Outcome            `json:"outcome"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Problems []string           `json:"problems,omitempty"`
	Error    string             `json:"error,omitempty"`
	TraceID  string             `json:"trace_id,omitempty"`
	Details  any                `json:"details,omitempty"`
}

// Suite status after thresholds and the baseline are applied.
const (
	StatusOK             = "ok"
	StatusBelowThreshold = "below_threshold"
	StatusRegression     = "regression"
	StatusInconclusive   = "inconclusive"
)

type Change struct {
	Metric    string  `json:"metric"`
	Baseline  float64 `json:"baseline"`
	Current   float64 `json:"current"`
	Delta     float64 `json:"delta"`
	Regressed bool    `json:"regressed"`
}

type SuiteResult struct {
	Name       string             `json:"name"`
	Status     string             `json:"status"`
	Counts     map[Outcome]int    `json:"counts"`
	Aggregates map[string]float64 `json:"aggregates"`
	Changes    []Change           `json:"changes,omitempty"`
	Notes      []string           `json:"notes,omitempty"`
	Cases      []CaseResult       `json:"cases"`
}

type Report struct {
	DatasetVersion  string            `json:"dataset_version"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      time.Time         `json:"finished_at"`
	GitCommit       string            `json:"git_commit,omitempty"`
	Models          map[string]string `json:"models"`
	PromptVersions  map[string]string `json:"prompt_versions,omitempty"`
	Judge           bool              `json:"judge"`
	Tokens          map[string]int    `json:"tokens"` // input + output tokens each model reported
	BaselineVersion string            `json:"baseline_dataset_version,omitempty"`
	LikeForLike     bool              `json:"baseline_like_for_like"`
	Suites          []SuiteResult     `json:"suites"`
	ExitCode        int               `json:"exit_code"`
}

// Thresholds are the pass rules, kept in backend/evaluation/thresholds.json.
type Thresholds struct {
	// Floors are minimum values for aggregate metrics, per suite.
	Floors map[string]map[string]float64 `json:"floors"`
	// MaxJudgeDrop is how far a judge_* average (1-5) may fall below the baseline.
	MaxJudgeDrop float64 `json:"max_judge_drop"`
	// MaxErrorRate is the share of errored cases above which a suite is inconclusive.
	MaxErrorRate float64 `json:"max_error_rate"`
}

func LoadThresholds(path string) (*Thresholds, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Thresholds
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &t, nil
}

// LoadReport reads a saved report, such as the baseline.
func LoadReport(path string) (*Report, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// NewSuite counts the outcomes; the suite code fills in Aggregates.
func NewSuite(name string, cases []CaseResult) SuiteResult {
	s := SuiteResult{Name: name, Cases: cases, Counts: map[Outcome]int{}, Aggregates: map[string]float64{}}
	for _, c := range cases {
		s.Counts[c.Outcome]++
	}
	return s
}

// zeroFailed gives failed cases that never produced metrics (the product call itself failed) a 0 on
// the suite's main metric, so they count against it instead of vanishing from the average.
func zeroFailed(cases []CaseResult, metric string) {
	for i := range cases {
		if cases[i].Outcome != Fail {
			continue
		}
		if _, ok := cases[i].Metrics[metric]; !ok {
			if cases[i].Metrics == nil {
				cases[i].Metrics = map[string]float64{}
			}
			cases[i].Metrics[metric] = 0
		}
	}
}

// MeanMetric averages one metric over the scored (pass or fail) cases that have it.
func MeanMetric(cases []CaseResult, name string) (float64, bool) {
	var vals []float64
	for _, c := range cases {
		if v, ok := c.Metrics[name]; ok && (c.Outcome == Pass || c.Outcome == Fail) {
			vals = append(vals, v)
		}
	}
	return Mean(vals), len(vals) > 0
}

// Finish applies the thresholds and baseline to every suite and sets the exit code:
// 0 all good, 1 a suite is below a floor or regressed, 2 a suite is inconclusive (and none failed).
func (r *Report) Finish(t *Thresholds, baseline *Report) {
	if baseline != nil {
		r.BaselineVersion = baseline.DatasetVersion
		r.LikeForLike = baseline.DatasetVersion == r.DatasetVersion
	}
	failed, inconclusive := false, false
	for i := range r.Suites {
		s := &r.Suites[i]
		s.Status = StatusOK
		scored := s.Counts[Pass] + s.Counts[Fail] + s.Counts[Error]
		if scored == 0 || float64(s.Counts[Error])/float64(scored) > t.MaxErrorRate {
			s.Status = StatusInconclusive
			note := fmt.Sprintf("%d of %d cases hit infrastructure errors; scores are not reliable", s.Counts[Error], scored)
			if scored == 0 {
				note = "no case was scored (all skipped)"
			}
			s.Notes = append(s.Notes, note)
			inconclusive = true
			continue
		}
		for metric, floor := range t.Floors[s.Name] {
			if v, ok := s.Aggregates[metric]; ok && v < floor {
				s.Status = StatusBelowThreshold
				s.Notes = append(s.Notes, fmt.Sprintf("%s %.3f is below the floor %.3f", metric, v, floor))
			}
		}
		if prev := suiteNamed(baseline, s.Name); prev != nil {
			s.Changes = compare(prev.Aggregates, s.Aggregates, t.MaxJudgeDrop)
			for _, c := range s.Changes {
				if c.Regressed && r.LikeForLike && s.Status == StatusOK {
					s.Status = StatusRegression
				}
				if c.Regressed {
					s.Notes = append(s.Notes, fmt.Sprintf("%s fell %.2f below the baseline", c.Metric, -c.Delta))
				}
			}
		}
		if s.Status != StatusOK {
			failed = true
		}
	}
	switch {
	case failed:
		r.ExitCode = 1
	case inconclusive:
		r.ExitCode = 2
	default:
		r.ExitCode = 0
	}
}

func suiteNamed(r *Report, name string) *SuiteResult {
	if r == nil {
		return nil
	}
	for i := range r.Suites {
		if r.Suites[i].Name == name {
			return &r.Suites[i]
		}
	}
	return nil
}

// compare reports every metric's change. Only judge_* averages can regress against the baseline;
// deterministic metrics are held to their floors instead, which minor model variation can't trip.
func compare(base, cur map[string]float64, maxJudgeDrop float64) []Change {
	var out []Change
	for metric, now := range cur {
		was, ok := base[metric]
		if !ok {
			continue
		}
		c := Change{Metric: metric, Baseline: was, Current: now, Delta: now - was}
		c.Regressed = strings.HasPrefix(metric, "judge_") && c.Delta < -maxJudgeDrop
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metric < out[j].Metric })
	return out
}

// Save writes the report as indented JSON.
func (r *Report) Save(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// PrintSummary writes a readable table of the run.
func (r *Report) PrintSummary(w io.Writer) {
	fmt.Fprintf(w, "\nPagePilot eval  dataset %s  commit %s  judge %v\n", r.DatasetVersion, orDash(r.GitCommit), r.Judge)
	if r.BaselineVersion != "" && !r.LikeForLike {
		fmt.Fprintf(w, "Baseline used dataset %s, so changes are shown but not treated as regressions.\n", r.BaselineVersion)
	}
	for _, s := range r.Suites {
		fmt.Fprintf(w, "\n%-11s %-15s pass %d  fail %d  error %d  skipped %d\n",
			s.Name, strings.ToUpper(s.Status), s.Counts[Pass], s.Counts[Fail], s.Counts[Error], s.Counts[Skipped])
		changes := map[string]Change{}
		for _, c := range s.Changes {
			changes[c.Metric] = c
		}
		names := make([]string, 0, len(s.Aggregates))
		for n := range s.Aggregates {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			line := fmt.Sprintf("  %-28s %7.3f", n, s.Aggregates[n])
			if c, ok := changes[n]; ok && math.Abs(c.Delta) >= 0.0005 {
				line += fmt.Sprintf("  (%+.3f vs baseline)", c.Delta)
			}
			fmt.Fprintln(w, line)
		}
		for _, n := range s.Notes {
			fmt.Fprintf(w, "  ! %s\n", n)
		}
		for _, c := range s.Cases {
			switch c.Outcome {
			case Fail:
				fmt.Fprintf(w, "  FAIL  %s: %s\n", c.ID, strings.Join(c.Problems, "; "))
			case Error:
				fmt.Fprintf(w, "  ERROR %s: %s\n", c.ID, c.Error)
			}
		}
	}
	models := make([]string, 0, len(r.Tokens))
	for m := range r.Tokens {
		models = append(models, m)
	}
	sort.Strings(models)
	fmt.Fprintf(w, "\nTook %s. Tokens used:", r.FinishedAt.Sub(r.StartedAt).Round(time.Second))
	for _, m := range models {
		fmt.Fprintf(w, " %s %d;", m, r.Tokens[m])
	}
	fmt.Fprintf(w, "\nExit code %d\n", r.ExitCode)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Rate limits, provider outages and timeouts (status codes matched as whole numbers), and a provider
// rejecting the request itself (4xx), which is a setup problem, not a sign of answer quality.
var infraRe = regexp.MustCompile(`(?i)\b(429|500|502|503|504)\b|rate.?limit|too many requests|timeout|timed out|connection (reset|refused)|no such host|unexpected EOF|overloaded|API error: 4\d\d|returned status 4\d\d`)

// IsInfra reports whether err is an infrastructure problem rather than a quality failure.
func IsInfra(err error) bool {
	if err == nil {
		return false
	}
	var je *JudgeError
	return errors.As(err, &je) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, embeddings.ErrDailyQuota) ||
		infraRe.MatchString(err.Error())
}

// ErrorResult turns err into an error (infrastructure) or fail (product) result.
func ErrorResult(id, traceID string, err error) CaseResult {
	if IsInfra(err) {
		return CaseResult{ID: id, Outcome: Error, Error: err.Error(), TraceID: traceID}
	}
	return CaseResult{ID: id, Outcome: Fail, Problems: []string{err.Error()}, TraceID: traceID}
}
