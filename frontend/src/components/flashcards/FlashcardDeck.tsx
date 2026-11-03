import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RotateCcw } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys } from '@/hooks/queries'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { CitationChip } from '@/components/chat/CitationChip'
import { cn } from '@/lib/utils'
import type { CardStatus, FlashcardSet } from '@/types'

/** A flashcard set inside the chat: flip to reveal, step through, and mark what you know. */
export function FlashcardDeck({ setId, documentId }: { setId: string; documentId: string }) {
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
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.progress(documentId) }),
  })

  if (set.isLoading) {
    return <Skeleton className="h-56 w-full rounded-xl" aria-label="Loading flashcards" />
  }
  if (set.error || !set.data) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {set.error?.message ?? 'Flashcards not found.'}
      </p>
    )
  }

  const cards = set.data.cards
  const card = cards[Math.min(index, cards.length - 1)]
  if (!card) return <p className="text-sm text-muted-foreground">This set has no cards.</p>
  const known = cards.filter((c) => c.status === 'known').length
  const practice = cards.filter((c) => c.status === 'practice').length

  const go = (delta: number) => {
    setFlipped(false)
    setIndex((i) => (i + delta + cards.length) % cards.length)
  }
  const setStatus = (status: CardStatus) => {
    mark.mutate({ cardId: card.id, status })
    if (index < cards.length - 1) go(1)
  }

  return (
    <div className="rounded-xl border bg-background p-4 sm:p-5">
      <div className="flex items-center justify-between text-sm">
        <span className="font-semibold">
          Card {index + 1} of {cards.length}
        </span>
        <span className="text-muted-foreground">
          {known} known, {practice} to practise
        </span>
      </div>

      <div className="card-3d mt-3">
        <button
          type="button"
          onClick={() => setFlipped((f) => !f)}
          aria-label={flipped ? 'Show the question' : 'Show the answer'}
          data-flipped={flipped}
          className="card-3d-inner relative block h-56 w-full text-left"
        >
          <span className="card-3d-face absolute inset-0 flex flex-col rounded-xl border bg-card p-5">
            <span className="text-sm text-muted-foreground">Question</span>
            <span className="my-auto font-reading text-xl leading-snug font-medium">{card.front}</span>
            <span className="text-xs text-muted-foreground">Click or press Enter to flip</span>
          </span>
          <span className="card-3d-face card-3d-back absolute inset-0 flex flex-col overflow-auto rounded-xl border border-primary/40 bg-card p-5">
            <span className="text-sm text-muted-foreground">Answer</span>
            <span className="my-auto font-reading text-lg leading-relaxed">{card.back}</span>
          </span>
        </button>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="icon-sm" aria-label="Previous card" onClick={() => go(-1)}>
          <ChevronLeft />
        </Button>
        <Button variant="ghost" size="icon-sm" aria-label="Next card" onClick={() => go(1)}>
          <ChevronRight />
        </Button>
        {card.citation_page && <CitationChip documentId={documentId} page={card.citation_page} />}
        <span className="flex-1" />
        {flipped ? (
          <>
            <Button variant="outline" size="sm" className={cn(card.status === 'practice' && 'border-destructive text-destructive')} onClick={() => setStatus('practice')}>
              Need practice
            </Button>
            <Button variant="outline" size="sm" className={cn(card.status === 'known' && 'border-success text-success')} onClick={() => setStatus('known')}>
              Known
            </Button>
          </>
        ) : (
          <Button variant="outline" size="sm" onClick={() => setFlipped(true)}>
            <RotateCcw />
            Show answer
          </Button>
        )}
      </div>
    </div>
  )
}
