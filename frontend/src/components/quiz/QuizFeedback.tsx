import { motion } from 'motion/react'
import { CircleCheck, CircleX } from 'lucide-react'
import { QuizCitation } from '@/components/quiz/QuizCitation'
import { cn } from '@/lib/utils'
import type { AnswerFeedback } from '@/types'

export function QuizFeedback({ result, documentId }: { result: AnswerFeedback; documentId?: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', stiffness: 340, damping: 28 }}
      role="status"
      className={cn(
        'mt-4 space-y-3 rounded-xl border border-l-4 bg-card/95 p-4 shadow-xs',
        result.is_correct ? 'border-success/30 border-l-success' : 'border-destructive/30 border-l-destructive',
      )}
    >
      <p className={cn('flex items-center gap-2 font-semibold', result.is_correct ? 'text-success' : 'text-destructive')}>
        {result.is_correct ? <CircleCheck className="size-5" /> : <CircleX className="size-5" />}
        {result.is_correct ? 'Correct' : 'Not quite'}
      </p>
      {result.feedback && <p className="reading">{result.feedback}</p>}
      {result.reference_answer && (
        <p className="reading rounded-lg bg-muted/60 px-3 py-2 text-sm">
          <span className="font-sans text-xs font-semibold tracking-wide text-muted-foreground uppercase">Model answer: </span>
          {result.reference_answer}
        </p>
      )}
      {result.explanation && <p className="reading">{result.explanation}</p>}
      <QuizCitation documentId={documentId} page={result.citation_page} excerpt={result.citation_excerpt} />
    </motion.div>
  )
}
