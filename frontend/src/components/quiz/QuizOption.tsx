import { motion } from 'motion/react'
import { Check, X } from 'lucide-react'
import { cn } from '@/lib/utils'

export type OptionState = 'idle' | 'selected' | 'correct' | 'incorrect' | 'dim'

const LETTERS = ['A', 'B', 'C', 'D']

export function QuizOption({
  index,
  text,
  state,
  onSelect,
  disabled,
}: {
  index: number
  text: string
  state: OptionState
  onSelect: () => void
  disabled: boolean
}) {
  return (
    <motion.button
      type="button"
      role="radio"
      aria-checked={state === 'selected'}
      disabled={disabled}
      onClick={onSelect}
      whileHover={!disabled ? { x: 4 } : undefined}
      whileTap={!disabled ? { scale: 0.99 } : undefined}
      transition={{ type: 'spring', stiffness: 400, damping: 28 }}
      className={cn(
        'flex w-full items-start gap-3.5 rounded-xl border bg-card px-4 py-3 text-left transition-colors',
        !disabled && 'hover:border-primary/60 hover:bg-accent/30',
        state === 'selected' && 'border-primary bg-accent shadow-xs',
        state === 'correct' && 'border-success bg-success/10',
        state === 'incorrect' && 'border-destructive bg-destructive/10',
        state === 'dim' && 'opacity-55',
      )}
    >
      <span
        className={cn(
          'grid size-6 shrink-0 place-items-center rounded-full border text-xs font-semibold transition-colors',
          state === 'idle' && 'border-border bg-muted/60 text-muted-foreground',
          state === 'selected' && 'border-primary bg-primary text-primary-foreground',
          state === 'correct' && 'border-success bg-success text-white',
          state === 'incorrect' && 'border-destructive bg-destructive text-white',
        )}
        aria-hidden
      >
        {state === 'correct' ? (
          <Check className="size-3.5" />
        ) : state === 'incorrect' ? (
          <X className="size-3.5" />
        ) : (
          (LETTERS[index] ?? index + 1)
        )}
      </span>
      <span className="pt-0.5 leading-snug">{text}</span>
    </motion.button>
  )
}
