import { PageChip } from '@/components/chat/CitationChip'

/** The page a quiz answer came from; hidden for quizzes made without a document. */
export function QuizCitation({ documentId, page, excerpt }: { documentId?: string; page?: number; excerpt?: string }) {
  if (!page || !documentId) return null
  return (
    <div className="flex items-start gap-2 text-sm">
      <span className="shrink-0 pt-0.5 text-muted-foreground">Source</span>
      <PageChip documentId={documentId} page={page} />
      {excerpt && <q className="font-reading text-muted-foreground italic">{excerpt}</q>}
    </div>
  )
}
