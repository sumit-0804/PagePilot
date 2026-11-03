package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

// Document processing statuses
const (
	StatusUploaded   = "UPLOADED"
	StatusExtracting = "EXTRACTING"
	StatusChunking   = "CHUNKING"
	StatusEmbedding  = "EMBEDDING"
	StatusIndexing   = "INDEXING"
	StatusReady      = "READY"
	StatusFailed     = "FAILED"
)

// Table layouts are defined in supabase/migrations; these structs mirror them.

type Document struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	UserID         string    `json:"user_id"`
	Filename       string    `json:"filename"`
	StoragePath    string    `json:"-"`
	FileSize       int64     `json:"file_size"`
	ContentHash    string    `json:"-"`
	PageCount      int       `json:"page_count"`
	Status         string    `json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	EmbeddingModel string    `json:"embedding_model,omitempty"`
	ChunksTotal    int       `json:"chunks_total"`
	ChunksEmbedded int       `json:"chunks_embedded"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DocumentChunk struct {
	ID           string          `gorm:"primaryKey" json:"id"`
	DocumentID   string          `json:"document_id"`
	UserID       string          `json:"user_id"`
	PageNumber   int             `json:"page_number"`
	ChunkIndex   int             `json:"chunk_index"`
	ChunkText    string          `json:"chunk_text"`
	DocumentName string          `json:"document_name"`
	Embedding    pgvector.Vector `json:"-"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Conversation struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	UserID     string    `json:"user_id"`
	DocumentID string    `json:"document_id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Messages []Message `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
}

// Citation links an answer claim to a specific page
type Citation struct {
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	PageNumber   int    `json:"page_number"`
	Snippet      string `json:"snippet"`
}

type Message struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"` // "user", "assistant", "system"
	Content        string    `json:"content"`
	Type           string    `json:"type"` // "chat", "quiz", "flashcard", "summary", "explanation"
	CitationsJSON  string    `json:"citations_json,omitempty"`
	MetadataJSON   string    `json:"metadata_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type QuizSession struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	UserID         string    `json:"user_id"`
	DocumentID     string    `json:"document_id"`
	ConversationID *string   `json:"conversation_id,omitempty"`
	Topic          string    `json:"topic"`
	Difficulty     string    `json:"difficulty"`
	TotalQuestions int       `json:"total_questions"`
	CurrentIndex   int       `json:"current_index"`
	Score          int       `json:"score"`
	Completed      bool      `json:"completed"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	Questions []QuizQuestion `gorm:"foreignKey:SessionID" json:"questions,omitempty"`
}

type QuizQuestion struct {
	ID              string  `gorm:"primaryKey" json:"id"`
	SessionID       string  `json:"session_id"`
	QuestionOrder   int     `json:"question_order"`
	QuestionType    string  `json:"question_type"`
	QuestionText    string  `json:"question_text"`
	OptionsJSON     string  `json:"options_json"`
	CorrectOption   *int    `json:"correct_option"` // 0-indexed; nil for short answers
	ReferenceAnswer *string `json:"reference_answer,omitempty"`
	Explanation     string  `json:"explanation"`
	CitationPage    *int    `json:"citation_page,omitempty"`
	CitationExcerpt *string `json:"citation_excerpt,omitempty"`

	Answer *QuizAnswer `gorm:"foreignKey:QuestionID" json:"answer,omitempty"`
}

type QuizAnswer struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	QuestionID     string    `json:"question_id"`
	SessionID      string    `json:"session_id"`
	UserID         string    `json:"user_id"`
	SelectedOption *int      `json:"selected_option,omitempty"`
	AnswerText     *string   `json:"answer_text,omitempty"`
	IsCorrect      bool      `json:"is_correct"`
	Feedback       *string   `json:"feedback,omitempty"` // grader feedback for short answers
	AnsweredAt     time.Time `json:"answered_at"`
}

type FlashcardSet struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	UserID         string    `json:"user_id"`
	DocumentID     string    `json:"document_id"`
	ConversationID *string   `json:"conversation_id,omitempty"`
	Title          string    `json:"title"`
	Topic          string    `json:"topic"`
	CreatedAt      time.Time `json:"created_at"`

	Cards []Flashcard `gorm:"foreignKey:SetID" json:"cards,omitempty"`
}

type Flashcard struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	SetID        string    `json:"set_id"`
	Front        string    `json:"front"`
	Back         string    `json:"back"`
	CitationPage *int      `json:"citation_page,omitempty"`
	Status       string    `json:"status"` // "new", "known", "practice"
	CreatedAt    time.Time `json:"created_at"`
}

// Summary caches a generated summary per document, type and topic.
type Summary struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	UserID        string    `json:"user_id"`
	DocumentID    string    `json:"document_id"`
	Type          string    `json:"type"`
	Topic         string    `json:"topic"`
	Content       string    `json:"content"`
	CitationsJSON string    `json:"citations_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
