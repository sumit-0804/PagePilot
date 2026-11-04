import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'motion/react'
import { Loader2 } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys } from '@/hooks/queries'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { QuizProgress } from '@/components/quiz/QuizProgress'
import { QuizQuestionCard } from '@/components/quiz/QuizQuestionCard'
import { QuizFeedback } from '@/components/quiz/QuizFeedback'
import { QuizResult } from '@/components/quiz/QuizResult'
import type { AnswerFeedback, QuizQuestion } from '@/types'

/** An interactive quiz inside the chat: one question at a time, graded by the server. */
export function QuizBlock({ sessionId }: { sessionId: string }) {
  const queryClient = useQueryClient()
  const quiz = useQuery({ queryKey: queryKeys.quiz(sessionId), queryFn: () => api.quizzes.get(sessionId) })
  const [graded, setGraded] = useState<{ question: QuizQuestion; result: AnswerFeedback } | null>(null)
  const [advancing, setAdvancing] = useState(false)

  const answer = useMutation({
    mutationFn: (vars: { question: QuizQuestion; body: { selected_option?: number; answer_text?: string } }) =>
      api.quizzes.answer(sessionId, { question_id: vars.question.id, ...vars.body }),
    onSuccess: (result, vars) => {
      setGraded({ question: vars.question, result })
      void queryClient.invalidateQueries({ queryKey: queryKeys.progressAll })
    },
  })

  async function next() {
    setAdvancing(true)
    await quiz.refetch()
    setGraded(null)
    setAdvancing(false)
    answer.reset()
  }

  return (
    <div className="rounded-2xl border border-border/80 bg-card/85 p-4 shadow-sm backdrop-blur-sm sm:p-6">
      {quiz.isLoading && (
        <div className="space-y-3" role="status" aria-label="Loading quiz">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-11 w-full rounded-xl" />
          <Skeleton className="h-11 w-full rounded-xl" />
        </div>
      )}

      {quiz.error && (
        <div role="alert" className="text-sm">
          <p className="text-destructive">{quiz.error.message}</p>
          <Button variant="outline" size="sm" className="mt-2 rounded-xl" onClick={() => quiz.refetch()}>
            Try again
          </Button>
        </div>
      )}

      {quiz.data &&
        (graded ? (
          <motion.div key="graded" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
            <QuizProgress
              number={graded.result.current_index}
              answered={graded.result.current_index}
              total={graded.result.total_questions}
              score={graded.result.current_score}
            />
            <div className="mt-5">
              <QuizQuestionCard question={graded.question} result={graded.result} />
              <QuizFeedback result={graded.result} documentId={quiz.data.document_id ?? undefined} />
            </div>
            <Button className="mt-4 rounded-xl" onClick={next} disabled={advancing}>
              {advancing && <Loader2 className="animate-spin" />}
              {graded.result.completed ? 'See results' : 'Next question'}
            </Button>
          </motion.div>
        ) : quiz.data.completed ? (
          <QuizResult quiz={quiz.data} />
        ) : quiz.data.current_question ? (
          <AnimatePresence mode="wait">
            <motion.div
              key={quiz.data.current_question.id}
              initial={{ opacity: 0, x: 12 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -12 }}
              transition={{ duration: 0.22 }}
            >
              <QuizProgress
                number={quiz.data.current_index + 1}
                answered={quiz.data.current_index}
                total={quiz.data.total_questions}
                score={quiz.data.score}
              />
              <div className="mt-5">
                <QuizQuestionCard
                  key={quiz.data.current_question.id}
                  question={quiz.data.current_question}
                  pending={answer.isPending}
                  error={answer.error?.message}
                  onSubmit={(body) => answer.mutate({ question: quiz.data.current_question!, body })}
                />
              </div>
            </motion.div>
          </AnimatePresence>
        ) : null)}
    </div>
  )
}
