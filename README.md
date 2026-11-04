# PagePilot

Study any PDF by asking it questions. Every answer cites the page it came from; click a citation to open the PDF at that page. When you're ready, take a quiz in the chat, flip through flashcards, or get a summary, all grounded in your document.

Runs locally: a Go API, a React app, and a local Supabase stack (Postgres + pgvector, Auth, Storage). Groq runs the language model and Gemini creates the embeddings.

The architecture diagram is [docs/architecture.excalidraw](docs/architecture.excalidraw); open it in [Excalidraw](https://excalidraw.com).

## How it works

- **Upload:** the PDF is checked, stored in a private Supabase Storage bucket, split into passages page by page, embedded with Gemini `gemini-embedding-2` (1536 dimensions, within your rate limits) and saved in Postgres with pgvector. Scanned pages (pages with no text layer) are read with a Gemini Flash model first, and keep their page numbers for citations.
- **Chat:** a Google ADK tutor agent (on Groq `openai/gpt-oss-120b`) searches your document, answers with `[Page N]` citations, and can create quizzes, flashcards and summaries, or explain your progress. Only pages it actually retrieved can be cited.
- **Accounts:** Supabase Auth (email and password). Every request is scoped to the signed-in user.

| Part | Where |
|---|---|
| React app (Vite, Bun) | http://localhost:5173 |
| Go API | http://localhost:8080 |
| Supabase API (Auth, Storage) | http://127.0.0.1:54321 |
| Postgres + pgvector | 127.0.0.1:54322 |
| Supabase Studio | http://127.0.0.1:54323 |
| Mailpit (auth emails) | http://127.0.0.1:54324 |

## Prerequisites

- [Go](https://go.dev/dl/) 1.27 or newer
- [Bun](https://bun.sh) 1.4 or newer
- [Docker Desktop](https://www.docker.com/products/docker-desktop/), running (the Supabase CLI uses it)
- A [Groq API key](https://console.groq.com/keys)
- A [Gemini API key](https://aistudio.google.com/apikey), plus the rate limits for `gemini-embedding-2` and your OCR model (default `gemini-3.5-flash-lite`) from the AI Studio rate-limit page (requests per minute, tokens per minute, requests per day)

The Supabase CLI runs through `bunx`, so there's nothing else to install.

## Setup

Run these from the repository root.

1. Install the frontend dependencies:

   ```bash
   bun install --cwd frontend
   ```

2. Create your config file:

   ```bash
   cp .env.example .env
   ```

   In `.env`, set `GROQ_API_KEY`, `GEMINI_API_KEY`, and from AI Studio `EMBEDDING_RPM`, `EMBEDDING_TPM`, `EMBEDDING_RPD`, `OCR_RPM`, `OCR_TPM`, `OCR_RPD`.

3. Start the local Supabase stack. The first run downloads Docker images and applies the migrations in `supabase/migrations`:

   ```bash
   bunx supabase start
   ```

4. Show the local keys:

   ```bash
   bunx supabase status
   ```

   Copy the `PUBLISHABLE_KEY` value into both `SUPABASE_PUBLISHABLE_KEY` and `VITE_SUPABASE_PUBLISHABLE_KEY`, and the `SECRET_KEY` value into `SUPABASE_SECRET_KEY`. The URLs in `.env.example` already match the local stack.

## Run

Start the API (it checks the database, storage, Auth and the embedding model on startup, and stops with a clear message if anything is missing):

```bash
cd backend && go run ./cmd/server
```

In a second terminal, start the app:

```bash
bun run --cwd frontend dev
```

Open http://localhost:5173, create an account, and upload a PDF. Local sign-ups don't need email confirmation.

## Tests

The Go tests use the local Supabase database (keep `bunx supabase start` running) and fake language models, so they need no API keys:

```bash
cd backend && go test ./...
```

Frontend type-check, build and lint:

```bash
bun run --cwd frontend build
```

```bash
bun run --cwd frontend lint
```

## Configuration

All settings live in `.env` at the repository root; the frontend reads only the `VITE_` ones. See `.env.example` for every option. A few worth knowing:

- `CHAT_ENGINE`: `adk` (default) uses the Google ADK tutor agent; `direct` uses one routing call plus the matching service.
- `EMBEDDING_RPM`, `EMBEDDING_TPM`, `EMBEDDING_RPD`: Google counts every passage as one request, even inside a batch, so with 100 RPM and 1,000 RPD you can index about 100 passages a minute and 1,000 a day (roughly 1,000 pages of light text, fewer for dense pages). Processing waits when a per-minute limit is reached. When the daily limit is used up, the document is marked failed with a message asking you to upload it again after the reset at midnight Pacific time. Documents interrupted by an API restart resume automatically.
- `OCR_PROVIDER`: `gemini` reads scanned pages with `OCR_MODEL`; `none` turns OCR off, and scanned PDFs then fail with a message asking for a text layer. A page counts as scanned when it has fewer than 40 characters of text. Each request reads up to `OCR_PAGES_PER_CALL` pages, so with 250 RPD and 10 pages per call you can read about 2,500 scanned pages a day. `OCR_MAX_PAGES` caps the scanned pages per document. OCR can misread words or numbers, so the document page says when it was used.
- `FRONTEND_URL`: comma-separated list of browser origins allowed to call the API.

## Database changes

Schema changes are SQL migrations in `supabase/migrations`.

- Apply new migrations to the local database:

  ```bash
  bunx supabase migration up
  ```

- Start the local database over from scratch (this deletes all local data):

  ```bash
  bunx supabase db reset
  ```

- Push migrations to a hosted Supabase project, using its session pooler connection string (Dashboard, Connect, Session pooler). Write any `@` in the password as `%40`:

  ```bash
  bunx supabase db push --remote "postgresql://postgres.<project-ref>:<password>@<pooler-host>:5432/postgres"
  ```

## Troubleshooting

- **`failed to connect to the docker API`:** start Docker Desktop, then run `bunx supabase start` again.
- **The API stops with `invalid configuration`:** the message lists each missing or invalid setting in `.env`.
- **`embedding provider check failed`:** check `GEMINI_API_KEY`, and that `EMBEDDING_DIM` is 1536.
- **`OCR model check failed`:** check `OCR_MODEL` against the model list in AI Studio, or set `OCR_PROVIDER=none`.
- **A PDF ends in "Failed":** the document page shows why. Encrypted PDFs can't be read, and scanned PDFs need OCR turned on.
- **A PDF fails with "The daily limit for indexing documents has been reached":** upload it again after the reset at midnight Pacific time, or raise the limit in Google AI Studio and update `EMBEDDING_RPD`.

## Privacy

PDF text (and, with OCR on, the scanned pages themselves) goes to Google's Gemini API. On the free tier, Google may use the content you send to improve its products. Don't upload sensitive PDFs unless you're on a paid tier.
