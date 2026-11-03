import { CitationChip } from '@/components/chat/CitationChip'

export function QuizCitation({ documentId, page, excerpt }: { documentId: string; page?: number; excerpt?: string }) {
  if (!page) return null
  return (
    <div className="flex items-start gap-2 text-sm">
      <span className="shrink-0 pt-0.5 text-muted-foreground">Source</span>
      <CitationChip documentId={documentId} page={page} />
      {excerpt && <q className="font-reading text-muted-foreground italic">{excerpt}</q>}
    </div>
  )
}
