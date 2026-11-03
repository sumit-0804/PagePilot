import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { QuizOption, type OptionState } from '@/components/quiz/QuizOption'
import type { AnswerFeedback, QuizQuestion } from '@/types'

interface Props {
  question: QuizQuestion
  /** Present once the question has been graded. */
  result?: AnswerFeedback
  onSubmit?: (answer: { selected_option?: number; answer_text?: string }) => void
  pending?: boolean
  error?: string | null
}

export function QuizQuestionCard({ question, result, onSubmit, pending = false, error }: Props) {
  const [selected, setSelected] = useState<number | null>(null)
  const [text, setText] = useState('')
  const answered = !!result
  const isShort = question.question_type === 'short_answer'

  const optionState = (i: number): OptionState => {
    if (!result) return selected === i ? 'selected' : 'idle'
    if (i === result.correct_option) return 'correct'
    if (i === result.selected_option) return 'incorrect'
    return 'dim'
  }

  const canSubmit = !answered && !pending && (isShort ? text.trim().length > 0 : selected !== null)

  return (
    <div>
      <p className="font-reading text-lg leading-snug font-medium">{question.question_text}</p>

      {isShort ? (
        <div className="mt-4">
          <label htmlFor={`answer-${question.id}`} className="sr-only">
            Your answer
          </label>
          <Textarea
            id={`answer-${question.id}`}
            value={answered ? (result.answer_text ?? text) : text}
            onChange={(e) => setText(e.target.value)}
            readOnly={answered}
            maxLength={2000}
            placeholder="Write your answer in a sentence or two"
            className="min-h-24 bg-card text-base"
          />
        </div>
      ) : (
        <div role="radiogroup" aria-label="Answer options" className="mt-4 space-y-2">
          {question.options.map((opt, i) => (
            <QuizOption key={i} index={i} text={opt} state={optionState(i)} disabled={answered || pending} onSelect={() => setSelected(i)} />
          ))}
        </div>
      )}

      {!answered && (
        <div className="mt-4 flex items-center gap-3">
          <Button
            disabled={!canSubmit}
            onClick={() => onSubmit?.(isShort ? { answer_text: text.trim() } : { selected_option: selected ?? undefined })}
          >
            {pending && <Loader2 className="animate-spin" />}
            {pending && isShort ? 'Checking…' : 'Check answer'}
          </Button>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
