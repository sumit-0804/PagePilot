import { useState } from 'react'
import { ThumbsDown, ThumbsUp } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { api } from '@/services/api'
import type { MessageRating, RatingReason } from '@/types'

const REASONS: { value: RatingReason; label: string }[] = [
  { value: 'incorrect', label: 'Incorrect' },
  { value: 'not_grounded', label: 'Not from my document' },
  { value: 'unhelpful', label: 'Not helpful' },
  { value: 'unclear', label: 'Unclear' },
  { value: 'other', label: 'Other' },
]

/** Thumbs up/down on an answer. Clicking the chosen thumb again removes the rating. */
export function RatingBar({ messageId, initial }: { messageId: string; initial?: MessageRating }) {
  const [rating, setRating] = useState<MessageRating | undefined>(initial)
  const [saving, setSaving] = useState(false)

  const save = async (next: MessageRating | undefined) => {
    const previous = rating
    setRating(next) // show the choice at once; undone below if saving fails
    setSaving(true)
    try {
      if (next) await api.messages.rate(messageId, next)
      else await api.messages.clearRating(messageId)
    } catch {
      setRating(previous)
      toast.error("Couldn't save your feedback. Please try again.")
    } finally {
      setSaving(false)
    }
  }

  const thumb = (value: 1 | -1) => {
    const active = rating?.rating === value
    const Icon = value === 1 ? ThumbsUp : ThumbsDown
    const label = value === 1 ? 'Helpful answer' : 'Not a good answer'
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={label}
            aria-pressed={active}
            disabled={saving}
            onClick={() => save(active ? undefined : { rating: value })}
            className={cn('text-muted-foreground', active && 'bg-accent text-primary')}
          >
            <Icon className={cn(active && 'fill-current')} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{active ? 'Remove rating' : label}</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <div className="mt-3 flex flex-wrap items-center gap-1">
      {thumb(1)}
      {thumb(-1)}
      {rating?.rating === -1 && (
        <div className="ml-1 flex flex-wrap items-center gap-1" role="group" aria-label="What was wrong? (optional)">
          {REASONS.map((r) => (
            <button
              key={r.value}
              type="button"
              disabled={saving}
              aria-pressed={rating.reason === r.value}
              onClick={() => save({ rating: -1, reason: rating.reason === r.value ? undefined : r.value })}
              className={cn(
                'rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted disabled:opacity-60',
                rating.reason === r.value && 'border-primary/40 bg-accent text-foreground',
              )}
            >
              {r.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
