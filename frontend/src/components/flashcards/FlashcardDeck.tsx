import React, { useState } from 'react'
import {
  ChevronLeft,
  ChevronRight,
  RotateCcw,
  Check,
  Bookmark,
  BookOpen,
  Sparkles,
} from 'lucide-react'
import type { FlashcardSet } from '../../types'
import { api } from '../../services/api'

interface FlashcardDeckProps {
  flashcardSet: FlashcardSet
  onSelectPage: (page: number) => void
}

export const FlashcardDeck: React.FC<FlashcardDeckProps> = ({
  flashcardSet,
  onSelectPage,
}) => {
  const [currentIndex, setCurrentIndex] = useState<number>(0)
  const [isFlipped, setIsFlipped] = useState<boolean>(false)
  const [cards, setCards] = useState(flashcardSet.cards || [])

  const currentCard = cards[currentIndex]

  const handleNext = () => {
    setIsFlipped(false)
    setCurrentIndex((prev) => (prev + 1 < cards.length ? prev + 1 : 0))
  }

  const handlePrev = () => {
    setIsFlipped(false)
    setCurrentIndex((prev) => (prev > 0 ? prev - 1 : cards.length - 1))
  }

  const handleMarkStatus = async (status: 'known' | 'practice') => {
    if (!currentCard) return
    try {
      await api.updateFlashcardStatus(currentCard.id, status)
      setCards((prev) =>
        prev.map((c, i) => (i === currentIndex ? { ...c, status } : c))
      )
      handleNext()
    } catch (e) {
      console.error('Failed to update status', e)
    }
  }

  if (!currentCard || cards.length === 0) {
    return null
  }

  return (
    <div className="bg-slate-900/90 border border-indigo-500/30 rounded-2xl p-5 my-4 shadow-xl backdrop-blur-md max-w-xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between pb-3 mb-4 border-b border-slate-800 text-xs">
        <div className="flex items-center space-x-2 text-indigo-400 font-semibold uppercase tracking-wider">
          <Sparkles className="w-3.5 h-3.5" />
          <span>Flashcard Revision</span>
        </div>
        <div className="font-mono text-slate-400 bg-slate-800 px-2.5 py-0.5 rounded-full">
          Card {currentIndex + 1} of {cards.length}
        </div>
      </div>

      {/* 3D Flip Card */}
      <div
        onClick={() => setIsFlipped(!isFlipped)}
        className="w-full h-64 cursor-pointer perspective-1000 mb-4 select-none"
      >
        <div
          className={`relative w-full h-full duration-500 transform-style-3d transition-transform ${
            isFlipped ? 'rotate-y-180' : ''
          }`}
        >
          {/* Front (Question / Concept) */}
          <div className="absolute inset-0 backface-hidden bg-gradient-to-br from-slate-800 to-slate-850 border border-slate-700/80 rounded-2xl p-6 flex flex-col justify-between shadow-lg">
            <div className="flex items-center justify-between text-xs text-indigo-400 font-medium">
              <span>CONCEPT / QUESTION</span>
              <span className="text-[11px] text-slate-400">Click to flip</span>
            </div>
            <div className="text-base font-semibold text-white text-center leading-relaxed">
              {currentCard.front}
            </div>
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span className="capitalize text-indigo-300">{flashcardSet.topic || 'Document'}</span>
              {currentCard.citation_page > 0 && (
                <span className="text-slate-400">Page {currentCard.citation_page}</span>
              )}
            </div>
          </div>

          {/* Back (Answer / Explanation) */}
          <div className="absolute inset-0 backface-hidden rotate-y-180 bg-gradient-to-br from-indigo-950/80 to-slate-900 border border-indigo-500/50 rounded-2xl p-6 flex flex-col justify-between shadow-lg">
            <div className="flex items-center justify-between text-xs text-emerald-400 font-medium">
              <span>EXPLANATION / ANSWER</span>
              <span className="text-[11px] text-slate-400">Click to flip</span>
            </div>
            <div className="text-sm text-slate-200 overflow-y-auto max-h-40 text-center leading-relaxed font-normal">
              {currentCard.back}
            </div>
            <div className="flex items-center justify-between text-xs pt-2 border-t border-slate-800">
              {currentCard.citation_page > 0 ? (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    onSelectPage(currentCard.citation_page)
                  }}
                  className="inline-flex items-center space-x-1 text-indigo-400 hover:text-indigo-300 font-medium"
                >
                  <BookOpen className="w-3.5 h-3.5" />
                  <span>View Page {currentCard.citation_page}</span>
                </button>
              ) : (
                <span />
              )}
              <span className="text-slate-400">Card {currentIndex + 1}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Mastery Controls */}
      <div className="flex items-center justify-between pt-2">
        <button
          onClick={handlePrev}
          className="p-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 transition"
          title="Previous card"
        >
          <ChevronLeft className="w-4 h-4" />
        </button>

        <div className="flex items-center space-x-2">
          <button
            onClick={() => handleMarkStatus('practice')}
            className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold border transition ${
              currentCard.status === 'practice'
                ? 'bg-amber-500/20 text-amber-300 border-amber-500/50'
                : 'bg-slate-800 text-slate-300 border-slate-700 hover:bg-amber-500/10 hover:text-amber-300'
            }`}
          >
            <Bookmark className="w-3.5 h-3.5" />
            <span>Need Practice</span>
          </button>

          <button
            onClick={() => handleMarkStatus('known')}
            className={`flex items-center space-x-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold border transition ${
              currentCard.status === 'known'
                ? 'bg-emerald-500/20 text-emerald-300 border-emerald-500/50'
                : 'bg-slate-800 text-slate-300 border-slate-700 hover:bg-emerald-500/10 hover:text-emerald-300'
            }`}
          >
            <Check className="w-3.5 h-3.5" />
            <span>Known</span>
          </button>
        </div>

        <button
          onClick={handleNext}
          className="p-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 transition"
          title="Next card"
        >
          <ChevronRight className="w-4 h-4" />
        </button>
      </div>
    </div>
  )
}
