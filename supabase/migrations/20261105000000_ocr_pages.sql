-- How many pages of a document had no text layer and were read with OCR.
alter table public.documents add column ocr_pages integer not null default 0;
