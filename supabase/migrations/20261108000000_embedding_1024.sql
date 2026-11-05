-- Embeddings move to Cloudflare Workers AI (@cf/qwen/qwen3-embedding-0.6b, 1024 dimensions).
-- Every existing document was indexed with 1536-dimension Gemini vectors, which can't be converted,
-- so the documents are deleted and users upload them again.
--
-- Deleting a document also deletes (on delete cascade) its chunks, its links to conversations, and the
-- quizzes, flashcard sets and summaries made from it. Conversations and their messages are kept.
-- Uploaded files are NOT removed: Supabase blocks SQL deletes on storage.objects, so empty the
-- "pdfs" bucket from the dashboard (Storage -> pdfs -> Empty bucket) when applying this.

delete from public.documents;

alter table public.document_chunks
  alter column embedding type extensions.vector(1024);
