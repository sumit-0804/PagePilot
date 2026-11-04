import { motion } from 'motion/react'
import {
  BookOpen,
  BrainCircuit,
  Compass,
  Globe,
  GraduationCap,
  Layers,
  Lightbulb,
  Microscope,
  Sparkles,
  WandSparkles,
} from 'lucide-react'
import { CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { cn } from '@/lib/utils'
import type { ExplanationStyle } from '@/types'

const GENERAL_STARTERS = [
  {
    text: 'Explain how vaccines train the immune system',
    hint: 'A clear explanation, step by step',
    icon: Lightbulb,
    badgeClass: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    spotlight: 'rgba(245, 158, 11, 0.18)',
  },
  {
    text: 'Quiz me on photosynthesis',
    hint: 'Interactive questions to test recall',
    icon: BrainCircuit,
    badgeClass: 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
    spotlight: 'rgba(244, 63, 94, 0.18)',
  },
  {
    text: 'Make flashcards on the French Revolution',
    hint: '3D flip deck for active memorization',
    icon: Layers,
    badgeClass: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
    spotlight: 'rgba(168, 85, 247, 0.18)',
  },
  {
    text: "What's new in AI research this month?",
    hint: 'Searches the web and cites its sources',
    icon: Globe,
    badgeClass: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
    spotlight: 'rgba(59, 130, 246, 0.18)',
  },
  {
    text: 'Summarize the causes of World War I',
    hint: 'Key takeaways in a few bullet points',
    icon: Sparkles,
    badgeClass: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-300',
    spotlight: 'rgba(34, 211, 238, 0.18)',
  },
  {
    text: 'Help me plan a week of exam revision',
    hint: 'High-yield topics & study strategy',
    icon: GraduationCap,
    badgeClass: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    spotlight: 'rgba(16, 185, 129, 0.18)',
  },
]

const DOCUMENT_STARTERS = [
  {
    text: 'Explain this document',
    hint: 'Clear overview of purpose & structure',
    icon: BookOpen,
    badgeClass: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
    spotlight: 'rgba(59, 130, 246, 0.18)',
  },
  {
    text: 'Summarize this document',
    hint: 'Key takeaways & section breakdown',
    icon: Sparkles,
    badgeClass: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-300',
    spotlight: 'rgba(34, 211, 238, 0.18)',
  },
  {
    text: 'What are the most important concepts?',
    hint: 'Core definitions & big ideas',
    icon: Lightbulb,
    badgeClass: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    spotlight: 'rgba(245, 158, 11, 0.18)',
  },
  {
    text: 'Generate a quiz from this document',
    hint: 'Interactive questions to test recall',
    icon: BrainCircuit,
    badgeClass: 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
    spotlight: 'rgba(244, 63, 94, 0.18)',
  },
  {
    text: 'Create flashcards',
    hint: '3D flip deck for active memorization',
    icon: Layers,
    badgeClass: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
    spotlight: 'rgba(168, 85, 247, 0.18)',
  },
  {
    text: 'Help me prepare for an exam',
    hint: 'High-yield topics & study strategy',
    icon: GraduationCap,
    badgeClass: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    spotlight: 'rgba(16, 185, 129, 0.18)',
  },
]

/** Starting points shown on an empty chat; each one is sent as written. */
export function SuggestedPrompts({
  onPick,
  disabled,
  variant = 'general',
}: {
  onPick: (text: string) => void
  disabled?: boolean
  /** "document" starters assume documents are attached. */
  variant?: 'general' | 'document'
}) {
  const starters = variant === 'document' ? DOCUMENT_STARTERS : GENERAL_STARTERS
  return (
    <div>
      <p className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
        {variant === 'document' ? 'Suggested study prompts' : 'Try asking'}
      </p>
      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {starters.map((s, idx) => {
          const Icon = s.icon
          return (
            <motion.div
              key={s.text}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: idx * 0.04, duration: 0.25 }}
            >
              <button
                type="button"
                disabled={disabled}
                onClick={() => onPick(s.text)}
                className="w-full text-left disabled:pointer-events-none disabled:opacity-50"
              >
                <CardSpotlight spotlightColor={s.spotlight} className="h-full rounded-xl p-4">
                  <div className="flex items-start gap-3">
                    <span
                      className={cn(
                        'mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-xl transition-transform duration-200 group-hover/spotlight:scale-110',
                        s.badgeClass,
                      )}
                    >
                      <Icon className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <span className="block text-sm font-semibold text-foreground transition-colors group-hover/spotlight:text-primary">
                        {s.text}
                      </span>
                      <span className="mt-0.5 block text-xs text-muted-foreground">{s.hint}</span>
                    </div>
                  </div>
                </CardSpotlight>
              </button>
            </motion.div>
          )
        })}
      </div>
    </div>
  )
}

const FOLLOW_UPS: {
  label: string
  message: string
  style?: ExplanationStyle
  icon: typeof Sparkles
  iconClass: string
}[] = [
  { label: 'Explain more simply', message: 'Explain that more simply.', style: 'simple', icon: WandSparkles, iconClass: 'text-emerald-500 dark:text-emerald-400' },
  { label: 'Give another example', message: 'Give me another example of that.', style: 'example', icon: Compass, iconClass: 'text-amber-500 dark:text-amber-400' },
  { label: 'Go deeper', message: 'Go deeper into that.', style: 'detailed', icon: Microscope, iconClass: 'text-purple-500 dark:text-purple-400' },
  { label: 'Test my understanding', message: 'Test my understanding of this topic.', icon: BrainCircuit, iconClass: 'text-rose-500 dark:text-rose-400' },
  { label: 'Create flashcards', message: 'Create flashcards on this topic.', icon: Layers, iconClass: 'text-cyan-500 dark:text-cyan-400' },
]

/** Next steps offered under the latest answer; they continue the same conversation. */
export function FollowUps({
  onPick,
  disabled,
}: {
  onPick: (text: string, style?: ExplanationStyle) => void
  disabled?: boolean
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      className="flex flex-wrap gap-2 pt-1"
      aria-label="Follow-up actions"
    >
      {FOLLOW_UPS.map((f) => {
        const Icon = f.icon
        return (
          <motion.button
            key={f.label}
            type="button"
            whileHover={{ y: -1, scale: 1.02 }}
            whileTap={{ scale: 0.98 }}
            disabled={disabled}
            onClick={() => onPick(f.message, f.style)}
            className="inline-flex items-center gap-1.5 rounded-full border border-border/80 bg-card/90 px-3.5 py-1.5 text-xs font-medium text-muted-foreground shadow-2xs transition-colors hover:border-primary/60 hover:bg-accent/50 hover:text-foreground disabled:opacity-50"
          >
            <Icon className={cn('size-3.5', f.iconClass)} />
            {f.label}
          </motion.button>
        )
      })}
    </motion.div>
  )
}
