import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { Link, useLocation } from 'react-router-dom'
import { ArrowUp, Check, FilePlus, Loader2, LogIn, Paperclip, SlidersHorizontal, Square, UserPlus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { GlowContainer } from '@/components/ui/aceternity/moving-border'
import { FormatIcon } from '@/components/documents/FormatIcon'
import { useStudyStore } from '@/stores/useStudyStore'
import { useAuth } from '@/hooks/useAuth'
import { useDocuments } from '@/hooks/queries'
import { displayName, STYLE_OPTIONS, statusLabel, styleLabel } from '@/lib/format'
import type { ExplanationStyle, StudyDocument } from '@/types'

interface ComposerProps {
  onSend: (text: string, style: ExplanationStyle | '') => void
  onStop?: () => void
  busy: boolean
  disabled?: boolean
  autoFocus?: boolean
  /** Documents attached to this chat, including uploads still being processed. */
  documents: StudyDocument[]
  /** Leave out when the attached documents are fixed (e.g. on a document page). */
  onRemoveDocument?: (id: string) => void
  onAttachExisting?: (doc: StudyDocument) => void
  onUploadNew?: () => void
}

const PLACEHOLDERS = [
  'Ask anything…',
  'Ask about your attached documents…',
  'Quiz me on photosynthesis…',
  'Make flashcards for key definitions…',
]

export function Composer({
  onSend,
  onStop,
  busy,
  disabled = false,
  autoFocus = false,
  documents,
  onRemoveDocument,
  onAttachExisting,
  onUploadNew,
}: ComposerProps) {
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const [text, setText] = useState('')
  const [style, setStyle] = useState<ExplanationStyle | ''>(defaultStyle)
  const [hintIndex, setHintIndex] = useState(0)
  const ref = useRef<HTMLTextAreaElement>(null)
  const placeholders = documents.length > 0 ? PLACEHOLDERS.slice(1) : PLACEHOLDERS

  useEffect(() => {
    if (text.length > 0) return
    const timer = setInterval(() => {
      setHintIndex((i) => (i + 1) % placeholders.length)
    }, 4000)
    return () => clearInterval(timer)
  }, [text.length, placeholders.length])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const fit = () => {
      el.style.height = 'auto'
      el.style.height = `${Math.min(el.scrollHeight, 220)}px`
    }
    fit()
    // Refit when the width changes too (e.g. the sidebar animating open at mount).
    const observer = new ResizeObserver(fit)
    if (el.parentElement) observer.observe(el.parentElement)
    return () => observer.disconnect()
  }, [text])

  const canSend = text.trim().length > 0 && !busy && !disabled

  function send() {
    if (!canSend) return
    onSend(text.trim(), style)
    setText('')
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter adds a line. Ignore Enter while an IME is composing.
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      send()
    }
  }

  return (
    <GlowContainer active={busy}>
      <div className="relative rounded-2xl border border-border/90 bg-card/95 shadow-md shadow-foreground/[0.03] backdrop-blur-md transition-colors focus-within:border-primary/60">
        <label htmlFor="composer" className="sr-only">
          Message
        </label>

        <div className="relative">
          {/* Aceternity animated rotating placeholder when textarea is empty */}
          <AnimatePresence mode="wait">
            {text.length === 0 && (
              <motion.span
                key={hintIndex}
                initial={{ opacity: 0, y: 5 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -5 }}
                transition={{ duration: 0.22 }}
                className="pointer-events-none absolute top-3.5 right-4 left-4 truncate text-base text-muted-foreground select-none"
                aria-hidden
              >
                {placeholders[hintIndex % placeholders.length]}
              </motion.span>
            )}
          </AnimatePresence>

          <textarea
            id="composer"
            ref={ref}
            rows={1}
            autoFocus={autoFocus}
            value={text}
            disabled={disabled}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Ask anything…"
            className="relative z-10 block max-h-56 w-full resize-none bg-transparent px-4 pt-3.5 pb-2 text-base outline-none placeholder:text-transparent disabled:opacity-60"
          />
        </div>

        <div className="flex items-center gap-1.5 px-2.5 pb-2.5">
          {onUploadNew && onAttachExisting && (
            <AttachPicker attached={documents} onUploadNew={onUploadNew} onAttachExisting={onAttachExisting} />
          )}

          {documents.length > 0 && (
            <ul className="flex min-w-0 flex-wrap items-center gap-1.5" aria-label="Attached documents">
              {documents.map((doc) => {
                const name = displayName(doc.filename)
                const processing = doc.status !== 'READY'
                return (
                  <li
                    key={doc.id}
                    className="flex min-w-0 items-center gap-1.5 rounded-lg border border-border/60 bg-muted/70 py-1 pr-1 pl-2.5 text-xs font-medium text-muted-foreground"
                    title={processing ? `${name} · ${statusLabel(doc)}` : name}
                  >
                    {processing ? (
                      <Loader2 className="size-3.5 shrink-0 animate-spin text-primary" aria-label="Processing" />
                    ) : (
                      <FormatIcon format={doc.format} className="size-3.5 shrink-0 text-primary" />
                    )}
                    <span className="max-w-32 truncate sm:max-w-48">{name}</span>
                    {onRemoveDocument && (
                      <button
                        type="button"
                        onClick={() => onRemoveDocument(doc.id)}
                        aria-label={`Remove ${name} from this chat`}
                        className="rounded-md p-0.5 hover:bg-background hover:text-foreground"
                      >
                        <X className="size-3" />
                      </button>
                    )}
                  </li>
                )
              })}
            </ul>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto rounded-xl text-xs font-medium text-muted-foreground hover:text-foreground"
                aria-label={`Explanation style: ${styleLabel(style)}`}
              >
                <SlidersHorizontal className="size-3.5 text-primary" />
                {styleLabel(style)}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64 rounded-xl">
              <DropdownMenuLabel>How should answers explain?</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => setStyle('')}>
                <span className="flex-1">Default</span>
                {style === '' && <Check className="text-primary" />}
              </DropdownMenuItem>
              {STYLE_OPTIONS.map((opt) => (
                <DropdownMenuItem key={opt.value} onSelect={() => setStyle(opt.value)}>
                  <span className="flex-1">
                    <span className="block font-medium">{opt.label}</span>
                    <span className="block text-xs text-muted-foreground">{opt.hint}</span>
                  </span>
                  {style === opt.value && <Check className="text-primary" />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          {busy && onStop ? (
            <motion.div whileTap={{ scale: 0.92 }}>
              <Button
                size="icon"
                variant="secondary"
                className="rounded-xl"
                aria-label="Stop generating"
                onClick={onStop}
              >
                <Square className="size-3.5 fill-current" />
              </Button>
            </motion.div>
          ) : (
            <motion.div whileHover={canSend ? { scale: 1.05 } : undefined} whileTap={canSend ? { scale: 0.94 } : undefined}>
              <Button
                size="icon"
                className="rounded-xl shadow-sm shadow-primary/20"
                aria-label="Send"
                disabled={!canSend}
                onClick={send}
              >
                <ArrowUp />
              </Button>
            </motion.div>
          )}
        </div>
      </div>
    </GlowContainer>
  )
}

/** The paperclip: upload a new document or attach one already uploaded. Guests are sent to log in. */
function AttachPicker({
  attached,
  onUploadNew,
  onAttachExisting,
}: {
  attached: StudyDocument[]
  onUploadNew: () => void
  onAttachExisting: (doc: StudyDocument) => void
}) {
  const { isAccount } = useAuth()
  const location = useLocation()
  const documents = useDocuments()
  const attachedIds = new Set(attached.map((d) => d.id))
  const available = documents.data?.filter((d) => d.status === 'READY' && !attachedIds.has(d.id)) ?? []

  if (!isAccount) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button asChild variant="ghost" size="icon-sm" className="rounded-xl text-muted-foreground hover:text-foreground">
            <Link to="/login" state={{ from: location.pathname }} aria-label="Log in to attach documents">
              <Paperclip />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Log in to attach documents</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Attach a document"
          className="shrink-0 rounded-xl text-muted-foreground hover:text-foreground"
        >
          <Paperclip />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72 rounded-xl">
        <DropdownMenuItem onSelect={onUploadNew}>
          <FilePlus className="text-primary" />
          Upload new
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>Attach existing</DropdownMenuLabel>
        {documents.isLoading && <DropdownMenuItem disabled>Loading…</DropdownMenuItem>}
        {!documents.isLoading && available.length === 0 && (
          <DropdownMenuItem disabled>No other ready documents</DropdownMenuItem>
        )}
        {available.map((doc) => (
          <DropdownMenuItem key={doc.id} onSelect={() => onAttachExisting(doc)}>
            <FormatIcon format={doc.format} />
            <span className="truncate">{displayName(doc.filename)}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** Shown in place of the composer once a guest has used their free message. */
export function LoginPrompt() {
  const location = useLocation()
  const state = { from: location.pathname }
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      className="rounded-2xl border border-primary/30 bg-card/95 p-5 shadow-md backdrop-blur-md"
      role="status"
    >
      <p className="font-reading text-lg font-semibold">Sign up or log in to keep chatting</p>
      <p className="mt-1 text-sm text-muted-foreground">
        Guests get one free message. Your chat so far moves to your account.
      </p>
      <div className="mt-4 flex flex-wrap gap-2">
        <Button asChild className="rounded-xl">
          <Link to="/signup" state={state}>
            <UserPlus />
            Create account
          </Link>
        </Button>
        <Button asChild variant="outline" className="rounded-xl">
          <Link to="/login" state={state}>
            <LogIn />
            Log in
          </Link>
        </Button>
      </div>
    </motion.div>
  )
}
