// Mirrors the Go API's JSON shapes.

export type DocumentStatus = 'UPLOADED' | 'EXTRACTING' | 'CHUNKING' | 'EMBEDDING' | 'INDEXING' | 'READY' | 'FAILED'

export type DocumentFormat = 'pdf' | 'docx' | 'pptx' | 'txt' | 'md' | 'csv' | 'html'

export interface StudyDocument {
  id: string
  user_id: string
  filename: string
  format: DocumentFormat
  file_size: number
  page_count: number
  status: DocumentStatus
  error_message?: string
  embedding_model?: string
  chunks_total: number
  chunks_embedded: number
  /** Pages with no text layer whose text was read by OCR. */
  ocr_pages: number
  created_at: string
  updated_at: string
}

export interface DocumentStatusResponse {
  id: string
  status: DocumentStatus
  error_message?: string
  page_count: number
  chunks_total: number
  chunks_embedded: number
  updated_at: string
}

/**
 * A source behind an answer: numbered [n] markers match by `n`; summaries and older
 * messages use [Page N] markers, have no `n`, and match by `page_number`.
 */
export interface Citation {
  n?: number
  /** Missing on older citations, which are always document pages. */
  kind?: 'document' | 'web'
  document_id?: string
  document_name?: string
  page_number?: number
  /** Section label from the document, e.g. "Slide 4" or "Page 3". */
  label?: string
  url?: string
  title?: string
  snippet: string
}

export interface DocumentSection {
  number: number
  label: string
  text: string
}

export type MessageType = 'chat' | 'quiz' | 'flashcards' | 'summary'

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  type: MessageType
  citations_json?: string
  metadata_json?: string
  created_at: string
}

export interface Conversation {
  id: string
  user_id: string
  title: string
  created_at: string
  updated_at: string
  /** Attached documents; the list endpoint may leave this out. */
  documents?: StudyDocument[]
  messages?: Message[]
}

export type ExplanationStyle =
  | 'simple'
  | 'detailed'
  | 'step_by_step'
  | 'beginner'
  | 'technical'
  | 'analogy'
  | 'example'
  | 'exam'

export interface ChatDoneEvent {
  type: 'done'
  conversation_id: string
  user_message: Message
  assistant_message: Message
  citations: Citation[]
}

export type QuestionType = 'mcq' | 'true_false' | 'short_answer'

export interface QuizQuestion {
  id: string
  question_order: number
  question_type: QuestionType
  question_text: string
  options: string[]
  is_answered: boolean
  selected_option?: number
  answer_text?: string
  is_correct?: boolean
  correct_option?: number
  reference_answer?: string
  explanation?: string
  feedback?: string
  citation_page?: number
  citation_excerpt?: string
}

export interface QuizSession {
  session_id: string
  /** Empty when the quiz was made without a document. */
  document_id?: string | null
  topic: string
  difficulty: string
  total_questions: number
  current_index: number
  score: number
  completed: boolean
  current_question?: QuizQuestion
  all_questions?: QuizQuestion[]
}

export interface AnswerFeedback {
  question_id: string
  question_type: QuestionType
  is_correct: boolean
  correct_option?: number | null
  selected_option?: number
  answer_text?: string
  reference_answer?: string
  feedback?: string
  explanation: string
  citation_page?: number
  citation_excerpt?: string
  current_score: number
  total_questions: number
  current_index: number
  completed: boolean
  next_question?: QuizQuestion
}

export type CardStatus = 'new' | 'known' | 'practice'

export interface Flashcard {
  id: string
  set_id: string
  front: string
  back: string
  citation_page?: number
  status: CardStatus
  created_at: string
}

export interface FlashcardSet {
  id: string
  user_id: string
  document_id?: string | null
  conversation_id?: string
  title: string
  topic: string
  created_at: string
  cards: Flashcard[]
}

export interface SummaryResponse {
  document_id: string
  type: string
  topic: string
  summary: string
  citations: Citation[]
  cached: boolean
}

export interface WeakArea {
  page: number
  missed: number
  attempted: number
  example_question: string
}

export interface Progress {
  document_id: string
  quizzes_completed: number
  quizzes_in_progress: number
  questions_answered: number
  correct_answers: number
  average_score_pct: number
  flashcards: { total: number; known: number; practice: number; new: number }
  weak_areas: WeakArea[]
  weak_topics: string[]
  recommendations: string[]
  last_studied_at?: string
}
