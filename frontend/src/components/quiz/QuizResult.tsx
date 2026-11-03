import { Link } from 'react-router-dom'
import { Check, X } from 'lucide-react'
import { CitationChip } from '@/components/chat/CitationChip'
import type { QuizSession } from '@/types'

export function QuizResult({ quiz }: { quiz: QuizSession }) {
  const pct = Math.round((100 * quiz.score) / Math.max(quiz.total_questions, 1))
  const missed = (quiz.all_questions ?? []).filter((q) => q.is_answered && !q.is_correct)

  return (
    <div>
      <p className="font-reading text-2xl font-semibold">
        You got {quiz.score} of {quiz.total_questions} right
      </p>
      <p className="mt-1 text-muted-foreground">
        {pct >= 80 ? 'Strong result.' : pct >= 50 ? 'A good base. Review the pages you missed below.' : 'Worth another pass: start with the pages below.'}
      </p>

      <ol className="mt-4 space-y-2">
        {(quiz.all_questions ?? []).map((q, i) => (
          <li key={q.id} className="flex gap-3 rounded-lg border bg-card px-3 py-2.5">
            <span className={q.is_correct ? 'text-success' : 'text-destructive'} aria-label={q.is_correct ? 'Correct' : 'Incorrect'}>
              {q.is_correct ? <Check className="size-4 translate-y-1" /> : <X className="size-4 translate-y-1" />}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-sm text-muted-foreground">Question {i + 1}</span>
              <span className="block font-reading">{q.question_text}</span>
              {!q.is_correct && (
                <span className="mt-1 block text-sm">
                  Answer: {q.question_type === 'short_answer' ? q.reference_answer : q.options[q.correct_option ?? -1]}
                </span>
              )}
            </span>
            {q.citation_page && <CitationChip documentId={quiz.document_id} page={q.citation_page} className="h-fit self-center" />}
          </li>
        ))}
      </ol>

      {missed.length > 0 && (
        <Link to="/history" className="mt-4 inline-block text-sm font-semibold text-primary underline-offset-4 hover:underline">
          See all your weak spots in Study progress
        </Link>
      )}
    </div>
  )
}
