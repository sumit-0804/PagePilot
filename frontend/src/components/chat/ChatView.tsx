import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { BookOpen, Loader2 } from 'lucide-react'
import { api, ApiError } from '@/services/api'
import { queryKeys, useDocument } from '@/hooks/queries'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/layout/PageHeader'
import { Composer } from '@/components/chat/Composer'
import { Markdown } from '@/components/chat/Markdown'
import { MessageItem, UserBubble } from '@/components/chat/MessageItem'
import { FollowUps, SuggestedPrompts } from '@/components/chat/Suggestions'
import { UploadDialog } from '@/components/documents/UploadDialog'
import type { ExplanationStyle } from '@/types'

interface InFlight {
  question: string
  answer: string
}

export function ChatView({ conversationId }: { conversationId: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const conversation = useQuery({ queryKey: queryKeys.conversation(conversationId), queryFn: () => api.conversations.get(conversationId) })
  const document = useDocument(conversation.data?.document_id)
  const openPdf = useStudyStore((s) => s.openPdf)
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const pending = useStudyStore((s) => s.pendingMessage)
  const setPending = useStudyStore((s) => s.setPendingMessage)

  const [inFlight, setInFlight] = useState<InFlight | null>(null)
  const [sendError, setSendError] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const stickToBottom = useRef(true)
  const sentPending = useRef<string | null>(null)

  const send = useCallback(
    async (text: string, style: ExplanationStyle | '' = defaultStyle) => {
      setSendError(null)
      setInFlight({ question: text, answer: '' })
      stickToBottom.current = true
      const controller = new AbortController()
      abortRef.current = controller
      try {
        await api.chat.stream(
          { conversation_id: conversationId, message: text, style: style || undefined },
          { onToken: (t) => setInFlight((f) => f && { ...f, answer: f.answer + t }), signal: controller.signal },
        )
      } catch (err) {
        if (!controller.signal.aborted) {
          setSendError(err instanceof ApiError || err instanceof Error ? err.message : 'Something went wrong.')
        }
      } finally {
        // The server saved both messages (or at least the question); reload before dropping the live copy.
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: queryKeys.conversation(conversationId) }),
          queryClient.invalidateQueries({ queryKey: queryKeys.conversations }),
        ])
        setInFlight(null)
        abortRef.current = null
      }
    },
    [conversationId, defaultStyle, queryClient],
  )

  // A message typed on the document page is sent once this new conversation opens (once, even in StrictMode).
  useEffect(() => {
    if (!pending || pending.conversationId !== conversationId) return
    const key = `${pending.conversationId}:${pending.text}`
    if (sentPending.current === key) return
    sentPending.current = key
    setPending(null)
    void send(pending.text, pending.style ?? '')
  }, [pending, conversationId, send, setPending])
  // No abort on unmount: leaving mid-answer lets the server finish and save it (and StrictMode's
  // test unmount would otherwise cancel the first message). The Stop button aborts deliberately.

  const messages = conversation.data?.messages ?? []

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el && stickToBottom.current) el.scrollTop = el.scrollHeight
  }, [messages.length, inFlight?.answer, inFlight?.question])

  const onScroll = () => {
    const el = scrollRef.current
    if (el) stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
  }

  if (conversation.isLoading) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto w-full max-w-3xl space-y-4 px-4 py-8" role="status" aria-label="Loading chat">
          <Skeleton className="ml-auto h-10 w-1/2" />
          <Skeleton className="h-24 w-full" />
        </div>
      </div>
    )
  }

  if (conversation.error || !conversation.data) {
    const notFound = conversation.error instanceof ApiError && conversation.error.status === 404
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto max-w-md px-6 py-16">
          <h1 className="font-reading text-2xl font-semibold">{notFound ? 'Chat not found' : "This chat didn't load"}</h1>
          <p className="mt-2 text-muted-foreground">{notFound ? 'It may have been deleted.' : conversation.error?.message}</p>
          <Button className="mt-6" variant="outline" onClick={() => (notFound ? navigate('/') : conversation.refetch())}>
            {notFound ? 'Go to your documents' : 'Try again'}
          </Button>
        </div>
      </div>
    )
  }

  const conv = conversation.data
  const docName = document.data ? displayName(document.data.filename) : 'your document'
  const busy = !!inFlight
  const last = messages[messages.length - 1]
  const showFollowUps = !busy && last?.role === 'assistant' && last.type === 'chat'

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={conv.title}
        actions={
          <Button variant="outline" size="sm" onClick={() => openPdf(conv.document_id)}>
            <BookOpen />
            Open PDF
          </Button>
        }
      />

      <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl space-y-8 px-4 py-8 sm:px-6">
          {messages.length === 0 && !busy && (
            <div>
              <h1 className="font-reading text-3xl font-semibold tracking-tight">{docName}</h1>
              <div className="mt-6">
                <SuggestedPrompts onPick={(t) => send(t)} />
              </div>
            </div>
          )}

          {messages.map((m) => (
            <MessageItem key={m.id} message={m} documentId={conv.document_id} />
          ))}

          {inFlight && (
            <>
              <UserBubble text={inFlight.question} />
              {inFlight.answer ? (
                <Markdown text={inFlight.answer} documentId={conv.document_id} citations={[]} streaming />
              ) : (
                <p className="flex items-center gap-2 text-muted-foreground" role="status">
                  <Loader2 className="size-4 animate-spin" />
                  Reading your document…
                </p>
              )}
            </>
          )}

          {sendError && (
            <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm">
              <p className="font-semibold text-destructive">The answer couldn't be generated</p>
              <p className="mt-1 text-muted-foreground">{sendError}</p>
            </div>
          )}

          {showFollowUps && <FollowUps onPick={(t, style) => send(t, style ?? defaultStyle)} />}
        </div>
      </div>

      <div className="mx-auto w-full max-w-3xl px-4 pb-4 sm:px-6">
        <Composer
          documentName={docName}
          busy={busy}
          onSend={(t, style) => send(t, style)}
          onStop={() => abortRef.current?.abort()}
          onAttach={() => setUploadOpen(true)}
        />
        <p className="mt-2 text-center text-xs text-muted-foreground">Answers can be wrong. Check the cited pages.</p>
      </div>

      <UploadDialog open={uploadOpen} onOpenChange={setUploadOpen} onUploaded={(doc) => navigate(`/documents/${doc.id}`)} />
    </div>
  )
}
