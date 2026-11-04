import { memo } from 'react'
import { motion } from 'motion/react'
import { BookOpenCheck, FileText, Globe } from 'lucide-react'
import { Markdown } from '@/components/chat/Markdown'
import { CitationChip } from '@/components/chat/CitationChip'
import { sourceName } from '@/lib/citations'
import { QuizBlock } from '@/components/quiz/QuizBlock'
import { FlashcardDeck } from '@/components/flashcards/FlashcardDeck'
import { parseCitations, parseMetadata } from '@/services/api'
import type { Citation, Message } from '@/types'

export function UserBubble({ text }: { text: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8, scale: 0.98 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ duration: 0.22, ease: 'easeOut' }}
      className="flex justify-end"
    >
      <p className="max-w-[85%] rounded-2xl rounded-br-sm border border-primary/20 bg-gradient-to-br from-primary/12 via-accent to-secondary px-4 py-3 text-foreground shadow-2xs whitespace-pre-wrap">
        {text}
      </p>
    </motion.div>
  )
}

function Sources({ citations }: { citations: Citation[] }) {
  if (citations.length === 0) return null
  return (
    <div className="mt-4 rounded-xl border border-border/70 bg-muted/50 px-3 py-2 text-xs">
      <span className="inline-flex items-center gap-1.5 font-semibold text-muted-foreground">
        <BookOpenCheck className="size-3.5 text-primary" />
        Sources
      </span>
      <ol className="mt-1.5 space-y-1">
        {[...citations]
          .sort((a, b) => (a.n ?? a.page_number ?? 0) - (b.n ?? b.page_number ?? 0))
          .map((c, i) => (
            <li key={`${c.n ?? 'p'}-${c.page_number ?? i}`} className="flex min-w-0 items-center gap-2">
              <CitationChip citation={c} />
              {c.kind === 'web' ? (
                <Globe className="size-3 shrink-0 text-muted-foreground" />
              ) : (
                <FileText className="size-3 shrink-0 text-muted-foreground" />
              )}
              <span className="min-w-0 truncate text-muted-foreground">{sourceName(c)}</span>
            </li>
          ))}
      </ol>
    </div>
  )
}

export const MessageItem = memo(function MessageItem({ message }: { message: Message }) {
  if (message.role === 'user') return <UserBubble text={message.content} />

  const citations = parseCitations(message)
  const meta = parseMetadata(message)

  switch (message.type) {
    case 'quiz':
      return (
        <motion.div
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          className="space-y-4"
        >
          <p className="reading">{message.content}</p>
          {meta.quiz_session_id && <QuizBlock sessionId={meta.quiz_session_id} />}
        </motion.div>
      )
    case 'flashcards':
      return (
        <motion.div
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          className="space-y-4"
        >
          <p className="reading">{message.content}</p>
          {meta.flashcard_set_id && <FlashcardDeck setId={meta.flashcard_set_id} />}
        </motion.div>
      )
    default:
      return (
        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.25 }}
          className="rounded-2xl border border-border/60 bg-card/70 p-5 shadow-2xs backdrop-blur-xs"
        >
          <Markdown text={message.content} citations={citations} />
          <Sources citations={citations} />
        </motion.div>
      )
  }
})
