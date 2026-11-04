import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { Check, ChevronLeft, ChevronRight, Layers, RotateCcw, Sparkles } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys } from '@/hooks/queries'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tilt3DCard } from '@/components/ui/aceternity/card-3d'
import { PageChip } from '@/components/chat/CitationChip'
import { cn } from '@/lib/utils'
import type { CardStatus, FlashcardSet } from '@/types'

/** A flashcard set inside the chat: 3D stacked deck with tilt & flip physics, mastery bar, and status controls. */
export function FlashcardDeck({ setId }: { setId: string }) {
  const queryClient = useQueryClient()
  const set = useQuery({ queryKey: queryKeys.flashcards(setId), queryFn: () => api.flashcards.get(setId) })
  const [index, setIndex] = useState(0)
  const [flipped, setFlipped] = useState(false)

  const mark = useMutation({
    mutationFn: ({ cardId, status }: { cardId: string; status: CardStatus }) => api.flashcards.setCardStatus(cardId, status),
    onMutate: ({ cardId, status }) => {
      queryClient.setQueryData<FlashcardSet>(queryKeys.flashcards(setId), (old) =>
        old && { ...old, cards: old.cards.map((c) => (c.id === cardId ? { ...c, status } : c)) },
      )
    },
    onError: () => void queryClient.invalidateQueries({ queryKey: queryKeys.flashcards(setId) }),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.progressAll }),
  })

  if (set.isLoading) {
    return <Skeleton className="h-64 w-full rounded-2xl" aria-label="Loading flashcards" />
  }
  if (set.error || !set.data) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {set.error?.message ?? 'Flashcards not found.'}
      </p>
    )
  }

  const cards = set.data.cards
  const safeIndex = Math.min(index, Math.max(cards.length - 1, 0))
  const card = cards[safeIndex]
  if (!card) return <p className="text-sm text-muted-foreground">This set has no cards.</p>
  const known = cards.filter((c) => c.status === 'known').length
  const practice = cards.filter((c) => c.status === 'practice').length

  const go = (delta: number) => {
    setFlipped(false)
    setIndex((i) => (i + delta + cards.length) % cards.length)
  }
  const setStatus = (status: CardStatus) => {
    mark.mutate({ cardId: card.id, status })
    if (safeIndex < cards.length - 1) go(1)
  }

  return (
    <div className="rounded-2xl border border-border/80 bg-card/85 p-4 shadow-sm backdrop-blur-sm sm:p-6">
      {/* Header & Mastery summary */}
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <div className="flex items-center gap-2">
          <span className="inline-flex size-7 items-center justify-center rounded-lg bg-accent text-accent-foreground">
            <Layers className="size-4" />
          </span>
          <span className="font-semibold">
            Card {safeIndex + 1} of {cards.length}
          </span>
        </div>
        <div className="flex items-center gap-3 text-xs font-medium">
          <span className="inline-flex items-center gap-1.5 text-success">
            <span className="size-2 rounded-full bg-success" />
            {known} known
          </span>
          <span className="inline-flex items-center gap-1.5 text-terracotta">
            <span className="size-2 rounded-full bg-terracotta" />
            {practice} to practise
          </span>
        </div>
      </div>

      {/* Segmented card mastery dots */}
      <div className="mt-3 flex gap-1" aria-hidden>
        {cards.map((c, idx) => (
          <button
            key={c.id}
            type="button"
            tabIndex={-1}
            onClick={() => {
              setFlipped(false)
              setIndex(idx)
            }}
            className={cn(
              'h-1.5 flex-1 rounded-full transition-all',
              idx === safeIndex && 'ring-2 ring-primary/40 ring-offset-1 ring-offset-background',
              c.status === 'known'
                ? 'bg-success'
                : c.status === 'practice'
                  ? 'bg-terracotta'
                  : idx === safeIndex
                    ? 'bg-primary'
                    : 'bg-border',
            )}
          />
        ))}
      </div>

      {/* 3D Stacked Deck & Flip Card */}
      <div className="relative mt-5 mb-2">
        {/* Background deck layers */}
        {cards.length > 1 && (
          <>
            <div
              className="pointer-events-none absolute inset-x-5 -bottom-2.5 h-10 rounded-2xl border border-border/60 bg-secondary/50"
              aria-hidden
            />
            <div
              className="pointer-events-none absolute inset-x-2.5 -bottom-1.5 h-10 rounded-2xl border border-border/80 bg-secondary/80"
              aria-hidden
            />
          </>
        )}

        <Tilt3DCard intensity={4} className="card-3d">
          <button
            type="button"
            onClick={() => setFlipped((f) => !f)}
            aria-label={flipped ? 'Show the question' : 'Show the answer'}
            data-flipped={flipped}
            className="card-3d-inner relative block h-60 w-full text-left"
          >
            {/* Front face: Question */}
            <span className="card-3d-face absolute inset-0 flex flex-col justify-between overflow-hidden rounded-2xl border border-border bg-gradient-to-br from-card via-card to-secondary/40 p-6 shadow-md">
              <span className="flex items-center justify-between">
                <span className="inline-flex items-center gap-1.5 rounded-full bg-accent px-2.5 py-0.5 text-xs font-semibold text-accent-foreground">
                  Question
                </span>
                {card.status !== 'new' && (
                  <span
                    className={cn(
                      'rounded-full px-2 py-0.5 text-[11px] font-semibold capitalize',
                      card.status === 'known' ? 'bg-success/15 text-success' : 'bg-terracotta/15 text-terracotta',
                    )}
                  >
                    {card.status === 'known' ? 'Known' : 'Practice'}
                  </span>
                )}
              </span>
              <span className="my-auto font-reading text-xl leading-snug font-medium text-foreground">
                {card.front}
              </span>
              <span className="flex items-center justify-between text-xs text-muted-foreground">
                <span>Click or press Enter to flip</span>
                <RotateCcw className="size-3.5 opacity-70" />
              </span>
            </span>

            {/* Back face: Answer */}
            <span className="card-3d-face card-3d-back absolute inset-0 flex flex-col justify-between overflow-auto rounded-2xl border-2 border-primary/45 bg-gradient-to-br from-accent/40 via-card to-card p-6 shadow-md">
              <span className="inline-flex w-fit items-center gap-1.5 rounded-full bg-primary/15 px-2.5 py-0.5 text-xs font-semibold text-primary">
                <Sparkles className="size-3" />
                Answer
              </span>
              <span className="my-auto font-reading text-lg leading-relaxed text-foreground">
                {card.back}
              </span>
              <span className="text-xs text-muted-foreground">Rate how well you knew this below</span>
            </span>
          </button>
        </Tilt3DCard>
      </div>

      {/* Controls */}
      <div className="mt-5 flex flex-wrap items-center gap-2">
        <Button variant="outline" size="icon-sm" className="rounded-xl" aria-label="Previous card" onClick={() => go(-1)}>
          <ChevronLeft />
        </Button>
        <Button variant="outline" size="icon-sm" className="rounded-xl" aria-label="Next card" onClick={() => go(1)}>
          <ChevronRight />
        </Button>
        {card.citation_page && set.data.document_id && (
          <PageChip documentId={set.data.document_id} page={card.citation_page} />
        )}
        <span className="flex-1" />
        {flipped ? (
          <motion.div initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className={cn(
                'rounded-xl',
                card.status === 'practice' && 'border-terracotta bg-terracotta/10 text-terracotta',
              )}
              onClick={() => setStatus('practice')}
            >
              Need practice
            </Button>
            <Button
              variant="outline"
              size="sm"
              className={cn(
                'rounded-xl',
                card.status === 'known' && 'border-success bg-success/10 text-success',
              )}
              onClick={() => setStatus('known')}
            >
              <Check className="size-3.5" />
              Known
            </Button>
          </motion.div>
        ) : (
          <Button variant="outline" size="sm" className="rounded-xl" onClick={() => setFlipped(true)}>
            <RotateCcw />
            Show answer
          </Button>
        )}
      </div>
    </div>
  )
}
