# PagePilot evaluations

Version-controlled datasets and settings for `go run ./cmd/eval`. The code lives in `internal/eval`.

| Path | What it is |
|---|---|
| `corpus/*.md` | Five short study documents written for these evals (CC0). Each paragraph is one line so evidence text matches the indexed chunks. |
| `datasets/rag.jsonl` | 50 retrieval questions. A chunk is relevant when it contains one of the case's `evidence` strings. Cases with `"answer": true` also run through the full agent for the answer and citation suite. |
| `datasets/tools.jsonl` | 20 tool-selection cases: `expect` must be called, `forbid` must not, `allow` is fine either way. Cases with `"requires": "web"` are skipped when web search is off. |
| `datasets/quiz.jsonl`, `datasets/flashcards.jsonl` | 10 generation cases each. |
| `thresholds.json` | Floors for the deterministic metrics and the largest allowed drop in judge scores. |
| `baselines/baseline.json` | The run new results are compared with. Refresh it with `-update-baseline` after a deliberate change. |
| `reports/` | Run reports (git-ignored). |

Each report records a dataset version, a hash of every dataset and corpus file. When it differs from the baseline's, the comparison is shown but marked as not like-for-like.
