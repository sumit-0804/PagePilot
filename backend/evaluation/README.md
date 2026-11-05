# PagePilot evaluations

Version-controlled datasets and settings for `go run ./cmd/eval`. The code lives in `internal/eval`.

| Path | What it is |
|---|---|
| `corpus/*.md` | Four short study documents written for these evals (CC0). Each paragraph is one line so evidence text matches the indexed chunks. |
| `datasets/rag.jsonl` | 40 retrieval questions. A chunk is relevant when it contains one of the case's `evidence` strings. The 8 cases with `"answer": true` also run through the full agent for the answer and citation suite. |
| `datasets/tools.jsonl` | 8 tool-selection cases, one per tool: `expect` must be called, `forbid` must not, `allow` is fine either way. Cases with `"requires": "web"` are skipped when web search is off. Content-generating tools are stubbed here; the quiz and flashcard suites test them for real. |
| `datasets/quiz.jsonl`, `datasets/flashcards.jsonl` | 8 generation cases each. |
| `thresholds.json` | Floors for the deterministic metrics and the largest allowed drop in judge scores. |
| `baselines/baseline.json` | The run new results are compared with. Refresh it with `-update-baseline` after a deliberate change. |
| `reports/` | Run reports (git-ignored). |

## Keys and limits

Evals never use production keys. Set `EVAL_GROQ_API_KEY` (agent, study services and judge) and `EVAL_CLOUDFLARE_API_KEY` + `EVAL_CLOUDFLARE_ACCOUNT_ID` (embeddings, same model as production). The judge runs on a different Groq model (`EVAL_JUDGE_MODEL`, default `qwen/qwen3.8-27b`), which has its own daily allowance.

The sets are kept small so a full judged run stays well under Groq's free 200k tokens a day per model. `-max-tokens` (default 120k) skips the remaining LLM cases once a run has used that many, `-limit N` runs only the first N cases of each suite for smoke tests, and `-concurrency` (default 3) sets how many cases run at once. Each report records the tokens it used.

Each report records a dataset version, a hash of every dataset and corpus file. When it differs from the baseline's, the comparison is shown but marked as not like-for-like.
