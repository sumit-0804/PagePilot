import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { motion } from 'motion/react'
import {
  AlertTriangle,
  Award,
  BookOpen,
  BrainCircuit,
  CheckCircle2,
  Compass,
  Layers,
  Sparkles,
} from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { displayName, timeAgo } from '@/lib/format'
import { Skeleton } from '@/components/ui/skeleton'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'
import { CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { PageHeader } from '@/components/layout/PageHeader'
import { PageChip } from '@/components/chat/CitationChip'
import type { Progress, StudyDocument } from '@/types'

export default function HistoryPage() {
  const documents = useDocuments()
  const ready = documents.data?.filter((d) => d.status === 'READY') ?? []
  const progress = useQueries({
    queries: ready.map((d) => ({ queryKey: queryKeys.progress(d.id), queryFn: () => api.progress.get(d.id) })),
  })

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <BackgroundAmbient variant="grid" className="opacity-45" />
      <PageHeader title="Study progress" />
      <div className="relative z-10 min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-6xl px-6 py-10 lg:px-10">
          <div className="inline-flex items-center gap-1.5 rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
            <Sparkles className="size-3.5" />
            Analytics & Mastery
          </div>
          <h1 className="mt-2.5 font-reading text-3xl font-semibold tracking-tight sm:text-4xl">Study progress</h1>
          <p className="mt-2 text-muted-foreground">
            How your quizzes and flashcards are going, and which pages to review next.
          </p>

          {documents.isLoading && <Skeleton className="mt-8 h-48 w-full rounded-2xl" />}
          {!documents.isLoading && ready.length === 0 && (
            <div className="mt-8 rounded-2xl border border-border/80 bg-card/85 p-8 text-center">
              <p className="text-muted-foreground">
                Nothing to show yet.{' '}
                <Link to="/" className="font-semibold text-primary underline-offset-4 hover:underline">
                  Upload a document
                </Link>{' '}
                and take a quiz to see your progress here.
              </p>
            </div>
          )}

          <div className="mt-8 space-y-10">
            {ready.map((doc, i) => (
              <DocumentProgress
                key={doc.id}
                doc={doc}
                progress={progress[i]?.data}
                error={progress[i]?.error?.message}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function DocumentProgress({ doc, progress, error }: { doc: StudyDocument; progress?: Progress; error?: string }) {
  return (
    <motion.section
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      aria-labelledby={`progress-${doc.id}`}
      className="rounded-2xl border border-border/80 bg-card/85 p-6 shadow-xs backdrop-blur-sm"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-4 border-b border-border/70 pb-4">
        <div className="flex items-center gap-2.5">
          <span className="flex size-9 items-center justify-center rounded-xl bg-accent text-accent-foreground">
            <BookOpen className="size-4" />
          </span>
          <h2 id={`progress-${doc.id}`} className="truncate font-reading text-xl font-semibold">
            <Link to={`/documents/${doc.id}`} className="transition-colors hover:text-primary hover:underline">
              {displayName(doc.filename)}
            </Link>
          </h2>
        </div>
        {progress?.last_studied_at && (
          <span className="shrink-0 rounded-full bg-secondary px-3 py-1 text-xs font-medium text-muted-foreground">
            Last quiz {timeAgo(progress.last_studied_at)}
          </span>
        )}
      </div>

      {error && <p className="mt-4 text-sm text-destructive">{error}</p>}
      {!progress && !error && <Skeleton className="mt-4 h-28 w-full rounded-xl" />}

      {progress && (
        <>
          {/* Bento 4-column stat cards */}
          <dl className="mt-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat
              icon={<BrainCircuit className="size-4 text-primary" />}
              label="Quizzes finished"
              value={progress.quizzes_completed}
            />
            <Stat
              icon={<Award className="size-4 text-terracotta" />}
              label="Average score"
              value={progress.quizzes_completed ? `${progress.average_score_pct}%` : '—'}
            />
            <Stat
              icon={<CheckCircle2 className="size-4 text-success" />}
              label="Questions right"
              value={`${progress.correct_answers} of ${progress.questions_answered}`}
            />
            <Stat
              icon={<Layers className="size-4 text-primary" />}
              label="Flashcards known"
              value={`${progress.flashcards.known} of ${progress.flashcards.total}`}
            />
          </dl>

          <div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2">
            {/* Weak areas / pages to review */}
            {progress.weak_areas.length > 0 && (
              <div className="rounded-xl border border-terracotta/25 bg-terracotta/5 p-4">
                <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                  <AlertTriangle className="size-4 text-terracotta" />
                  Pages to review
                </h3>
                <ul className="mt-3 space-y-2.5">
                  {progress.weak_areas.map((w) => (
                    <li key={w.page} className="flex items-start gap-3 rounded-lg bg-card/80 p-2.5">
                      <PageChip documentId={doc.id} page={w.page} />
                      <span className="min-w-0 flex-1 text-sm">
                        <span className="font-medium">
                          Missed {w.missed} of {w.attempted}
                        </span>
                        {w.example_question && (
                          <span className="mt-0.5 block font-reading text-xs text-muted-foreground">
                            {w.example_question}
                          </span>
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* Next steps recommendations */}
            {progress.recommendations.length > 0 && (
              <div className="rounded-xl border border-primary/20 bg-accent/35 p-4">
                <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                  <Compass className="size-4 text-primary" />
                  Next steps
                </h3>
                <ul className="mt-3 space-y-2 text-sm">
                  {progress.recommendations.map((r) => (
                    <li key={r} className="flex items-start gap-2 rounded-lg bg-card/80 px-3 py-2">
                      <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary" />
                      <span>{r}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </>
      )}
    </motion.section>
  )
}

function Stat({ icon, label, value }: { icon: React.ReactNode; label: string; value: string | number }) {
  return (
    <CardSpotlight className="rounded-xl p-3.5">
      <div className="flex items-center justify-between gap-2">
        <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
        {icon}
      </div>
      <dd className="mt-2 font-reading text-2xl font-semibold tabular-nums">{value}</dd>
    </CardSpotlight>
  )
}
