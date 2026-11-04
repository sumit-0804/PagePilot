import { motion } from 'motion/react'
import { Check, Loader2, Sparkles } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DocumentStatus } from '@/types'

const STEPS: { key: DocumentStatus; label: string; detail: string }[] = [
  { key: 'UPLOADED', label: 'Document received', detail: 'Validating PDF structure and metadata' },
  { key: 'EXTRACTING', label: 'Reading pages & OCR', detail: 'Extracting typography, formulas, and scanned pages' },
  { key: 'CHUNKING', label: 'Structuring passages', detail: 'Segmenting pages into context-rich study blocks' },
  { key: 'EMBEDDING', label: 'Building semantic index', detail: 'Creating page-verified vector embeddings for instant Q&A' },
]

const ORDER: Record<DocumentStatus, number> = {
  UPLOADED: 0,
  EXTRACTING: 1,
  CHUNKING: 2,
  EMBEDDING: 3,
  INDEXING: 3,
  READY: 4,
  FAILED: -1,
}

/**
 * Aceternity Multi-Step Loader card for document processing.
 */
export function MultiStepLoader({
  status,
  statusText,
  percent,
}: {
  status: DocumentStatus
  statusText: string
  percent: number
}) {
  const currentIdx = ORDER[status] ?? 0

  return (
    <div
      role="status"
      className="relative overflow-hidden rounded-2xl border border-primary/25 bg-card/90 p-6 shadow-lg shadow-primary/5 backdrop-blur-md"
    >
      <div className="pointer-events-none absolute -top-20 -right-20 h-48 w-48 rounded-full bg-primary/10 blur-3xl" aria-hidden />
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2.5">
          <span className="inline-flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Sparkles className="size-4 animate-pulse" />
          </span>
          <div>
            <p className="font-semibold text-foreground">{statusText}</p>
            <p className="text-xs text-muted-foreground">You can ask questions as soon as indexing completes</p>
          </div>
        </div>
        <span className="rounded-full bg-accent px-2.5 py-1 text-xs font-semibold tabular-nums text-accent-foreground">
          {Math.round(percent)}%
        </span>
      </div>

      {/* Animated gradient progress bar */}
      <div className="mt-4 h-2 w-full overflow-hidden rounded-full bg-secondary">
        <motion.div
          className="h-full rounded-full bg-gradient-to-r from-primary via-terracotta to-primary"
          initial={{ width: 0 }}
          animate={{ width: `${Math.max(percent, 8)}%` }}
          transition={{ duration: 0.5, ease: 'easeOut' }}
        />
      </div>

      {/* Multi-step checklist */}
      <ol className="mt-5 space-y-3">
        {STEPS.map((step, idx) => {
          const isDone = idx < currentIdx
          const isCurrent = idx === currentIdx
          return (
            <motion.li
              key={step.key}
              initial={{ opacity: 0, x: -8 }}
              animate={{ opacity: idx <= currentIdx + 1 ? 1 : 0.45, x: 0 }}
              transition={{ delay: idx * 0.08 }}
              className="flex items-start gap-3"
            >
              <span
                className={cn(
                  'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border text-xs transition-colors',
                  isDone && 'border-primary bg-primary text-primary-foreground',
                  isCurrent && 'border-primary bg-primary/15 text-primary',
                  !isDone && !isCurrent && 'border-border bg-muted text-muted-foreground',
                )}
              >
                {isDone ? (
                  <Check className="size-3" />
                ) : isCurrent ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <span className="size-1.5 rounded-full bg-muted-foreground/50" />
                )}
              </span>
              <div className="min-w-0 flex-1">
                <p className={cn('text-sm leading-tight font-medium', isCurrent && 'text-primary font-semibold')}>
                  {step.label}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">{step.detail}</p>
              </div>
            </motion.li>
          )
        })}
      </ol>
    </div>
  )
}
