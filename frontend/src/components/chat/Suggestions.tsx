import type { ExplanationStyle } from '@/types'

const STARTERS = [
  'Explain this PDF',
  'Summarize this document',
  'What are the most important concepts?',
  'Generate a quiz from this PDF',
  'Create flashcards',
  'Help me prepare for an exam',
]

/** Starting points shown on an empty chat; each one is sent as written. */
export function SuggestedPrompts({ onPick, disabled }: { onPick: (text: string) => void; disabled?: boolean }) {
  return (
    <div>
      <p className="text-sm text-muted-foreground">Try one of these, or ask your own question.</p>
      <div className="mt-3 flex flex-wrap gap-2">
        {STARTERS.map((s) => (
          <button
            key={s}
            disabled={disabled}
            onClick={() => onPick(s)}
            className="rounded-full border bg-card px-3.5 py-1.5 text-sm hover:border-primary hover:text-primary disabled:opacity-50"
          >
            {s}
          </button>
        ))}
      </div>
    </div>
  )
}

const FOLLOW_UPS: { label: string; message: string; style?: ExplanationStyle }[] = [
  { label: 'Explain more simply', message: 'Explain that more simply.', style: 'simple' },
  { label: 'Give another example', message: 'Give me another example of that.', style: 'example' },
  { label: 'Go deeper', message: 'Go deeper into that.', style: 'detailed' },
  { label: 'Test my understanding', message: 'Test my understanding of this topic.' },
  { label: 'Create flashcards', message: 'Create flashcards on this topic.' },
]

/** Next steps offered under the latest answer; they continue the same conversation. */
export function FollowUps({ onPick, disabled }: { onPick: (text: string, style?: ExplanationStyle) => void; disabled?: boolean }) {
  return (
    <div className="flex flex-wrap gap-1.5" aria-label="Follow-up actions">
      {FOLLOW_UPS.map((f) => (
        <button
          key={f.label}
          disabled={disabled}
          onClick={() => onPick(f.message, f.style)}
          className="rounded-full border bg-card px-3 py-1 text-sm text-muted-foreground hover:border-primary hover:text-primary disabled:opacity-50"
        >
          {f.label}
        </button>
      ))}
    </div>
  )
}
