-- Thumbs up/down on tutor answers, and the trace id that links an answer to its Langfuse trace.

-- One rating per assistant message. Ownership comes from the message's conversation, so no user id is
-- stored, and a guest's ratings follow their chats when they sign up.
create table public.message_feedback (
  message_id uuid primary key references public.messages (id) on delete cascade,
  rating     smallint not null check (rating in (-1, 1)),
  reason     text check (reason in ('incorrect', 'not_grounded', 'unhelpful', 'unclear', 'other')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index message_feedback_rating_idx on public.message_feedback (rating, created_at);

-- RLS on with no policies, as for every table: only the backend (postgres role) can read or write.
alter table public.message_feedback enable row level security;

-- Set on assistant messages when tracing is on; null otherwise.
alter table public.messages add column trace_id text;
