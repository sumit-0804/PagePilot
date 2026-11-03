-- PagePilot schema: app tables, pgvector chunks, RLS lockdown and the private PDF bucket.

create extension if not exists vector with schema extensions;

-- Profiles mirror auth.users; created automatically on sign-up.
create table public.profiles (
  id           uuid primary key references auth.users (id) on delete cascade,
  display_name text,
  created_at   timestamptz not null default now()
);

create function public.handle_new_user()
returns trigger
language plpgsql
security definer
set search_path = ''
as $$
begin
  insert into public.profiles (id, display_name)
  values (new.id, new.raw_user_meta_data ->> 'display_name');
  return new;
end;
$$;

create trigger on_auth_user_created
  after insert on auth.users
  for each row execute function public.handle_new_user();

create table public.documents (
  id              uuid primary key default gen_random_uuid(),
  user_id         uuid not null references auth.users (id) on delete cascade,
  filename        text not null,
  storage_path    text not null,
  file_size       bigint not null,
  content_hash    text not null,
  page_count      integer not null default 0,
  status          text not null check (status in ('UPLOADED', 'EXTRACTING', 'CHUNKING', 'EMBEDDING', 'INDEXING', 'READY', 'FAILED')),
  error_message   text,
  embedding_model text,
  chunks_total    integer not null default 0,
  chunks_embedded integer not null default 0,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now()
);
create index documents_user_created_idx on public.documents (user_id, created_at desc);
create index documents_user_hash_idx on public.documents (user_id, content_hash);
create index documents_status_idx on public.documents (status);

-- Search always targets one document, so an exact distance sort over its chunks is used (no ANN index).
create table public.document_chunks (
  id            uuid primary key default gen_random_uuid(),
  document_id   uuid not null references public.documents (id) on delete cascade,
  user_id       uuid not null references auth.users (id) on delete cascade,
  page_number   integer not null,
  chunk_index   integer not null,
  chunk_text    text not null,
  document_name text not null,
  embedding     extensions.vector(1536) not null,
  created_at    timestamptz not null default now(),
  unique (document_id, chunk_index)
);
create index document_chunks_user_doc_idx on public.document_chunks (user_id, document_id);

create table public.conversations (
  id          uuid primary key default gen_random_uuid(),
  user_id     uuid not null references auth.users (id) on delete cascade,
  document_id uuid not null references public.documents (id) on delete cascade,
  title       text not null,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
create index conversations_user_updated_idx on public.conversations (user_id, updated_at desc);

create table public.messages (
  id              uuid primary key default gen_random_uuid(),
  conversation_id uuid not null references public.conversations (id) on delete cascade,
  role            text not null check (role in ('user', 'assistant', 'system')),
  content         text not null,
  type            text not null default 'chat',
  citations_json  text,
  metadata_json   text,
  created_at      timestamptz not null default now()
);
create index messages_conversation_created_idx on public.messages (conversation_id, created_at);

create table public.quiz_sessions (
  id              uuid primary key default gen_random_uuid(),
  user_id         uuid not null references auth.users (id) on delete cascade,
  document_id     uuid not null references public.documents (id) on delete cascade,
  conversation_id uuid references public.conversations (id) on delete set null,
  topic           text not null default '',
  difficulty      text not null default 'medium',
  total_questions integer not null default 0,
  current_index   integer not null default 0,
  score           integer not null default 0,
  completed       boolean not null default false,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now()
);
create index quiz_sessions_user_doc_idx on public.quiz_sessions (user_id, document_id);

create table public.quiz_questions (
  id               uuid primary key default gen_random_uuid(),
  session_id       uuid not null references public.quiz_sessions (id) on delete cascade,
  question_order   integer not null,
  question_type    text not null default 'mcq' check (question_type in ('mcq', 'true_false', 'short_answer')),
  question_text    text not null,
  options_json     text not null default '[]',
  correct_option   integer,
  reference_answer text,
  explanation      text not null default '',
  citation_page    integer,
  citation_excerpt text,
  unique (session_id, question_order)
);

create table public.quiz_answers (
  id              uuid primary key default gen_random_uuid(),
  question_id     uuid not null unique references public.quiz_questions (id) on delete cascade,
  session_id      uuid not null references public.quiz_sessions (id) on delete cascade,
  user_id         uuid not null references auth.users (id) on delete cascade,
  selected_option integer,
  answer_text     text,
  is_correct      boolean not null,
  answered_at     timestamptz not null default now()
);
create index quiz_answers_session_idx on public.quiz_answers (session_id);

create table public.flashcard_sets (
  id              uuid primary key default gen_random_uuid(),
  user_id         uuid not null references auth.users (id) on delete cascade,
  document_id     uuid not null references public.documents (id) on delete cascade,
  conversation_id uuid references public.conversations (id) on delete set null,
  title           text not null,
  topic           text not null default '',
  created_at      timestamptz not null default now()
);
create index flashcard_sets_user_doc_idx on public.flashcard_sets (user_id, document_id);

create table public.flashcards (
  id            uuid primary key default gen_random_uuid(),
  set_id        uuid not null references public.flashcard_sets (id) on delete cascade,
  front         text not null,
  back          text not null,
  citation_page integer,
  status        text not null default 'new' check (status in ('new', 'known', 'practice')),
  created_at    timestamptz not null default now()
);
create index flashcards_set_idx on public.flashcards (set_id);

create table public.study_progress (
  id                   uuid primary key default gen_random_uuid(),
  user_id              uuid not null references auth.users (id) on delete cascade,
  document_id          uuid not null references public.documents (id) on delete cascade,
  total_quizzes_taken  integer not null default 0,
  average_score        double precision not null default 0,
  flashcards_known     integer not null default 0,
  flashcards_need_work integer not null default 0,
  weak_topics_json     text not null default '[]',
  last_studied_at      timestamptz not null default now(),
  unique (user_id, document_id)
);

create table public.summaries (
  id             uuid primary key default gen_random_uuid(),
  user_id        uuid not null references auth.users (id) on delete cascade,
  document_id    uuid not null references public.documents (id) on delete cascade,
  type           text not null,
  topic          text not null default '',
  content        text not null,
  citations_json text,
  created_at     timestamptz not null default now()
);
create index summaries_user_doc_idx on public.summaries (user_id, document_id);

-- RLS on with no policies: the browser's publishable key can't reach app data through the
-- auto-generated API. The Go backend connects as the postgres role, which bypasses RLS.
alter table public.profiles enable row level security;
alter table public.documents enable row level security;
alter table public.document_chunks enable row level security;
alter table public.conversations enable row level security;
alter table public.messages enable row level security;
alter table public.quiz_sessions enable row level security;
alter table public.quiz_questions enable row level security;
alter table public.quiz_answers enable row level security;
alter table public.flashcard_sets enable row level security;
alter table public.flashcards enable row level security;
alter table public.study_progress enable row level security;
alter table public.summaries enable row level security;

-- Private bucket for uploaded PDFs; files live at {user_id}/{document_id}.pdf.
insert into storage.buckets (id, name, public, file_size_limit, allowed_mime_types)
values ('pdfs', 'pdfs', false, 52428800, array['application/pdf']);
