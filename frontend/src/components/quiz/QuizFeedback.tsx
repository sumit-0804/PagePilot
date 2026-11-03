import { CircleCheck, CircleX } from 'lucide-react'
import { QuizCitation } from '@/components/quiz/QuizCitation'
import { cn } from '@/lib/utils'
import type { AnswerFeedback } from '@/types'

export function QuizFeedback({ result, documentId }: { result: AnswerFeedback; documentId: string }) {
  return (
    <div
      role="status"
      className={cn('mt-4 space-y-3 rounded-lg border-l-4 bg-card p-4', result.is_correct ? 'border-l-success' : 'border-l-destructive')}
    >
      <p className={cn('flex items-center gap-2 font-semibold', result.is_correct ? 'text-success' : 'text-destructive')}>
        {result.is_correct ? <CircleCheck className="size-5" /> : <CircleX className="size-5" />}
        {result.is_correct ? 'Correct' : 'Not quite'}
      </p>
      {result.feedback && <p className="reading">{result.feedback}</p>}
      {result.reference_answer && (
        <p className="reading">
          <span className="font-sans text-sm font-semibold text-muted-foreground">Model answer: </span>
          {result.reference_answer}
        </p>
      )}
      {result.explanation && <p className="reading">{result.explanation}</p>}
      <QuizCitation documentId={documentId} page={result.citation_page} excerpt={result.citation_excerpt} />
    </div>
  )
}
