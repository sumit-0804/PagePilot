import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import { queryKeys, useDocuments } from '@/hooks/queries'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName } from '@/lib/format'
import { PdfViewer } from '@/components/pdf/PdfViewer'

export function PdfPanel() {
  const pdf = useStudyStore((s) => s.pdf)
  const closePdf = useStudyStore((s) => s.closePdf)
  const documents = useDocuments()
  const documentId = pdf?.documentId ?? ''

  // Signed URLs last 10 minutes; refresh a little before that.
  const file = useQuery({
    queryKey: queryKeys.fileUrl(documentId),
    queryFn: () => api.documents.fileUrl(documentId),
    enabled: !!pdf,
    staleTime: 8 * 60_000,
    gcTime: 8 * 60_000,
  })

  if (!pdf) return null
  const doc = documents.data?.find((d) => d.id === pdf.documentId)

  return (
    <PdfViewer
      key={pdf.documentId}
      url={file.data?.url}
      urlError={file.error?.message}
      title={doc ? displayName(doc.filename) : 'PDF'}
      target={pdf}
      onClose={closePdf}
    />
  )
}
