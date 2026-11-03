export type DocumentStatus =
  | 'UPLOADED'
  | 'EXTRACTING'
  | 'CHUNKING'
  | 'EMBEDDING'
  | 'INDEXING'
  | 'READY'
  | 'FAILED'

export interface Document {
  id: string
  user_id: string
  filename: string
  file_path: string
  file_size: number
  page_count: number
  status: DocumentStatus
  error_message?: string
  created_at: string
  updated_at: string
}

export interface Citation {
  document_id: string
  document_name: string
  page_number: number
  snippet: string
}

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  type: 'chat' | 'quiz' | 'flashcard' | 'summary' | 'explanation'
  citations_json?: string
  metadata_json?: string
  created_at: string
  citations?: Citation[]
}

export interface Conversation {
  id: string
  user_id: string
  document_id?: string
  title: string
  created_at: string
  updated_at: string
  messages?: Message[]
}

export interface QuizQuestion {
  id: string
  question_order: number
  question_text: string
  options: string[]
  is_answered: boolean
  selected_option: number
  is_correct?: boolean
  correct_option?: number
  explanation?: string
  citation_page?: number
  citation_excerpt?: string
}

export interface QuizSession {
  session_id: string
  document_id: string
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
  is_correct: boolean
  correct_option: number
  selected_option: number
  explanation: string
  citation_page: number
  citation_excerpt: string
  current_score: number
  total_questions: number
  current_index: number
  completed: boolean
  next_question?: QuizQuestion
}

export interface Flashcard {
  id: string
  set_id: string
  front: string
  back: string
  citation_page: number
  status: 'new' | 'known' | 'practice'
  created_at: string
}

export interface FlashcardSet {
  id: string
  user_id: string
  document_id: string
  conversation_id?: string
  title: string
  topic: string
  created_at: string
  cards: Flashcard[]
}

export interface SummaryResponse {
  document_id: string
  type: string
  summary: string
  citations: Citation[]
}
