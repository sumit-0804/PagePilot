import { Link } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { api } from '@/services/api'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { displayName, timeAgo } from '@/lib/format'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/layout/PageHeader'
import { CitationChip } from '@/components/chat/CitationChip'
import type { Progress, StudyDocument } from '@/types'

export default function HistoryPage() {
  const documents = useDocuments()
  const ready = documents.data?.filter((d) => d.status === 'READY') ?? []
  const progress = useQueries({
    queries: ready.map((d) => ({ queryKey: queryKeys.progress(d.id), queryFn: () => api.progress.get(d.id) })),
  })

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Study progress" />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6">
          <h1 className="font-reading text-3xl font-semibold tracking-tight">Study progress</h1>
          <p className="mt-2 text-muted-foreground">How your quizzes and flashcards are going, and what to review next.</p>

          {documents.isLoading && <Skeleton className="mt-8 h-40 w-full" />}
          {!documents.isLoading && ready.length === 0 && (
            <p className="mt-8">
              Nothing to show yet.{' '}
              <Link to="/" className="font-semibold text-primary underline-offset-4 hover:underline">
                Upload a PDF
              </Link>{' '}
              and take a quiz to see your progress here.
            </p>
          )}

          <div className="mt-8 space-y-10">
            {ready.map((doc, i) => (
              <DocumentProgress key={doc.id} doc={doc} progress={progress[i]?.data} error={progress[i]?.error?.message} />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function DocumentProgress({ doc, progress, error }: { doc: StudyDocument; progress?: Progress; error?: string }) {
  return (
    <section aria-labelledby={`progress-${doc.id}`}>
      <div className="flex items-baseline justify-between gap-4 border-b pb-2">
        <h2 id={`progress-${doc.id}`} className="truncate font-reading text-xl font-semibold">
          <Link to={`/documents/${doc.id}`} className="hover:underline">
            {displayName(doc.filename)}
          </Link>
        </h2>
        {progress?.last_studied_at && <span className="shrink-0 text-sm text-muted-foreground">Last quiz {timeAgo(progress.last_studied_at)}</span>}
      </div>

      {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
      {!progress && !error && <Skeleton className="mt-3 h-20 w-full" />}

      {progress && (
        <>
          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4">
            <Stat label="Quizzes finished" value={progress.quizzes_completed} />
            <Stat label="Average score" value={progress.quizzes_completed ? `${progress.average_score_pct}%` : '—'} />
            <Stat label="Questions right" value={`${progress.correct_answers} of ${progress.questions_answered}`} />
            <Stat label="Flashcards known" value={`${progress.flashcards.known} of ${progress.flashcards.total}`} />
          </dl>

          {progress.weak_areas.length > 0 && (
            <div className="mt-5">
              <h3 className="font-semibold">Pages to review</h3>
              <ul className="mt-2 space-y-2">
                {progress.weak_areas.map((w) => (
                  <li key={w.page} className="flex items-start gap-3">
                    <CitationChip documentId={doc.id} page={w.page} />
                    <span className="text-sm">
                      Missed {w.missed} of {w.attempted}
                      {w.example_question && <span className="block font-reading text-muted-foreground">{w.example_question}</span>}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {progress.recommendations.length > 0 && (
            <div className="mt-5">
              <h3 className="font-semibold">Next steps</h3>
              <ul className="mt-2 list-disc space-y-1 pl-5">
                {progress.recommendations.map((r) => (
                  <li key={r}>{r}</li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  )
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="font-reading text-2xl font-semibold tabular-nums">{value}</dd>
    </div>
  )
}
