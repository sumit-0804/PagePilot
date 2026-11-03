import type {
  AnswerFeedback,
  Conversation,
  Document,
  FlashcardSet,
  Message,
  QuizSession,
  SummaryResponse,
} from '../types'

const API_BASE = '/api'

export const api = {
  // Documents
  async getDocuments(): Promise<Document[]> {
    const res = await fetch(`${API_BASE}/documents`)
    if (!res.ok) throw new Error('Failed to fetch documents')
    return res.json()
  },

  async uploadDocument(file: File): Promise<Document> {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('user_id', 'default_user')

    const res = await fetch(`${API_BASE}/documents/upload`, {
      method: 'POST',
      body: formData,
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Upload failed' }))
      throw new Error(err.error || 'Upload failed')
    }
    return res.json()
  },

  async getDocument(id: string): Promise<Document> {
    const res = await fetch(`${API_BASE}/documents/${id}`)
    if (!res.ok) throw new Error('Failed to fetch document')
    return res.json()
  },

  async getDocumentStatus(id: string): Promise<{ id: string; status: Document['status']; error_message?: string; page_count: number }> {
    const res = await fetch(`${API_BASE}/documents/${id}/status`)
    if (!res.ok) throw new Error('Failed to fetch document status')
    return res.json()
  },

  async deleteDocument(id: string): Promise<void> {
    const res = await fetch(`${API_BASE}/documents/${id}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to delete document')
  },

  getDocumentFileUrl(id: string): string {
    return `${API_BASE}/documents/${id}/file`
  },

  // Conversations
  async getConversations(): Promise<Conversation[]> {
    const res = await fetch(`${API_BASE}/conversations`)
    if (!res.ok) throw new Error('Failed to fetch conversations')
    return res.json()
  },

  async createConversation(documentId?: string, title?: string): Promise<Conversation> {
    const res = await fetch(`${API_BASE}/conversations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: 'default_user',
        document_id: documentId,
        title: title || 'New Study Session',
      }),
    })
    if (!res.ok) throw new Error('Failed to create conversation')
    return res.json()
  },

  async getConversation(id: string): Promise<Conversation> {
    const res = await fetch(`${API_BASE}/conversations/${id}`)
    if (!res.ok) throw new Error('Failed to fetch conversation')
    const data = await res.json()
    // Parse citations_json on messages
    if (data.messages) {
      data.messages = data.messages.map((m: Message) => {
        if (m.citations_json) {
          try {
            m.citations = JSON.parse(m.citations_json)
          } catch {
            m.citations = []
          }
        }
        return m
      })
    }
    return data
  },

  async deleteConversation(id: string): Promise<void> {
    const res = await fetch(`${API_BASE}/conversations/${id}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to delete conversation')
  },

  // Streaming Chat via SSE
  async sendStreamingMessage(
    conversationId: string,
    message: string,
    documentId?: string,
    onToken?: (token: string) => void,
    onDone?: (result: { assistant_message: Message; citations: any[] }) => void,
    onError?: (err: Error) => void
  ): Promise<void> {
    try {
      const response = await fetch(`${API_BASE}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'text/event-stream',
        },
        body: JSON.stringify({
          conversation_id: conversationId,
          document_id: documentId,
          message,
          stream: true,
        }),
      })

      if (!response.ok) {
        throw new Error(`Chat request failed with status ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) throw new Error('ReadableStream not supported')

      const decoder = new TextDecoder('utf-8')
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n\n')
        buffer = lines.pop() || ''

        for (const block of lines) {
          const trimmed = block.trim()
          if (!trimmed.startsWith('data:')) continue

          const jsonStr = trimmed.replace(/^data:\s*/, '')
          try {
            const data = JSON.parse(jsonStr)
            if (data.type === 'token' && onToken) {
              onToken(data.content)
            } else if (data.type === 'done' && onDone) {
              onDone(data)
            } else if (data.type === 'error' && onError) {
              onError(new Error(data.error))
            }
          } catch (e) {
            console.error('SSE JSON parse error:', e, jsonStr)
          }
        }
      }
    } catch (err) {
      if (onError && err instanceof Error) onError(err)
      else throw err
    }
  },

  // Quizzes
  async generateQuiz(
    documentId: string,
    topic: string = '',
    difficulty: string = 'medium',
    numQuestions: number = 5,
    conversationId?: string
  ): Promise<QuizSession> {
    const res = await fetch(`${API_BASE}/quizzes/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        document_id: documentId,
        user_id: 'default_user',
        conversation_id: conversationId,
        topic,
        difficulty,
        num_questions: numQuestions,
      }),
    })
    if (!res.ok) throw new Error('Failed to generate quiz')
    return res.json()
  },

  async getQuiz(sessionId: string): Promise<QuizSession> {
    const res = await fetch(`${API_BASE}/quizzes/${sessionId}`)
    if (!res.ok) throw new Error('Failed to fetch quiz')
    return res.json()
  },

  async answerQuiz(
    sessionId: string,
    questionId: string,
    selectedOption: number
  ): Promise<AnswerFeedback> {
    const res = await fetch(`${API_BASE}/quizzes/${sessionId}/answer`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        question_id: questionId,
        selected_option: selectedOption,
      }),
    })
    if (!res.ok) throw new Error('Failed to submit answer')
    return res.json()
  },

  // Flashcards
  async generateFlashcards(
    documentId: string,
    topic: string = '',
    count: number = 6,
    conversationId?: string
  ): Promise<FlashcardSet> {
    const res = await fetch(`${API_BASE}/flashcards/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        document_id: documentId,
        user_id: 'default_user',
        conversation_id: conversationId,
        topic,
        count,
      }),
    })
    if (!res.ok) throw new Error('Failed to generate flashcards')
    return res.json()
  },

  async getFlashcardSet(setId: string): Promise<FlashcardSet> {
    const res = await fetch(`${API_BASE}/flashcards/${setId}`)
    if (!res.ok) throw new Error('Failed to fetch flashcard set')
    return res.json()
  },

  async updateFlashcardStatus(cardId: string, status: 'known' | 'practice' | 'new'): Promise<void> {
    const res = await fetch(`${API_BASE}/flashcards/${cardId}/status`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status }),
    })
    if (!res.ok) throw new Error('Failed to update flashcard status')
  },

  // Summaries
  async generateSummary(
    documentId: string,
    type: string = 'complete',
    topic: string = ''
  ): Promise<SummaryResponse> {
    const res = await fetch(`${API_BASE}/summaries/generate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        document_id: documentId,
        type,
        topic,
      }),
    })
    if (!res.ok) throw new Error('Failed to generate summary')
    return res.json()
  },
}
