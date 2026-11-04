import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ComponentType } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'motion/react'
import { toast } from 'sonner'
import { ArrowDown, BookOpen, Loader2, Sparkles } from 'lucide-react'
import { api, ApiError, isLoginRequired } from '@/services/api'
import { supabase } from '@/lib/supabase'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { useAuth } from '@/hooks/useAuth'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'
import { PageHeader } from '@/components/layout/PageHeader'
import { Composer, LoginPrompt } from '@/components/chat/Composer'
import { Markdown } from '@/components/chat/Markdown'
import { MessageItem, UserBubble } from '@/components/chat/MessageItem'
import { FollowUps, SuggestedPrompts } from '@/components/chat/Suggestions'
import { UploadDialog } from '@/components/documents/UploadDialog'
import { FormatIcon } from '@/components/documents/FormatIcon'
import type { ExplanationStyle, StudyDocument } from '@/types'

interface InFlight {
  question: string
  answer: string
  /** Latest tool status from the agent, e.g. "Searching the web…". */
  status?: string
}

export type SendFn = (text: string, style?: ExplanationStyle | '') => void

/**
 * One chat. Without a conversationId it is a new chat (the home page): the first send creates
 * the conversation and then opens it at /chat/:id.
 */
export function ChatView({
  conversationId,
  emptyState: EmptyState,
}: {
  conversationId?: string
  /** Shown before the first message; its prompts are sent as written. */
  emptyState?: ComponentType<{ onPick: SendFn }>
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { session, loading: authLoading, isGuest } = useAuth()
  const conversation = useQuery({
    queryKey: queryKeys.conversation(conversationId ?? ''),
    queryFn: () => api.conversations.get(conversationId!),
    enabled: !!conversationId && !!session,
  })
  const documents = useDocuments()
  const openViewer = useStudyStore((s) => s.openViewer)
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const pending = useStudyStore((s) => s.pendingMessage)
  const setPending = useStudyStore((s) => s.setPendingMessage)

  const [inFlight, setInFlight] = useState<InFlight | null>(null)
  const [sendError, setSendError] = useState<string | null>(null)
  const [loginRequired, setLoginRequired] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [showJumpBottom, setShowJumpBottom] = useState(false)
  // A new chat keeps its document ids locally until the first send creates the conversation.
  const [draftIds, setDraftIds] = useState<string[]>([])
  // Uploads from this chat that get attached once they are READY.
  const [waitingIds, setWaitingIds] = useState<string[]>([])
  const settledUploads = useRef(new Set<string>())
  const abortRef = useRef<AbortController | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const stickToBottom = useRef(true)
  const sentPending = useRef<string | null>(null)

  const lookup = (ids: string[]) =>
    ids.map((id) => documents.data?.find((d) => d.id === id)).filter((d): d is StudyDocument => !!d && d.status !== 'FAILED')
  // In a new chat, uploads count as attached right away and are sent once READY.
  const attachedDocs = conversationId ? (conversation.data?.documents ?? []) : lookup([...new Set([...draftIds, ...waitingIds])])

  const refreshConversation = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.conversation(conversationId ?? '') }),
    [conversationId, queryClient],
  )

  const attach = useMutation({
    mutationFn: (doc: StudyDocument) => api.conversations.attachDocument(conversationId!, doc.id),
    onSuccess: () => refreshConversation(),
    onError: (err) => toast.error(err.message),
  })
  const detach = useMutation({
    mutationFn: (id: string) => api.conversations.detachDocument(conversationId!, id),
    onSuccess: () => refreshConversation(),
    onError: (err) => toast.error(err.message),
  })

  function attachDoc(doc: StudyDocument) {
    if (conversationId) attach.mutate(doc)
    else setDraftIds((ids) => (ids.includes(doc.id) ? ids : [...ids, doc.id]))
  }

  function removeDoc(id: string) {
    setWaitingIds((ids) => ids.filter((x) => x !== id))
    if (conversationId) detach.mutate(id)
    else setDraftIds((ids) => ids.filter((x) => x !== id))
  }

  // Once an upload finishes: attach it to an existing chat, or report that it failed.
  const attachMutate = attach.mutate
  useEffect(() => {
    for (const doc of documents.data ?? []) {
      if (!waitingIds.includes(doc.id) || settledUploads.current.has(doc.id)) continue
      if (doc.status === 'FAILED') {
        settledUploads.current.add(doc.id)
        toast.error(`${displayName(doc.filename)} couldn't be processed${doc.error_message ? `: ${doc.error_message}` : '.'}`)
      } else if (doc.status === 'READY' && conversationId) {
        settledUploads.current.add(doc.id)
        attachMutate(doc)
      }
    }
  }, [documents.data, waitingIds, conversationId, attachMutate])

  // In an existing chat, uploads still processing show as chips beside the attached documents.
  const chipDocs = conversationId
    ? [...attachedDocs, ...lookup(waitingIds).filter((d) => d.status !== 'READY' && !attachedDocs.some((a) => a.id === d.id))]
    : attachedDocs

  // Documents still processing aren't sent; the server only accepts READY ones.
  const draftReadyKey = conversationId ? '' : attachedDocs.filter((d) => d.status === 'READY').map((d) => d.id).join(',')
  const draftReadyIds = useMemo(() => (draftReadyKey ? draftReadyKey.split(',') : []), [draftReadyKey])

  const send = useCallback(
    async (text: string, style: ExplanationStyle | '' = defaultStyle) => {
      setSendError(null)
      setInFlight({ question: text, answer: '' })
      stickToBottom.current = true
      const controller = new AbortController()
      abortRef.current = controller
      let newConversationId: string | undefined
      try {
        if (!session) {
          // Visitors chat as Supabase anonymous users so the server can count their free message.
          const { error } = await supabase.auth.signInAnonymously()
          if (error) throw new Error(`Couldn't start a guest session: ${error.message}`)
        }
        const done = await api.chat.stream(
          {
            ...(conversationId
              ? { conversation_id: conversationId }
              : { document_ids: draftReadyIds }),
            message: text,
            style: style || undefined,
          },
          {
            onToken: (t) => setInFlight((f) => f && { ...f, answer: f.answer + t }),
            onStatus: (status) => setInFlight((f) => f && { ...f, status }),
            signal: controller.signal,
          },
        )
        if (!conversationId) newConversationId = done.conversation_id
      } catch (err) {
        if (isLoginRequired(err)) setLoginRequired(true)
        else if (!controller.signal.aborted) {
          setSendError(err instanceof Error ? err.message : 'Something went wrong.')
        }
      } finally {
        abortRef.current = null
        void queryClient.invalidateQueries({ queryKey: queryKeys.conversations })
        if (newConversationId) {
          // Load the saved chat first so the new page opens without a loading flash.
          await queryClient
            .prefetchQuery({
              queryKey: queryKeys.conversation(newConversationId),
              queryFn: () => api.conversations.get(newConversationId!),
            })
          navigate(`/chat/${newConversationId}`)
        } else {
          // The server saved both messages (or at least the question); reload before dropping the live copy.
          if (conversationId) await refreshConversation()
          setInFlight(null)
        }
      }
    },
    [conversationId, defaultStyle, draftReadyIds, navigate, queryClient, refreshConversation, session],
  )

  // A message typed on a document page is sent once this new conversation opens (once, even in StrictMode).
  useEffect(() => {
    if (!pending || pending.conversationId !== conversationId) return
    const key = `${pending.conversationId}:${pending.text}`
    if (sentPending.current === key) return
    sentPending.current = key
    setPending(null)
    void send(pending.text, pending.style ?? '')
  }, [pending, conversationId, send, setPending])

  const messages = conversation.data?.messages ?? []

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el && stickToBottom.current) el.scrollTop = el.scrollHeight
  }, [messages.length, inFlight?.answer, inFlight?.question])

  const onScroll = () => {
    const el = scrollRef.current
    if (!el) return
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 90
    stickToBottom.current = nearBottom
    setShowJumpBottom(!nearBottom && messages.length > 1)
  }

  const scrollToBottom = () => {
    const el = scrollRef.current
    if (!el) return
    stickToBottom.current = true
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    setShowJumpBottom(false)
  }

  if (conversationId && (authLoading || conversation.isLoading)) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto w-full max-w-3xl space-y-4 px-4 py-8" role="status" aria-label="Loading chat">
          <Skeleton className="ml-auto h-12 w-1/2 rounded-2xl" />
          <Skeleton className="h-28 w-full rounded-2xl" />
        </div>
      </div>
    )
  }

  if (conversationId && (conversation.error || !conversation.data)) {
    const status = conversation.error instanceof ApiError ? conversation.error.status : 0
    const notFound = status === 404
    const signedOut = !session || status === 401
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto max-w-md px-6 py-16 text-center">
          <h1 className="font-reading text-2xl font-semibold">
            {notFound ? 'Chat not found' : signedOut ? 'Log in to see this chat' : "This chat didn't load"}
          </h1>
          <p className="mt-2 text-muted-foreground">
            {notFound ? 'It may have been deleted.' : signedOut ? 'Chats belong to the account that started them.' : conversation.error?.message}
          </p>
          <Button
            className="mt-6 rounded-xl"
            variant="outline"
            onClick={() =>
              notFound ? navigate('/') : signedOut ? navigate('/login', { state: { from: `/chat/${conversationId}` } }) : conversation.refetch()
            }
          >
            {notFound ? 'Start a new chat' : signedOut ? 'Log in' : 'Try again'}
          </Button>
        </div>
      </div>
    )
  }

  const busy = !!inFlight
  const last = messages[messages.length - 1]
  const showFollowUps = !busy && last?.role === 'assistant' && last.type === 'chat'
  // Guests get one message; after it (or when the server says so) they must log in.
  const mustLogIn = loginRequired || (isGuest && !busy && messages.some((m) => m.role === 'user'))
  const pickPrompt: SendFn = (t, style) => void send(t, style ?? defaultStyle)
  const readyDocs = attachedDocs.filter((d) => d.status === 'READY')

  return (
    <div className="relative flex h-full flex-col">
      <BackgroundAmbient variant="dots" className="opacity-40" />

      <PageHeader
        title={conversation.data?.title}
        actions={
          readyDocs.length === 1 ? (
            <Button
              variant="outline"
              size="sm"
              className="rounded-xl border-primary/30 bg-card/80 hover:border-primary hover:bg-accent"
              onClick={() => openViewer(readyDocs[0].id, { format: readyDocs[0].format })}
            >
              <BookOpen className="text-primary" />
              Open document
            </Button>
          ) : readyDocs.length > 1 ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-xl border-primary/30 bg-card/80 hover:border-primary hover:bg-accent"
                >
                  <BookOpen className="text-primary" />
                  Open document
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-64 rounded-xl">
                {readyDocs.map((doc) => (
                  <DropdownMenuItem key={doc.id} onSelect={() => openViewer(doc.id, { format: doc.format })}>
                    <FormatIcon format={doc.format} />
                    <span className="truncate">{displayName(doc.filename)}</span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          ) : undefined
        }
      />

      <div ref={scrollRef} onScroll={onScroll} className="relative z-10 min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-5xl space-y-7 px-6 py-8 lg:px-10">
          {messages.length === 0 && !busy && (
            EmptyState ? (
              <EmptyState onPick={pickPrompt} />
            ) : (
              <motion.div
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                className="rounded-2xl border border-border/80 bg-card/80 p-6 shadow-xs backdrop-blur-sm"
              >
                <div className="inline-flex items-center gap-1.5 rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
                  <Sparkles className="size-3.5" />
                  Ready to explore
                </div>
                <h1 className="mt-3 font-reading text-3xl font-semibold tracking-tight">{conversation.data?.title}</h1>
                <p className="mt-1 text-sm text-muted-foreground">
                  Ask any question below, or pick a prompt to get summaries, quizzes, or 3D flashcards.
                </p>
                <div className="mt-6">
                  <SuggestedPrompts variant={attachedDocs.length > 0 ? 'document' : 'general'} onPick={(t) => pickPrompt(t)} />
                </div>
              </motion.div>
            )
          )}

          {messages.map((m) => (
            <MessageItem key={m.id} message={m} />
          ))}

          {inFlight && (
            <>
              <UserBubble text={inFlight.question} />
              {inFlight.answer ? (
                <div className="rounded-2xl border border-primary/30 bg-card/85 p-5 shadow-sm backdrop-blur-xs">
                  <Markdown text={inFlight.answer} citations={[]} streaming />
                </div>
              ) : (
                <motion.div
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  className="flex items-center gap-3 rounded-2xl border border-primary/25 bg-card/80 px-4 py-3.5 text-sm text-muted-foreground shadow-2xs"
                  role="status"
                >
                  <span className="flex size-7 items-center justify-center rounded-lg bg-primary/15 text-primary">
                    <Loader2 className="size-4 animate-spin" />
                  </span>
                  <span className="font-medium text-foreground/85">{inFlight.status || 'Thinking…'}</span>
                </motion.div>
              )}
            </>
          )}

          {sendError && (
            <motion.div
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              role="alert"
              className="rounded-2xl border border-destructive/35 bg-destructive/8 px-4 py-3.5 text-sm"
            >
              <p className="font-semibold text-destructive">The answer couldn't be generated</p>
              <p className="mt-1 text-muted-foreground">{sendError}</p>
            </motion.div>
          )}

          {showFollowUps && !mustLogIn && <FollowUps onPick={pickPrompt} />}
        </div>
      </div>

      {/* Floating scroll-to-bottom pill */}
      <AnimatePresence>
        {showJumpBottom && (
          <motion.div
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 10 }}
            className="pointer-events-none absolute right-0 bottom-28 left-0 z-20 flex justify-center"
          >
            <Button
              size="sm"
              variant="secondary"
              onClick={scrollToBottom}
              className="pointer-events-auto rounded-full border border-border shadow-md"
            >
              <ArrowDown className="size-3.5" />
              Latest message
            </Button>
          </motion.div>
        )}
      </AnimatePresence>

      <div className="relative z-10 mx-auto w-full max-w-5xl px-6 pt-2 pb-4 lg:px-10">
        {mustLogIn ? (
          <LoginPrompt />
        ) : (
          <>
            <Composer
              autoFocus={!conversationId}
              busy={busy}
              documents={chipDocs}
              onSend={(t, style) => void send(t, style)}
              onStop={() => abortRef.current?.abort()}
              onRemoveDocument={removeDoc}
              onAttachExisting={attachDoc}
              onUploadNew={() => setUploadOpen(true)}
            />
            <p className="mt-2 text-center text-xs text-muted-foreground">Answers can be wrong. Check the cited sources.</p>
          </>
        )}
      </div>

      <UploadDialog
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        onUploaded={(doc) => setWaitingIds((ids) => (ids.includes(doc.id) ? ids : [...ids, doc.id]))}
      />
    </div>
  )
}
