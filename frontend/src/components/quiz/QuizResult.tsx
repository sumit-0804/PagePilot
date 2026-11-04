import { Link } from 'react-router-dom'
import { motion } from 'motion/react'
import { Award, Check, X } from 'lucide-react'
import { PageChip } from '@/components/chat/CitationChip'
import type { QuizSession } from '@/types'

export function QuizResult({ quiz }: { quiz: QuizSession }) {
  const pct = Math.round((100 * quiz.score) / Math.max(quiz.total_questions, 1))
  const missed = (quiz.all_questions ?? []).filter((q) => q.is_answered && !q.is_correct)
  const radius = 34
  const circumference = 2 * Math.PI * radius
  const strokeDashoffset = circumference - (pct / 100) * circumference

  return (
    <motion.div initial={{ opacity: 0, scale: 0.98 }} animate={{ opacity: 1, scale: 1 }}>
      {/* Score ring & summary header */}
      <div className="flex flex-wrap items-center gap-5 rounded-2xl border border-border/70 bg-secondary/40 p-4 sm:p-5">
        <div className="relative flex size-22 shrink-0 items-center justify-center">
          <svg className="size-22 -rotate-90" viewBox="0 0 84 84" aria-hidden>
            <circle cx="42" cy="42" r={radius} stroke="var(--border)" strokeWidth="7" fill="transparent" />
            <motion.circle
              cx="42"
              cy="42"
              r={radius}
              stroke="var(--primary)"
              strokeWidth="7"
              strokeLinecap="round"
              fill="transparent"
              strokeDasharray={circumference}
              initial={{ strokeDashoffset: circumference }}
              animate={{ strokeDashoffset }}
              transition={{ duration: 1, ease: 'easeOut' }}
            />
          </svg>
          <span className="absolute font-reading text-lg font-bold tabular-nums">{pct}%</span>
        </div>

        <div className="min-w-0 flex-1">
          <div className="inline-flex items-center gap-1.5 rounded-full bg-accent px-2.5 py-0.5 text-xs font-semibold text-accent-foreground">
            <Award className="size-3.5" />
            Quiz complete
          </div>
          <p className="mt-1.5 font-reading text-2xl font-semibold">
            You got {quiz.score} of {quiz.total_questions} right
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {pct >= 80
              ? 'Strong result.'
              : pct >= 50
                ? 'A good base. Review the pages you missed below.'
                : 'Worth another pass: start with the pages below.'}
          </p>
        </div>
      </div>

      {/* Question-by-question breakdown */}
      <ol className="mt-4 space-y-2.5">
        {(quiz.all_questions ?? []).map((q, i) => (
          <motion.li
            key={q.id}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.04 }}
            className="flex gap-3 rounded-xl border border-border/80 bg-card px-4 py-3"
          >
            <span
              className={
                q.is_correct
                  ? 'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-success/15 text-success'
                  : 'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-destructive/15 text-destructive'
              }
              aria-label={q.is_correct ? 'Correct' : 'Incorrect'}
            >
              {q.is_correct ? <Check className="size-3.5" /> : <X className="size-3.5" />}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                Question {i + 1}
              </span>
              <span className="mt-0.5 block font-reading">{q.question_text}</span>
              {!q.is_correct && (
                <span className="mt-1.5 block rounded-lg bg-muted/60 px-2.5 py-1.5 text-sm">
                  <span className="font-semibold text-muted-foreground">Answer: </span>
                  {q.question_type === 'short_answer' ? q.reference_answer : q.options[q.correct_option ?? -1]}
                </span>
              )}
            </span>
            {q.citation_page && quiz.document_id && (
              <PageChip documentId={quiz.document_id} page={q.citation_page} className="h-fit self-center" />
            )}
          </motion.li>
        ))}
      </ol>

      {missed.length > 0 && (
        <Link
          to="/history"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold text-primary underline-offset-4 hover:underline"
        >
          See all your weak spots in Study progress →
        </Link>
      )}
    </motion.div>
  )
}
