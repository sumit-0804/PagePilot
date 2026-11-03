import { useStudyStore } from '@/stores/useStudyStore'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/** A page reference from the PDF, drawn as a highlighter mark that opens the page. */
export function CitationChip({ documentId, page, snippet, className }: { documentId: string; page: number; snippet?: string; className?: string }) {
  const openPdf = useStudyStore((s) => s.openPdf)
  const chip = (
    <button
      type="button"
      onClick={() => openPdf(documentId, page, true)}
      aria-label={`Open page ${page} of the PDF`}
      className={cn(
        'highlighter inline-flex items-baseline px-[0.4em] py-[0.05em] font-sans text-[0.8em] font-semibold tabular-nums whitespace-nowrap hover:brightness-95',
        className,
      )}
    >
      p. {page}
    </button>
  )
  if (!snippet) return chip
  return (
    <Tooltip>
      <TooltipTrigger asChild>{chip}</TooltipTrigger>
      <TooltipContent className="max-w-xs font-reading text-sm leading-snug">{snippet}</TooltipContent>
    </Tooltip>
  )
}

/** A page the model mentioned that wasn't among the retrieved pages: shown, but not as evidence. */
export function UnverifiedPage({ page }: { page: number }) {
  return (
    <span className="font-sans text-[0.8em] text-muted-foreground" title="This page wasn't among the passages the answer was based on">
      p. {page}
    </span>
  )
}
