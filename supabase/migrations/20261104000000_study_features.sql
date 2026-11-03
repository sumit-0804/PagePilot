-- Phase B: graded feedback for short answers, a summary cache, and computed (not stored) progress.

-- Feedback from grading short answers (multiple choice uses the question's explanation).
alter table public.quiz_answers add column feedback text;

-- One cached summary per document, summary type and topic; regenerated on request.
create unique index summaries_cache_key on public.summaries (user_id, document_id, type, topic);
alter table public.summaries add column updated_at timestamptz not null default now();

-- Progress is computed from quiz answers and flashcards when requested, so a stored copy could only drift.
drop table public.study_progress;
