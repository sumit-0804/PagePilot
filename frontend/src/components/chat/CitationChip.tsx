import { motion } from 'motion/react'
import { FileText, Globe } from 'lucide-react'
import { useStudyStore } from '@/stores/useStudyStore'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { citationLabel, hostname, safeUrl, sourceName } from '@/lib/citations'
import type { Citation } from '@/types'

const CHIP =
  'highlighter inline-flex items-baseline gap-1 px-[0.48em] py-[0.08em] font-sans text-[0.78em] font-semibold tabular-nums whitespace-nowrap transition-all hover:brightness-95'

/** A source chip: web sources open the link, document sources open the viewer at that page or section. */
export function CitationChip({ citation, className }: { citation: Citation; className?: string }) {
  const openViewer = useStudyStore((s) => s.openViewer)
  const url = citation.kind === 'web' ? safeUrl(citation.url) : null
  const heading = sourceName(citation)
  const label = citationLabel(citation)

  let chip
  if (url) {
    chip = (
      <motion.a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        whileHover={{ y: -1, scale: 1.04 }}
        whileTap={{ scale: 0.96 }}
        aria-label={`Source ${label}: ${heading} (opens in a new tab)`}
        className={cn(CHIP, 'no-underline', className)}
      >
        {label}
      </motion.a>
    )
  } else if (citation.kind !== 'web' && citation.document_id) {
    const documentId = citation.document_id
    chip = (
      <motion.button
        type="button"
        whileHover={{ y: -1, scale: 1.04 }}
        whileTap={{ scale: 0.96 }}
        onClick={() => openViewer(documentId, { page: citation.page_number ?? 1, fromCitation: true })}
        aria-label={`Source ${label}: ${heading}`}
        className={cn(CHIP, className)}
      >
        {label}
      </motion.button>
    )
  } else {
    return <UnverifiedCitation label={label} />
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{chip}</TooltipTrigger>
      <TooltipContent className="max-w-xs rounded-xl border border-border/80 bg-popover/95 p-3 text-popover-foreground shadow-xl backdrop-blur-md">
        <div className="mb-1 flex items-center gap-1.5 font-sans text-xs font-semibold text-primary">
          {url ? <Globe className="size-3 shrink-0" /> : <FileText className="size-3 shrink-0" />}
          <span className="truncate">{heading}</span>
        </div>
        {url && <p className="mb-1 truncate font-sans text-[0.7rem] text-muted-foreground">{hostname(url)}</p>}
        {citation.snippet && <p className="font-reading text-xs leading-relaxed italic">“{citation.snippet}”</p>}
      </TooltipContent>
    </Tooltip>
  )
}

/** A marker while the answer is still streaming; its source isn't known yet. */
export function PendingCitation({ label }: { label: string }) {
  return <span className={cn(CHIP, 'opacity-70')}>{label}</span>
}

/** A marker the model wrote that matches no source: shown, but not as evidence. */
export function UnverifiedCitation({ label }: { label: string }) {
  return (
    <span
      className="rounded-md bg-muted px-1.5 py-0.5 font-sans text-[0.78em] text-muted-foreground"
      title="This doesn't match any source the answer used"
    >
      [{label}]
    </span>
  )
}

/** A page or section reference (quizzes, flashcards, progress) that opens the document viewer. */
export function PageChip({ documentId, page, className }: { documentId: string; page: number; className?: string }) {
  const openViewer = useStudyStore((s) => s.openViewer)
  return (
    <motion.button
      type="button"
      whileHover={{ y: -1, scale: 1.04 }}
      whileTap={{ scale: 0.96 }}
      onClick={() => openViewer(documentId, { page, fromCitation: true })}
      aria-label={`Open part ${page} of the document`}
      className={cn(CHIP, className)}
    >
      <span>p. {page}</span>
    </motion.button>
  )
}
