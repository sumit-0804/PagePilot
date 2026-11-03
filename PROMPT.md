# Build an PagePilot-(AI Study Companion) Companion — Complete Development Prompt

Act as a senior full-stack engineer, AI architect, and UI/UX designer. Build a complete, functional **PagePilot-(AI Study Companion) Companion** that allows users to upload PDF documents, ask questions about their contents, understand difficult topics, take interactive quizzes, generate flashcards, and study through an AI-powered conversational interface.

Build the application for **local development only**. Do not implement production deployment or cloud hosting.

Do not just generate a plan, mockup, or static prototype. Build the actual working application with a functional backend, frontend, database, vector search, PDF processing, and AI integration.

---

# 1. Technology Stack

Use the following technologies.

### Backend
- Language: Go (Golang)
- AI agent framework: Google Agent Development Kit (ADK) for Go, version 2.0.0 baseline.
- LLM provider: Groq API, model `openai/gpt-oss-120b`.
- API architecture: REST API.
- Platform: Supabase, used for:
  - Database: Supabase PostgreSQL.
  - Vector search: pgvector inside the same Supabase PostgreSQL database.
  - Authentication: Supabase Auth (email + password).
  - File storage: Supabase Storage (private bucket).
- PDF text extraction: A suitable Go PDF extraction library.
- Embeddings: Google `gemini-embedding-2` at 1536 dimensions via the native Gemini API, behind a separately configurable embedding provider.
- Embedding rate limits (requests per minute, tokens per minute, requests per day) come from configuration and are never hardcoded.
- Configuration: Environment variables.

**Important ADK requirements:**
- Verify the exact current Google ADK Go APIs and package paths before implementation.
- Use actual supported ADK Go interfaces and methods.
- Do not invent package names, functions, constructors, or integration examples.
- If a requested API differs from the installed version, use the correct compatible implementation and document the difference.
- Integrate Groq through a supported OpenAI-compatible interface where appropriate.
- Do not assume Groq provides an embedding model. Keep embedding generation independent and configurable.

### Frontend
- React
- TypeScript
- Bun
- Vite
- Tailwind CSS
- shadcn/ui
- React Router
- TanStack Query
- Zustand (where useful)
- PDF.js for PDF viewing.

### Local infrastructure
- Supabase CLI local stack (`supabase start`, runs on Docker) for PostgreSQL + pgvector, Auth, Storage and Studio.
- Develop and test against the local stack first, then push the same migrations to the hosted Supabase project.
- Uploaded PDFs stored in Supabase Storage, not on the local filesystem.
- No production deployment configuration yet (see "Future deployment" in section 14).

---

# 2. Main Application Concept

The application should work like a personal PagePilot-(AI Study Companion) assistant.

The user uploads a PDF. The application processes the PDF and makes its content searchable.

The user can then interact with the document through a conversational interface similar in usability to ChatGPT or Claude.

The user should be able to:
1. Ask questions about the uploaded PDF.
2. Get explanations of difficult concepts.
3. Ask follow-up questions naturally.
4. Generate quizzes from the PDF.
5. Answer quizzes directly inside the conversation.
6. Receive instant feedback and explanations.
7. Generate flashcards.
8. Generate chapter-wise or complete summaries.
9. Revise important topics.
10. Ask the AI tutor to explain topics at different difficulty levels.

All learning activities must be grounded in the uploaded PDF wherever possible.

---

# 3. PDF Upload and Processing

Initially, support PDF files only.

Create an intuitive PDF upload experience.

### Upload workflow

1. User (signed in) selects or drops a PDF.
2. Frontend uploads the file to the Go backend with the user's access token.
3. Backend validates file type (`%PDF-` header, not just the extension), size, and content hash (reuse an existing READY document with the same hash for the same user).
4. Store the PDF in the private Supabase Storage bucket at `{user_id}/{document_id}.pdf`.
5. Create a document record in PostgreSQL.
6. Extract text from the PDF.
7. Preserve page numbers.
8. Divide extracted content into meaningful chunks.
9. Generate embeddings for each chunk in batches, respecting the configured rate limits: wait when the per-minute request or token limit is reached, and fail with a clear message when the daily request limit is used up.
10. Store chunk text, metadata and vectors (pgvector) in PostgreSQL in one transaction.
11. Show embedding progress (chunks done / total) while processing.
12. Mark the document as READY when processing finishes.

### Document processing statuses

Implement the following statuses:

- UPLOADED
- EXTRACTING
- CHUNKING
- EMBEDDING
- INDEXING
- READY
- FAILED

Show the current processing status in the frontend.

Allow the user to see meaningful error messages if processing fails.

### PDF processing requirements

- Preserve page numbers for every chunk.
- Use sensible chunk sizes and configurable overlap.
- Avoid splitting sentences or paragraphs unnecessarily.
- Support multi-page PDFs.
- Handle empty or corrupted PDFs gracefully.
- Detect PDFs with little or no extractable text.
- Provide an OCR extension point for scanned PDFs, but do not make OCR mandatory for the initial version.
- Avoid processing the same document repeatedly when it can safely be reused.

Each chunk must contain metadata such as:

- document_id
- user_id
- page_number
- chunk_index
- chunk_text
- document_name

The application must never mix content from different documents.

---

# 4. RAG-Based Question Answering

This is the most important feature.

Implement a proper Retrieval-Augmented Generation (RAG) pipeline.

### Query workflow

When a user asks a question:

1. Receive the question from the frontend.
2. Identify the active document or selected document collection.
3. Retrieve relevant conversation context.
4. Rewrite the question if necessary to improve retrieval.
5. Generate an embedding for the query (query embeddings and document embeddings use the retrieval settings recommended for each).
6. Search pgvector, filtered by the signed-in user's `user_id` and the active `document_id`.
7. Retrieve the most relevant chunks.
8. Optionally rerank retrieved chunks.
9. Send the relevant context and question to the Groq-powered LLM.
10. Generate an answer grounded in the retrieved PDF content.
11. Return citations with page numbers.
12. Display the response in the conversational interface.

### Strict grounding rules

The AI must follow these rules:

- Prioritize information from the uploaded PDF.
- Do not invent facts or references.
- Never fabricate page numbers.
- If the answer cannot be found in the document, clearly say so.
- Distinguish document information from general knowledge.
- Use citations for claims based on the PDF.
- Provide short supporting excerpts where useful.
- Never claim that something appears in the PDF unless retrieved content supports it.

Example response:

**Question:** What is machine learning?

**Answer:** Machine learning is a branch of artificial intelligence that enables systems to learn patterns from data and improve their performance without being explicitly programmed for every task.

**Source:** Page 12.

Clicking the citation should open the PDF viewer at the corresponding page.

---

# 5. ChatGPT / Claude-Style Conversational Interface

The main application experience should be a polished conversational interface inspired by ChatGPT and Claude.

Do not copy their logos, branding, or exact visual identity. Create an original design with comparable usability.

### Main layout

Build a three-part layout.

**Left sidebar:**
- Application logo and name.
- New Chat button.
- Chat history.
- Uploaded PDFs.
- Recent study sessions.
- Settings.
- Signed-in user section (Supabase Auth) with sign out.
- Collapse and expand sidebar functionality.

**Main conversation area:**
- Welcome screen when no conversation exists.
- AI and user messages.
- Markdown rendering.
- Syntax-highlighted code blocks.
- Mathematical expressions using KaTeX.
- PDF citations.
- Interactive quiz components.
- Interactive flashcard components.
- Explanation cards.
- Loading and streaming response states.

**Bottom composer:**
- Multiline text input.
- Send button.
- Attachment button where appropriate.
- Selected PDF indicator.
- Enter to send.
- Shift + Enter for a new line.
- Suggested prompt chips when the conversation is empty.

### Conversation behavior

The interface should support natural follow-up questions.

For example:

User: Explain neural networks.

AI: Provides an explanation grounded in the PDF.

User: Explain it more simply.

AI: Gives a simpler explanation.

User: Give me an example.

AI: Provides an example.

User: Test my understanding.

AI: Starts an interactive quiz based on the same topic.

Maintain conversation context and persist chat history in PostgreSQL.

### Suggested prompt examples

- Explain this PDF.
- Summarize this document.
- Explain the current topic simply.
- Generate a quiz from this PDF.
- Create flashcards.
- What are the most important concepts?
- Help me prepare for an exam.

---

# 6. Interactive Conversational Quiz System

Quizzes must happen inside the chat interface, not on a separate traditional quiz page.

The experience should feel like interacting with an AI tutor.

### Quiz generation

Allow users to request:
- Generate a quiz.
- Generate a quiz from a specific chapter.
- Generate questions about a particular topic.
- Generate easy, medium, or difficult questions.
- Generate multiple-choice questions.
- Generate true/false questions.
- Generate short-answer questions.

### Quiz interaction

Display one question at a time.

Example:

**AI Tutor:**

Let's test your understanding of neural networks.

**Question 1 of 5**

What is the primary purpose of an activation function in a neural network?

A. Store training data  
B. Introduce non-linearity  
C. Reduce the number of layers  
D. Replace the optimizer

The user selects an option.

After submission:

- Send the selected answer to the backend.
- Validate the answer on the backend.
- Show whether the answer is correct.
- Explain why the answer is correct.
- Explain why other options are incorrect when useful.
- Include PDF citations.
- Display the user's score and progress.
- Provide a Continue button for the next question.

Do not reveal the correct answer before the user submits their response.

### Quiz session management

- Persist quiz sessions in PostgreSQL.
- Save answers and scores.
- Allow users to resume unfinished quizzes.
- Show final results.
- Track weak topics.
- Use quiz performance to suggest future revision.

Create reusable frontend components:

- QuizQuestionCard
- QuizOption
- QuizFeedback
- QuizProgress
- QuizResult
- QuizCitation

---

# 7. AI Explanation System

The AI tutor should explain concepts in a clear, structured, student-friendly way.

Support different explanation styles:

- Simple explanation.
- Detailed explanation.
- Step-by-step explanation.
- Beginner-friendly explanation.
- Technical explanation.
- Real-world analogy.
- Example-based explanation.
- Exam-oriented explanation.

### Explanation format

Where appropriate, structure explanations as:

1. Simple definition.
2. Main concept.
3. Step-by-step explanation.
4. Real-world analogy.
5. Practical example.
6. Important points to remember.
7. PDF references.

After an explanation, show suggested actions such as:

- Explain more simply.
- Give another example.
- Go deeper.
- Test my understanding.
- Generate a quiz.
- Create flashcards.

These actions should send contextual follow-up messages to the AI.

---

# 8. Flashcards

Allow users to generate flashcards from the uploaded PDF.

Requirements:

- Generate question-and-answer cards.
- Support topic-specific flashcards.
- Support chapter-wise flashcards.
- Show one card at a time.
- Flip cards to reveal answers.
- Provide Next and Previous controls.
- Allow users to mark cards as Known or Need Practice.
- Persist flashcard sets and study progress.
- Include page citations for source material.

Flashcards should also be accessible through the conversational interface.

---

# 9. PDF Summarization

Allow users to request:

- Complete PDF summary.
- Chapter-wise summary.
- Topic-wise summary.
- Short revision notes.
- Detailed notes.
- Important definitions.
- Key concepts.
- Important formulas.
- Exam preparation notes.

Summaries must be grounded in the uploaded document.

Include page references wherever possible.

For large documents, use hierarchical summarization rather than sending the entire PDF text to the LLM in one request.

---

# 10. AI Agent Architecture Using Google ADK Go

Use Google ADK Go to organize the AI functionality into modular agents.

Design the system with the following responsibilities:

### Coordinator Agent
- Understand the user's intent.
- Identify the required task.
- Route requests to the appropriate agent.
- Maintain conversation flow.

### Retrieval Agent
- Search relevant PDF chunks.
- Retrieve supporting evidence.
- Return page-level citations.

### Tutor Agent
- Explain difficult concepts.
- Answer document-related questions.
- Handle follow-up questions.
- Adapt explanation difficulty.

### Quiz Agent
- Generate questions.
- Manage quiz flow.
- Evaluate submitted answers.
- Provide explanations and citations.

### Flashcard Agent
- Generate flashcards.
- Organize them by topic.
- Support revision.

### Summary Agent
- Generate summaries and study notes.
- Support chapter-wise summarization.

### Study Progress Agent
- Track quiz results.
- Identify weak topics.
- Recommend revision activities.

Do not overcomplicate the architecture.

Start with a working end-to-end application, then introduce agent specialization incrementally.

Use ADK's actual supported orchestration and tool patterns. If some functionality is better handled by normal Go services, keep it in normal Go services rather than forcing every operation into an agent.

---

# 11. Database Design

Use Supabase PostgreSQL for application data and vectors.

Create SQL migrations in `supabase/migrations/` (applied with `supabase db reset` locally and `supabase db push` for the hosted project) and matching models.

Suggested tables:

- profiles (id references `auth.users`; users themselves are managed by Supabase Auth)
- documents (including content_hash, storage_path, embedding_model)
- document_chunks
- conversations
- messages
- quiz_sessions
- quiz_questions
- quiz_answers
- flashcard_sets
- flashcards
- study_progress
- summaries

Use pgvector for vector storage: `document_chunks.embedding vector(1536)`, in the same row as the chunk text.

Every chunk row must carry the columns needed for filtering:

- user_id
- document_id
- page_number
- chunk_index

Ensure proper indexing, foreign keys (with `ON DELETE CASCADE`), and database relationships. Every user-owned table has `user_id` referencing `auth.users(id)`.

Enable Row Level Security on every app table with no policies, so the browser's publishable key cannot read app data through Supabase's auto-generated API. Only the Go backend, connecting directly to Postgres, reads and writes app data.

Use real Supabase Auth from the start. Every query is scoped to the signed-in user.

---

# 12. Backend API

Create a clean REST API.

Every endpoint except `/health` requires `Authorization: Bearer <Supabase access token>`. The backend verifies the token and scopes all data to that user; another user's resources return 404.

Suggested endpoints:

### Documents
- POST /api/documents/upload
- GET /api/documents
- GET /api/documents/{id}
- DELETE /api/documents/{id}
- GET /api/documents/{id}/status (includes embedding progress)
- GET /api/documents/{id}/file (returns a short-lived signed Supabase Storage URL)

### Conversations
- POST /api/conversations
- GET /api/conversations
- GET /api/conversations/{id}
- DELETE /api/conversations/{id}

### Chat
- POST /api/chat
- GET /api/conversations/{id}/messages

### Quiz
- POST /api/quizzes/generate
- POST /api/quizzes/{id}/answer
- GET /api/quizzes/{id}
- POST /api/quizzes/{id}/next

### Flashcards
- POST /api/flashcards/generate
- GET /api/flashcards
- GET /api/flashcards/{id}
- PATCH /api/flashcards/cards/{cardId}

### Summaries
- POST /api/summaries/generate

### Progress
- GET /api/progress?document_id=

### Health
- GET /health

Adapt endpoint names and request structures as needed.

Implement validation, consistent JSON responses, meaningful HTTP status codes, and appropriate error handling.

Use Server-Sent Events (SSE) for streaming AI responses. Send a heartbeat comment every ~15 seconds so proxies don't close idle streams. The frontend should display streamed text naturally.

---

# 13. Frontend Design Requirements

The design must look like a polished, modern AI product.

### Visual direction

- Minimal and clean.
- Modern typography.
- Generous whitespace.
- Neutral background.
- Subtle borders.
- Rounded cards.
- Smooth transitions.
- Responsive layout.
- Dark mode and light mode.
- Consistent spacing.
- Accessible colors and controls.

Avoid:
- Excessive gradients.
- Unnecessary animations.
- Overcrowded dashboards.
- Too many cards.
- Generic admin-panel appearance.

### Important screens

1. Sign in / Sign up.
2. Welcome / Dashboard.
3. PDF Upload.
4. PDF Processing Status.
5. Main AI Chat.
6. PDF Viewer.
7. Interactive Quiz.
8. Flashcards.
9. Study History.
10. Settings.

The chat interface should be the primary experience. Other features should feel naturally integrated into it.

### PDF viewer

Use PDF.js.

Requirements:
- Render the uploaded PDF.
- Display page navigation.
- Support zoom.
- Open a specific page when a citation is clicked.
- Keep the viewer and chat usable together.
- Support a collapsible or resizable viewer panel.

---

# 14. Local Development Setup

Build and test locally first. The hosted Supabase project is used only after the local workflow passes.

### Local architecture

React + Bun frontend:
http://localhost:5173

Go + Google ADK backend:
http://localhost:8080

Supabase local stack (Supabase CLI):
- API (Auth, Storage): http://127.0.0.1:54321
- PostgreSQL + pgvector: 127.0.0.1:54322
- Studio: http://127.0.0.1:54323
- Mailpit (auth emails): http://127.0.0.1:54324

Groq and Gemini:
External APIs accessed using the user's API keys.

### Supabase CLI

Use the Supabase CLI (`supabase start`, which runs on Docker) for PostgreSQL + pgvector, Auth and Storage. Keep its config and migrations in `supabase/`.

Do not containerize the frontend or backend unless needed for local development.

### File storage

Store uploaded PDFs in a private Supabase Storage bucket named `pdfs`, at `{user_id}/{document_id}.pdf`. The bucket is created by a migration. The PDF viewer loads files through short-lived signed URLs.

Store extracted text or intermediate processing files locally only when necessary.

### Environment variables

Create a `.env.example` file containing placeholders for:

- API_PORT
- FRONTEND_URL (comma-separated list of allowed CORS origins)
- GROQ_API_KEY
- GROQ_MODEL
- EMBEDDING_PROVIDER
- EMBEDDING_MODEL
- GEMINI_API_KEY
- EMBEDDING_DIM
- EMBEDDING_BATCH_SIZE
- EMBEDDING_RPM
- EMBEDDING_TPM
- EMBEDDING_RPD
- SUPABASE_URL
- SUPABASE_PUBLISHABLE_KEY
- SUPABASE_SECRET_KEY (backend only)
- DATABASE_URL
- STORAGE_BUCKET

Frontend (`frontend/.env.example`): VITE_SUPABASE_URL, VITE_SUPABASE_PUBLISHABLE_KEY, VITE_API_URL (defaults to `/api`).

Never commit real API keys. Keep `.env` and `.env.*` (except `.env.example`) out of Git.

### Local startup

The expected workflow should be simple:

1. Install Go, Bun, and Docker.
2. Clone the project.
3. Copy `.env.example` to `.env`.
4. Add the Groq and Gemini API keys, and the embedding RPM/TPM/RPD from Google AI Studio.
5. Start the Supabase local stack (`bunx supabase start`).
6. Apply migrations (`bunx supabase db reset`).
7. Copy the local Supabase URL and keys from `bunx supabase status` into `.env`.
8. Start the Go backend.
9. Start the React frontend using Bun.
10. Open the application in the browser and sign up (confirmation emails appear in Mailpit).

Provide exact commands in the README.

### Pushing to the hosted Supabase project

After the local workflow passes: `supabase link --project-ref <ref>`, then `supabase db push`. Connect the backend through the session pooler URL (the direct database host is IPv6-only), with pgx's simple protocol mode enabled.

### Future deployment (not built now)

- Frontend: Vercel, at `pagepilot.sumit0804.space`.
- Go API: Render, at `api.pagepilot.sumit0804.space`.
- Supabase cloud for database, vectors, auth and storage.

Keep all configuration env-driven so this needs no code changes, but do not add Vercel, Render or other deployment files yet.

---

# 15. Project Structure

Use a clean and maintainable structure similar to:

```text
ai-study-companion/
│
├── backend/
│   ├── cmd/
│   │   └── server/
│   ├── internal/
│   │   ├── api/
│   │   ├── auth/
│   │   ├── config/
│   │   ├── database/
│   │   ├── models/
│   │   ├── repositories/
│   │   ├── services/
│   │   ├── storage/
│   │   ├── pdf/
│   │   ├── embeddings/
│   │   ├── vectorstore/
│   │   ├── rag/
│   │   ├── agents/
│   │   ├── quizzes/
│   │   ├── flashcards/
│   │   └── summaries/
│   └── go.mod
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── chat/
│   │   │   ├── quiz/
│   │   │   ├── flashcards/
│   │   │   ├── pdf/
│   │   │   ├── layout/
│   │   │   └── ui/
│   │   ├── pages/
│   │   ├── hooks/
│   │   ├── services/
│   │   ├── stores/
│   │   ├── types/
│   │   └── utils/
│   ├── package.json
│   └── vite.config.ts
│
├── supabase/
│   ├── config.toml
│   └── migrations/
│
├── docs/
│   └── architecture.excalidraw
│
├── .env.example
├── .gitignore
└── README.md
```

You may adjust the structure when technically justified, but keep responsibilities separated.

---

# 16. Development Strategy

Build the application in phases. Do not attempt to generate every feature at once without verifying that the foundation works.

### Phase 1 — Project foundation
- Set up Go backend.
- Set up React + Bun frontend.
- Set up the Supabase local stack (PostgreSQL + pgvector, Auth, Storage) and migrations.
- Configure environment variables.
- Add Supabase Auth: token verification in the backend, sign in / sign up in the frontend.
- Verify frontend-to-backend communication.
- Implement health checks.

### Phase 2 — PDF ingestion
- Implement PDF upload.
- Store PDF in Supabase Storage.
- Extract text and page numbers.
- Implement chunking.
- Generate embeddings with Gemini, respecting the configured rate limits.
- Store chunks and vectors in PostgreSQL (pgvector).
- Save document metadata in PostgreSQL.
- Display processing status and embedding progress.

### Phase 3 — RAG chat
- Implement document-filtered vector retrieval.
- Integrate Groq.
- Implement grounded responses.
- Add page-level citations.
- Add persistent chat history.
- Add PDF viewer citation navigation.

### Phase 4 — Conversational interface
- Build the ChatGPT/Claude-inspired UI.
- Add Markdown and mathematical rendering.
- Add streaming responses.
- Add conversation history.
- Add selected-document context.

### Phase 5 — Study features
- Add interactive quizzes.
- Add answer validation and feedback.
- Add explanations.
- Add flashcards.
- Add summaries.
- Add progress tracking.

### Phase 6 — AI agent orchestration
- Integrate Google ADK Go.
- Implement the coordinator and specialized agents.
- Add tools and agent routing.
- Verify each agent's behavior.

### Phase 7 — Testing and refinement
- Test the complete workflow.
- Fix errors.
- Improve UI responsiveness.
- Improve retrieval quality.
- Add useful loading, empty, and error states.
- Verify that the README setup works from a clean local environment.
- Push migrations to the hosted Supabase project and repeat the smoke tests against it.

---

# 17. Testing and Acceptance Criteria

The application is not complete until the following workflow works:

1. Start the Supabase local stack.
2. Start the Go backend.
3. Start the React frontend with Bun.
4. Open the application.
5. Sign up and sign in.
6. Upload a PDF containing at least 10 pages.
7. See its processing status.
8. Wait until it becomes READY.
9. Open a conversation associated with the PDF.
10. Ask a question whose answer is clearly present in the PDF.
11. Receive a relevant answer with the correct page citation.
12. Click the citation and navigate to the correct PDF page.
13. Ask a follow-up question.
14. Request a simpler explanation.
15. Generate a quiz.
16. Answer a quiz question and receive feedback.
17. Generate flashcards.
18. Generate a summary.
19. Refresh the browser and verify that conversations and study progress persist.

Also test:

- An unsupported or corrupted PDF.
- A PDF with little or no extractable text.
- A question whose answer does not appear in the PDF.
- A document with multiple relevant pages.
- A long PDF.
- A failed Groq API request.
- A failed embedding request.
- Document isolation so results from one PDF cannot leak into another.
- User isolation: a second user cannot see or access the first user's documents, chats or files.
- Requests without a valid token are rejected.
- Embedding rate limits: processing waits at the per-minute limits and fails with a clear message when the daily limit is used up.

Write unit and integration tests. Use mock LLM and embedding clients where possible so automated tests do not require real API keys.

---

# 18. Important Instructions for Antigravity

Follow these rules throughout development:

1. Build a real working application, not a static demo.
2. Prioritize the PDF upload → processing → RAG → cited answer workflow.
3. Keep the interface conversational and make quizzes and explanations part of the chat.
4. Use Go for the backend and React + Bun for the frontend.
5. Use Google ADK Go with verified APIs.
6. Use Groq for LLM inference.
7. Use Gemini `gemini-embedding-2` behind a separately configurable embedding provider.
8. Use Supabase (PostgreSQL + pgvector, Auth, Storage), through the Supabase CLI locally.
9. Store PDFs in Supabase Storage.
10. Do not add production deployment files yet.
11. Do not hardcode API keys or provider rate limits.
12. Do not fabricate citations or page numbers.
13. Keep code modular and readable.
14. Do not leave core functionality as TODOs or fake implementations.
15. Do not silently substitute mock data for real application functionality.
16. If an external API or dependency is unavailable, explain the limitation and implement a clearly documented alternative.
17. Verify that all frontend API calls match the actual backend implementation.
18. Run builds and tests after completing each major phase.
19. Fix compiler errors, dependency errors, and runtime errors before proceeding.
20. Maintain a README with complete local setup instructions.

**Start by inspecting the workspace and creating the project foundation. Then implement Phase 1 and Phase 2, verify them, and continue through the remaining phases. Do not stop after producing a plan. Build the application incrementally until the complete local workflow is functional.**