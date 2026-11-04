import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { useStudyStore, type ViewerTarget } from '@/stores/useStudyStore'
import { displayName } from '@/lib/format'
import { PdfViewer } from '@/components/pdf/PdfViewer'

export function PdfPanel({ target }: { target: ViewerTarget }) {
  const closeViewer = useStudyStore((s) => s.closeViewer)
  const documents = useDocuments()
  const { documentId } = target

  // Signed URLs last 10 minutes; refresh a little before that.
  const file = useQuery({
    queryKey: queryKeys.fileUrl(documentId),
    queryFn: () => api.documents.fileUrl(documentId),
    staleTime: 8 * 60_000,
    gcTime: 8 * 60_000,
  })

  const doc = documents.data?.find((d) => d.id === documentId)

  return (
    <PdfViewer
      key={documentId}
      url={file.data?.url}
      urlError={file.error?.message}
      title={doc ? displayName(doc.filename) : 'PDF'}
      target={target}
      onClose={closeViewer}
    />
  )
}
