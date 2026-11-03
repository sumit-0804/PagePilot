import React, { useState } from 'react'
import {
  CheckCircle2,
  XCircle,
  HelpCircle,
  ArrowRight,
  RotateCcw,
  BookOpen,
  Award,
  Sparkles,
} from 'lucide-react'
import type { AnswerFeedback, QuizSession } from '../../types'
import { api } from '../../services/api'

interface QuizCardProps {
  session: QuizSession
  onSelectPage: (page: number) => void
  onComplete?: (finalScore: number, total: number) => void
}

export const QuizCard: React.FC<QuizCardProps> = ({
  session: initialSession,
  onSelectPage,
  onComplete,
}) => {
  const [session, setSession] = useState<QuizSession>(initialSession)
  const [selectedOption, setSelectedOption] = useState<number | null>(null)
  const [feedback, setFeedback] = useState<AnswerFeedback | null>(null)
  const [submitting, setSubmitting] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)

  const currentQ = feedback?.nextQuestion || session.current_question || session.all_questions?.[session.current_index]

  const handleSubmit = async () => {
    if (selectedOption === null || !currentQ) return
    setSubmitting(true)
    setError(null)

    try {
      const res = await api.answerQuiz(session.session_id, currentQ.id, selectedOption)
      setFeedback(res)
      setSession((prev) => ({
        ...prev,
        score: res.current_score,
        current_index: res.current_index,
        completed: res.completed,
      }))
      if (res.completed && onComplete) {
        onComplete(res.current_score, res.total_questions)
      }
    } catch (err: any) {
      setError(err.message || 'Failed to submit answer')
    } finally {
      setSubmitting(false)
    }
  }

  const handleNext = () => {
    setSelectedOption(null)
    setFeedback(null)
  }

  // Quiz Completed View
  if (session.completed && !feedback) {
    const percentage = Math.round((session.score / (session.total_questions || 1)) * 100)
    return (
      <div className="bg-gradient-to-br from-indigo-900/40 to-slate-900 border border-indigo-500/30 rounded-2xl p-6 text-center my-4 shadow-xl backdrop-blur-md">
        <div className="w-16 h-16 mx-auto mb-4 bg-indigo-500/20 text-indigo-400 rounded-full flex items-center justify-center border border-indigo-500/40">
          <Award className="w-8 h-8" />
        </div>
        <h3 className="text-xl font-bold text-white mb-1">Quiz Completed!</h3>
        <p className="text-sm text-slate-300 mb-4">
          Topic: <span className="text-indigo-300 font-medium">{session.topic || 'Document Concepts'}</span>
        </p>

        <div className="inline-flex items-center space-x-3 bg-slate-800/80 px-6 py-3 rounded-xl border border-slate-700 mb-6">
          <div className="text-3xl font-extrabold text-indigo-400">{session.score} / {session.total_questions}</div>
          <div className="text-left border-l border-slate-700 pl-3">
            <div className="text-xs text-slate-400">Accuracy</div>
            <div className="text-base font-semibold text-emerald-400">{percentage}%</div>
          </div>
        </div>

        <p className="text-xs text-slate-400 max-w-sm mx-auto mb-6">
          {percentage >= 80
            ? 'Outstanding mastery of this topic! You have demonstrated strong comprehension.'
            : 'Good effort! Review the citations and explanations below to reinforce weak areas.'}
        </p>
      </div>
    )
  }

  if (!currentQ) {
    return null
  }

  const optionLabels = ['A', 'B', 'C', 'D']

  return (
    <div className="bg-slate-900/95 border border-indigo-500/30 rounded-2xl p-5 my-4 shadow-xl backdrop-blur-md text-slate-200">
      {/* Header */}
      <div className="flex items-center justify-between pb-3 mb-4 border-b border-slate-800 text-xs">
        <div className="flex items-center space-x-2 text-indigo-400 font-semibold uppercase tracking-wider">
          <Sparkles className="w-3.5 h-3.5" />
          <span>Interactive Quiz</span>
        </div>
        <div className="font-mono text-slate-400 bg-slate-800 px-2.5 py-0.5 rounded-full">
          Question {(session.current_index || 0) + 1} of {session.total_questions}
        </div>
      </div>

      {/* Question Text */}
      <h4 className="text-base font-medium text-slate-100 mb-4 leading-relaxed">
        {currentQ.question_text}
      </h4>

      {/* Options List */}
      <div className="space-y-2.5 mb-5">
        {currentQ.options.map((opt, idx) => {
          const isSelected = selectedOption === idx
          const isSubmitted = feedback !== null
          const isCorrect = isSubmitted && feedback.correct_option === idx
          const isWrong = isSubmitted && feedback.selected_option === idx && !feedback.is_correct

          let borderClass = 'border-slate-800 hover:border-slate-700 bg-slate-850/60'
          if (isSelected && !isSubmitted) {
            borderClass = 'border-indigo-500 bg-indigo-500/15 text-indigo-200 shadow-md ring-1 ring-indigo-500'
          } else if (isCorrect) {
            borderClass = 'border-emerald-500 bg-emerald-500/15 text-emerald-200 ring-1 ring-emerald-500'
          } else if (isWrong) {
            borderClass = 'border-rose-500 bg-rose-500/15 text-rose-200 ring-1 ring-rose-500'
          }

          return (
            <button
              key={idx}
              onClick={() => !isSubmitted && setSelectedOption(idx)}
              disabled={isSubmitted}
              className={`w-full text-left p-3.5 rounded-xl border flex items-center space-x-3 transition-all ${borderClass}`}
            >
              <div
                className={`w-7 h-7 rounded-lg flex items-center justify-center font-bold text-xs shrink-0 ${
                  isSelected && !isSubmitted
                    ? 'bg-indigo-600 text-white'
                    : isCorrect
                    ? 'bg-emerald-600 text-white'
                    : isWrong
                    ? 'bg-rose-600 text-white'
                    : 'bg-slate-800 text-slate-400'
                }`}
              >
                {optionLabels[idx]}
              </div>
              <span className="text-sm flex-1 leading-snug">{opt}</span>
              {isCorrect && <CheckCircle2 className="w-5 h-5 text-emerald-400 shrink-0" />}
              {isWrong && <XCircle className="w-5 h-5 text-rose-400 shrink-0" />}
            </button>
          )
        })}
      </div>

      {/* Error Message */}
      {error && (
        <div className="p-2.5 mb-4 bg-red-950/40 border border-red-800 rounded-lg text-xs text-red-300">
          {error}
        </div>
      )}

      {/* Feedback Section (After Answer) */}
      {feedback && (
        <div className={`p-4 rounded-xl border mb-5 ${
          feedback.is_correct ? 'bg-emerald-950/30 border-emerald-500/40' : 'bg-rose-950/30 border-rose-500/40'
        }`}>
          <div className="flex items-center space-x-2 font-semibold text-sm mb-2">
            {feedback.is_correct ? (
              <span className="text-emerald-400 flex items-center space-x-1.5">
                <CheckCircle2 className="w-4 h-4" />
                <span>Correct!</span>
              </span>
            ) : (
              <span className="text-rose-400 flex items-center space-x-1.5">
                <XCircle className="w-4 h-4" />
                <span>Incorrect</span>
              </span>
            )}
          </div>
          <p className="text-xs text-slate-300 leading-relaxed mb-3">
            {feedback.explanation}
          </p>

          {/* Citation Button */}
          {feedback.citation_page > 0 && (
            <button
              onClick={() => onSelectPage(feedback.citation_page)}
              className="inline-flex items-center space-x-1.5 text-xs text-indigo-400 hover:text-indigo-300 bg-indigo-950/60 border border-indigo-800/60 px-2.5 py-1 rounded-md transition"
            >
              <BookOpen className="w-3.5 h-3.5" />
              <span>Source: Page {feedback.citation_page}</span>
            </button>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center justify-between pt-2">
        <div className="text-xs text-slate-400">
          Score: <span className="font-semibold text-white">{session.score}</span> / {session.total_questions}
        </div>

        {!feedback ? (
          <button
            onClick={handleSubmit}
            disabled={selectedOption === null || submitting}
            className="flex items-center space-x-2 px-5 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 disabled:hover:bg-indigo-600 text-white text-xs font-semibold rounded-xl transition shadow-lg shadow-indigo-600/20"
          >
            <span>{submitting ? 'Checking...' : 'Submit Answer'}</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        ) : (
          <button
            onClick={handleNext}
            className="flex items-center space-x-2 px-5 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold rounded-xl transition shadow-lg shadow-indigo-600/20"
          >
            <span>{feedback.completed ? 'View Results' : 'Continue to Next Question'}</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    </div>
  )
}
