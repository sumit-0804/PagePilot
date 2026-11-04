import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Loader2, X } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { useStudyStore, type ViewerTarget } from '@/stores/useStudyStore'
import { displayName, sectionNoun } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { FormatIcon } from '@/components/documents/FormatIcon'
import { cn } from '@/lib/utils'

/** Shows one section (slide, heading block) of a non-PDF document as plain text. */
export function TextPanel({ target }: { target: ViewerTarget }) {
  const closeViewer = useStudyStore((s) => s.closeViewer)
  const documents = useDocuments()
  const doc = documents.data?.find((d) => d.id === target.documentId)
  const [section, setSection] = useState(target.page)
  const [seenNonce, setSeenNonce] = useState(target.nonce)

  // A new citation click jumps to its section, even within the same document.
  if (target.nonce !== seenNonce) {
    setSeenNonce(target.nonce)
    setSection(target.page)
  }

  const query = useQuery({
    queryKey: queryKeys.section(target.documentId, section),
    queryFn: () => api.documents.section(target.documentId, section),
    staleTime: Infinity,
  })

  const format = target.format ?? doc?.format
  const total = doc?.page_count ?? 0
  const title = doc ? displayName(doc.filename) : 'Document'
  const isCited = target.fromCitation && section === target.page

  return (
    <section className="flex h-full flex-col bg-muted/60" aria-label={`Document viewer: ${title}`}>
      <header className="glass-panel flex h-13 items-center gap-2 border-b border-border/80 px-3">
        <FormatIcon format={format} className="size-4 shrink-0 text-primary" />
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold" title={title}>
          {title}
        </h2>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={`Previous ${sectionNoun(format)}`}
          disabled={section <= 1}
          onClick={() => setSection((s) => s - 1)}
        >
          <ChevronLeft />
        </Button>
        <span className="min-w-14 text-center text-xs font-medium tabular-nums text-muted-foreground">
          {total ? `${section} / ${total}` : section}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={`Next ${sectionNoun(format)}`}
          disabled={total > 0 && section >= total}
          onClick={() => setSection((s) => s + 1)}
        >
          <ChevronRight />
        </Button>
        <Button variant="ghost" size="icon-sm" aria-label="Close document" onClick={closeViewer}>
          <X />
        </Button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {query.isLoading && (
          <div className="grid h-full place-items-center" role="status">
            <Loader2 className="size-5 animate-spin text-primary" aria-label="Loading section" />
          </div>
        )}
        {query.error && (
          <p role="alert" className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
            {query.error.message}
          </p>
        )}
        {query.data && (
          <article
            className={cn(
              'mx-auto max-w-2xl rounded-2xl border bg-card p-5 shadow-xs',
              isCited ? 'border-primary/50 ring-2 ring-primary/20' : 'border-border/80',
            )}
          >
            <p className="mb-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
              {query.data.label} {query.data.number}
              {isCited && <span className="ml-2 normal-case tracking-normal text-primary">· Cited</span>}
            </p>
            <div className="reading text-sm leading-relaxed whitespace-pre-wrap">{query.data.text}</div>
          </article>
        )}
      </div>
    </section>
  )
}
