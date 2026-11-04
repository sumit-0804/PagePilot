-- Open chat: conversations no longer need a document, can attach many, and uploads accept more formats.

create table public.conversation_documents (
  conversation_id uuid not null references public.conversations (id) on delete cascade,
  document_id     uuid not null references public.documents (id) on delete cascade,
  user_id         uuid not null references auth.users (id) on delete cascade,
  created_at      timestamptz not null default now(),
  primary key (conversation_id, document_id)
);
create index conversation_documents_document_idx on public.conversation_documents (document_id);
alter table public.conversation_documents enable row level security;

insert into public.conversation_documents (conversation_id, document_id, user_id, created_at)
select id, document_id, user_id, created_at from public.conversations;

alter table public.conversations drop column document_id;

-- Quizzes and flashcards can now be made from general knowledge, without a document.
alter table public.quiz_sessions alter column document_id drop not null;
alter table public.flashcard_sets alter column document_id drop not null;

alter table public.documents add column format text not null default 'pdf'
  check (format in ('pdf', 'docx', 'pptx', 'txt', 'md', 'csv', 'html'));
alter table public.documents add column mime_type text not null default 'application/pdf';

update storage.buckets
set allowed_mime_types = array[
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  'text/plain',
  'text/markdown',
  'text/csv',
  'text/html'
]
where id = 'pdfs';
