import { motion } from 'motion/react'
import { Sparkles } from 'lucide-react'
import { cn } from '@/lib/utils'

/** Position in the quiz and the running score; one segment per question. */
export function QuizProgress({
  number,
  answered,
  total,
  score,
}: {
  number: number
  answered: number
  total: number
  score: number
}) {
  return (
    <div>
      <div className="flex items-center justify-between text-sm">
        <span className="font-semibold">
          Question {number} of {total}
        </span>
        <span className="inline-flex items-center gap-1.5 rounded-full bg-accent px-2.5 py-0.5 text-xs font-semibold text-accent-foreground">
          <Sparkles className="size-3" />
          {score} correct so far
        </span>
      </div>
      <div className="mt-2.5 flex gap-1.5" aria-hidden>
        {Array.from({ length: total }, (_, i) => {
          const done = i < answered
          const current = i === number - 1
          return (
            <span key={i} className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-border">
              {done && (
                <motion.span
                  initial={{ width: 0 }}
                  animate={{ width: '100%' }}
                  className="block h-full rounded-full bg-primary"
                />
              )}
              {!done && current && (
                <span className={cn('block h-full w-full rounded-full bg-primary/35')} />
              )}
            </span>
          )
        })}
      </div>
    </div>
  )
}
