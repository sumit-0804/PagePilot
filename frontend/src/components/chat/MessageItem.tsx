import { memo } from 'react'
import { Markdown } from '@/components/chat/Markdown'
import { CitationChip } from '@/components/chat/CitationChip'
import { QuizBlock } from '@/components/quiz/QuizBlock'
import { FlashcardDeck } from '@/components/flashcards/FlashcardDeck'
import { parseCitations, parseMetadata } from '@/services/api'
import type { Citation, Message } from '@/types'

export function UserBubble({ text }: { text: string }) {
  return (
    <div className="flex justify-end">
      <p className="max-w-[85%] rounded-2xl rounded-br-md bg-secondary px-4 py-2.5 whitespace-pre-wrap">{text}</p>
    </div>
  )
}

function Sources({ citations, documentId }: { citations: Citation[]; documentId: string }) {
  if (citations.length === 0) return null
  return (
    <div className="mt-3 flex flex-wrap items-center gap-1.5 text-sm">
      <span className="text-muted-foreground">From your PDF</span>
      {[...citations]
        .sort((a, b) => a.page_number - b.page_number)
        .map((c) => (
          <CitationChip key={c.page_number} documentId={documentId} page={c.page_number} snippet={c.snippet} />
        ))}
    </div>
  )
}

export const MessageItem = memo(function MessageItem({ message, documentId }: { message: Message; documentId: string }) {
  if (message.role === 'user') return <UserBubble text={message.content} />

  const citations = parseCitations(message)
  const meta = parseMetadata(message)

  switch (message.type) {
    case 'quiz':
      return (
        <div className="space-y-3">
          <p className="reading">{message.content}</p>
          {meta.quiz_session_id && <QuizBlock sessionId={meta.quiz_session_id} documentId={documentId} />}
        </div>
      )
    case 'flashcards':
      return (
        <div className="space-y-3">
          <p className="reading">{message.content}</p>
          {meta.flashcard_set_id && <FlashcardDeck setId={meta.flashcard_set_id} documentId={documentId} />}
        </div>
      )
    default:
      return (
        <div>
          <Markdown text={message.content} documentId={documentId} citations={citations} />
          <Sources citations={citations} documentId={documentId} />
        </div>
      )
  }
})
