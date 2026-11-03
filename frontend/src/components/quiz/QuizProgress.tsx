import { cn } from '@/lib/utils'

/** Position in the quiz and the running score; one segment per question. */
export function QuizProgress({ number, answered, total, score }: { number: number; answered: number; total: number; score: number }) {
  return (
    <div>
      <div className="flex items-center justify-between text-sm">
        <span className="font-semibold">
          Question {number} of {total}
        </span>
        <span className="text-muted-foreground">{score} correct so far</span>
      </div>
      <div className="mt-2 flex gap-1" aria-hidden>
        {Array.from({ length: total }, (_, i) => (
          <span key={i} className={cn('h-1 flex-1 rounded-full', i < answered ? 'bg-primary' : 'bg-border')} />
        ))}
      </div>
    </div>
  )
}
