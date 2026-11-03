import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
export function QuizBlock({ sessionId, documentId }: { sessionId: string; documentId: string }) {
  const queryClient = useQueryClient()
  const quiz = useQuery({ queryKey: queryKeys.quiz(sessionId), queryFn: () => api.quizzes.get(sessionId) })
  const [graded, setGraded] = useState<{ question: QuizQuestion; result: AnswerFeedback } | null>(null)
  const [advancing, setAdvancing] = useState(false)

  const answer = useMutation({
    mutationFn: (vars: { question: QuizQuestion; body: { selected_option?: number; answer_text?: string } }) =>
      api.quizzes.answer(sessionId, { question_id: vars.question.id, ...vars.body }),
    onSuccess: (result, vars) => {
      setGraded({ question: vars.question, result })
      void queryClient.invalidateQueries({ queryKey: queryKeys.progress(documentId) })
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
    <div className="rounded-xl border bg-background p-4 sm:p-5">
      {quiz.isLoading && (
        <div className="space-y-3" role="status" aria-label="Loading quiz">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-11 w-full" />
          <Skeleton className="h-11 w-full" />
        </div>
      )}

      {quiz.error && (
        <div role="alert" className="text-sm">
          <p className="text-destructive">{quiz.error.message}</p>
          <Button variant="outline" size="sm" className="mt-2" onClick={() => quiz.refetch()}>
            Try again
          </Button>
        </div>
      )}

      {quiz.data &&
        (graded ? (
          <>
            <QuizProgress
              number={graded.result.current_index}
              answered={graded.result.current_index}
              total={graded.result.total_questions}
              score={graded.result.current_score}
            />
            <div className="mt-4">
              <QuizQuestionCard question={graded.question} result={graded.result} />
              <QuizFeedback result={graded.result} documentId={documentId} />
            </div>
            <Button className="mt-4" onClick={next} disabled={advancing}>
              {advancing && <Loader2 className="animate-spin" />}
              {graded.result.completed ? 'See results' : 'Next question'}
            </Button>
          </>
        ) : quiz.data.completed ? (
          <QuizResult quiz={quiz.data} />
        ) : quiz.data.current_question ? (
          <>
            <QuizProgress
              number={quiz.data.current_index + 1}
              answered={quiz.data.current_index}
              total={quiz.data.total_questions}
              score={quiz.data.score}
            />
            <div className="mt-4">
              <QuizQuestionCard
                key={quiz.data.current_question.id}
                question={quiz.data.current_question}
                pending={answer.isPending}
                error={answer.error?.message}
                onSubmit={(body) => answer.mutate({ question: quiz.data.current_question!, body })}
              />
            </div>
          </>
        ) : null)}
    </div>
  )
}
