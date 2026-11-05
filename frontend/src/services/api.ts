import { noteApiProblem, pathOnly, reportApiFailure } from '@/lib/sentry'
import { parseSSE } from '@/lib/sse'
import { supabase } from '@/lib/supabase'
import type {
  AnswerFeedback,
  ChatDoneEvent,
  Citation,
  Conversation,
  DocumentSection,
  DocumentStatusResponse,
  ExplanationStyle,
  FlashcardSet,
  CardStatus,
  FollowUp,
  FollowUpKind,
  Message,
  MessageRating,
  Progress,
  QuestionType,
  QuizSession,
  StudyDocument,
  SummaryResponse,
} from '@/types'

const API_BASE = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '')

export class ApiError extends Error {
  readonly status: number
  /** The backend's trace id (traced AI routes only), to find the request in Langfuse and Sentry. */
  readonly traceId?: string
  constructor(status: number, message: string, traceId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.traceId = traceId
  }
}

const traceIdOf = (res: Response) => res.headers.get('X-Trace-Id') ?? undefined

async function authHeader(): Promise<Record<string, string>> {
  // getSession refreshes an expired access token when needed.
  const { data } = await supabase.auth.getSession()
  const token = data.session?.access_token
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...(await authHeader()), ...(init.headers as Record<string, string>) }
  if (init.body && !(init.body instanceof FormData)) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(API_BASE + path, { ...init, headers })
  } catch {
    // Usually the user is offline or the server is down; not an app fault worth an alert.
    noteApiProblem('request could not reach the server', pathOnly(path))
    throw new ApiError(0, "Can't reach the PagePilot server. Check that the backend is running.")
  }
  const text = await res.text()
  const body = text ? safeJSON(text) : null
  if (!res.ok) {
    const err = new ApiError(res.status, errorMessage(body, `Request failed (${res.status})`), traceIdOf(res))
    if (res.status >= 500) reportApiFailure(err, { path: pathOnly(path), status: res.status, traceId: err.traceId })
    throw err
  }
  // A non-JSON success (e.g. a host's HTML page) means the request never reached the PagePilot API.
  if (typeof body === 'string') throw new ApiError(res.status, "The PagePilot server sent an unexpected response. Check that VITE_API_URL points to the API.")
  return body as T
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

/** The API reports errors as {"error": "..."}. */
function errorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') return body.error
  return fallback
}

const json = (body: unknown) => JSON.stringify(body)

/** The API's error code when a guest has used their free message. */
export const LOGIN_REQUIRED = 'login_required'

export const isLoginRequired = (err: unknown) => err instanceof ApiError && err.message === LOGIN_REQUIRED

export interface StreamHandlers {
  onToken: (token: string) => void
  /** Progress notes such as "Searching the web…", sent when the agent starts a tool. */
  onStatus?: (status: string) => void
  signal?: AbortSignal
}

export interface ChatRequest {
  /** Leave out to start a new conversation; the "done" event carries its id. */
  conversation_id?: string
  document_ids?: string[]
  message: string
  style?: ExplanationStyle
}

/** Sends a chat message and streams the reply over SSE. Resolves with the final "done" event. */
async function streamChat(body: ChatRequest, handlers: StreamHandlers): Promise<ChatDoneEvent> {
  try {
    return await readChatStream(body, handlers)
  } catch (err) {
    reportStreamFailure(err)
    throw err
  }
}

/** Reports chat failures that point at a fault: server errors, broken streams, unreadable events. */
function reportStreamFailure(err: unknown) {
  if (err instanceof DOMException && err.name === 'AbortError') return // the user stopped the answer
  if (err instanceof ApiError) {
    if (err.status >= 500) reportApiFailure(err, { path: '/chat', status: err.status, traceId: err.traceId })
    return
  }
  if (err instanceof TypeError) {
    noteApiProblem('chat stream lost its connection', '/chat') // offline or a dropped connection
    return
  }
  if (err instanceof Error) reportApiFailure(err, { path: '/chat', status: 0 })
}

async function readChatStream(body: ChatRequest, { onToken, onStatus, signal }: StreamHandlers): Promise<ChatDoneEvent> {
  const res = await fetch(`${API_BASE}/chat`, {
    method: 'POST',
    headers: { ...(await authHeader()), 'Content-Type': 'application/json', Accept: 'text/event-stream' },
    body: json({ ...body, stream: true }),
    signal,
  })
  const traceId = traceIdOf(res)
  if (!res.ok || !res.body) {
    throw new ApiError(res.status, errorMessage(safeJSON(await res.text()), `Chat failed (${res.status})`), traceId)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const { payloads, rest } = parseSSE(buffer)
    buffer = rest
    for (const payload of payloads) {
      if (payload.type === 'token') onToken(String(payload.content))
      else if (payload.type === 'status') onStatus?.(String(payload.content))
      else if (payload.type === 'error') {
        throw new ApiError(payload.error === LOGIN_REQUIRED ? 403 : 500, String(payload.error), traceId)
      } else if (payload.type === 'done') return payload as unknown as ChatDoneEvent
    }
  }
  throw new ApiError(500, 'The answer stream ended unexpectedly. Please try again.', traceId)
}

export const api = {
  documents: {
    list: () => request<StudyDocument[]>('/documents'),
    get: (id: string) => request<StudyDocument>(`/documents/${id}`),
    status: (id: string) => request<DocumentStatusResponse>(`/documents/${id}/status`),
    upload: (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return request<StudyDocument>('/documents/upload', { method: 'POST', body: form })
    },
    fileUrl: (id: string) => request<{ url: string }>(`/documents/${id}/file`),
    section: (id: string, n: number) => request<DocumentSection>(`/documents/${id}/sections/${n}`),
    remove: (id: string) => request<void>(`/documents/${id}`, { method: 'DELETE' }),
  },
  conversations: {
    list: () => request<Conversation[]>('/conversations'),
    get: (id: string) => request<Conversation>(`/conversations/${id}`),
    create: (body: { document_ids?: string[]; title?: string }) =>
      request<Conversation>('/conversations', { method: 'POST', body: json(body) }),
    attachDocument: (id: string, documentId: string) =>
      request<void>(`/conversations/${id}/documents`, { method: 'POST', body: json({ document_id: documentId }) }),
    detachDocument: (id: string, documentId: string) =>
      request<void>(`/conversations/${id}/documents/${documentId}`, { method: 'DELETE' }),
    remove: (id: string) => request<void>(`/conversations/${id}`, { method: 'DELETE' }),
  },
  account: {
    /** Moves a guest's chats to the signed-in account. */
    claim: (guestToken: string) => request<void>('/account/claim', { method: 'POST', body: json({ guest_token: guestToken }) }),
  },
  chat: {
    stream: streamChat,
  },
  messages: {
    rate: (id: string, rating: MessageRating) =>
      request<MessageRating>(`/messages/${id}/feedback`, { method: 'PUT', body: json(rating) }),
    clearRating: (id: string) => request<void>(`/messages/${id}/feedback`, { method: 'DELETE' }),
  },
  quizzes: {
    get: (id: string) => request<QuizSession>(`/quizzes/${id}`),
    generate: (body: { document_id: string; topic?: string; difficulty?: string; num_questions?: number; question_types?: QuestionType[] }) =>
      request<QuizSession>('/quizzes/generate', { method: 'POST', body: json(body) }),
    answer: (sessionId: string, body: { question_id: string; selected_option?: number; answer_text?: string }) =>
      request<AnswerFeedback>(`/quizzes/${sessionId}/answer`, { method: 'POST', body: json(body) }),
  },
  flashcards: {
    get: (id: string) => request<FlashcardSet>(`/flashcards/${id}`),
    setCardStatus: (cardId: string, status: CardStatus) =>
      request<void>(`/flashcards/cards/${cardId}`, { method: 'PATCH', body: json({ status }) }),
  },
  summaries: {
    generate: (body: { document_id: string; type?: string; topic?: string; refresh?: boolean }) =>
      request<SummaryResponse>('/summaries/generate', { method: 'POST', body: json(body) }),
  },
  progress: {
    get: (documentId: string) => request<Progress>(`/progress?document_id=${encodeURIComponent(documentId)}`),
  },
}

export function parseCitations(message: Message): Citation[] {
  if (!message.citations_json) return []
  try {
    const parsed: unknown = JSON.parse(message.citations_json)
    return Array.isArray(parsed) ? (parsed as Citation[]) : []
  } catch {
    return []
  }
}

export function parseMetadata(message: Message): Record<string, string> {
  if (!message.metadata_json) return {}
  try {
    return JSON.parse(message.metadata_json) as Record<string, string>
  } catch {
    return {}
  }
}

const FOLLOW_UP_KINDS: FollowUpKind[] = ['simpler', 'example', 'deeper', 'quiz', 'flashcards', 'question']

/** Follow-ups the server suggested for this answer; [] when there are none (older messages, failures). */
export function parseFollowUps(message: Message): FollowUp[] {
  if (!message.metadata_json) return []
  try {
    const raw = (JSON.parse(message.metadata_json) as { follow_ups?: unknown }).follow_ups
    if (!Array.isArray(raw)) return []
    return raw.filter(
      (f): f is FollowUp =>
        typeof f?.text === 'string' && f.text.trim() !== '' && FOLLOW_UP_KINDS.includes(f.kind),
    )
  } catch {
    return []
  }
}
