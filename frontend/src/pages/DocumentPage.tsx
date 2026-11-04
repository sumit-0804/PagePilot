import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { BookOpen, Loader2 } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys, useConversations, useDocument } from '@/hooks/queries'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName, isProcessing, processingPercent, statusLabel, timeAgo } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/layout/PageHeader'
import { ConfirmDelete } from '@/components/layout/ConfirmDelete'
import { Composer } from '@/components/chat/Composer'
import { SuggestedPrompts } from '@/components/chat/Suggestions'
import { UploadDialog } from '@/components/documents/UploadDialog'
import type { ExplanationStyle } from '@/types'

export default function DocumentPage() {
  const { documentId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: doc, isLoading, error } = useDocument(documentId)
  const conversations = useConversations()
  const openPdf = useStudyStore((s) => s.openPdf)
  const setPending = useStudyStore((s) => s.setPendingMessage)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const start = useMutation({
    mutationFn: ({ text }: { text: string; style: ExplanationStyle | '' }) => api.conversations.create(documentId).then((conv) => ({ conv, text })),
    onSuccess: ({ conv, text }, { style }) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.conversations })
      setPending({ conversationId: conv.id, text, style: style || undefined })
      navigate(`/chat/${conv.id}`)
    },
    onError: (err) => toast.error(err.message),
  })

  const remove = useMutation({
    mutationFn: () => api.documents.remove(documentId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.documents })
      toast.success('Document deleted')
      navigate('/')
    },
    onError: (err) => toast.error(err.message),
  })

  if (isLoading) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto w-full max-w-3xl space-y-3 px-6 py-12">
          <Skeleton className="h-9 w-2/3" />
          <Skeleton className="h-4 w-40" />
        </div>
      </div>
    )
  }

  if (error || !doc) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto max-w-md px-6 py-16">
          <h1 className="font-reading text-2xl font-semibold">Document not found</h1>
          <p className="mt-2 text-muted-foreground">{error?.message ?? 'It may have been deleted.'}</p>
          <Link to="/" className="mt-6 inline-block font-semibold text-primary underline-offset-4 hover:underline">
            Go to your documents
          </Link>
        </div>
      </div>
    )
  }

  const name = displayName(doc.filename)
  const chats = conversations.data?.filter((c) => c.document_id === doc.id) ?? []
  const ready = doc.status === 'READY'

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={name}
        actions={
          ready && (
            <Button variant="outline" size="sm" onClick={() => openPdf(doc.id)}>
              <BookOpen />
              Open PDF
            </Button>
          )
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6">
          <h1 className="font-reading text-3xl font-semibold tracking-tight sm:text-4xl">{name}</h1>
          <p className="mt-2 text-muted-foreground">
            {doc.page_count ? `${doc.page_count} pages, uploaded ${timeAgo(doc.created_at)}` : `Uploaded ${timeAgo(doc.created_at)}`}
          </p>
          {ready && doc.ocr_pages > 0 && (
            <p className="mt-3 max-w-lg text-sm text-muted-foreground">
              {doc.ocr_pages === doc.page_count
                ? 'This PDF is scanned, so its text was read with OCR.'
                : `${doc.ocr_pages} of ${doc.page_count} pages are scanned, so their text was read with OCR.`}{' '}
              Check exact wording and numbers against the page.
            </p>
          )}

          {isProcessing(doc.status) && (
            <div className="mt-8 max-w-md" role="status">
              <p className="flex items-center gap-2 font-semibold">
                <Loader2 className="size-4 animate-spin" />
                {statusLabel(doc)}
              </p>
              <Progress value={processingPercent(doc)} className="mt-3" aria-label="Processing progress" />
              <p className="mt-3 text-sm text-muted-foreground">You can start asking questions as soon as it's ready. Long PDFs can take a few minutes.</p>
            </div>
          )}

          {doc.status === 'FAILED' && (
            <div className="mt-8 max-w-lg rounded-lg border border-destructive/30 bg-destructive/5 p-4" role="alert">
              <p className="font-semibold text-destructive">This PDF couldn't be processed</p>
              <p className="mt-1 text-sm">{doc.error_message}</p>
              <div className="mt-4 flex gap-2">
                <Button variant="outline" size="sm" onClick={() => setUploadOpen(true)}>
                  Upload a different file
                </Button>
                <Button variant="destructive" size="sm" onClick={() => setConfirmDelete(true)}>
                  Delete it
                </Button>
              </div>
            </div>
          )}

          {ready && (
            <>
              <div className="mt-8">
                <SuggestedPrompts disabled={start.isPending} onPick={(text) => start.mutate({ text, style: '' })} />
              </div>
              {chats.length > 0 && (
                <section className="mt-10" aria-labelledby="doc-chats">
                  <h2 id="doc-chats" className="font-semibold">
                    Your chats about this document
                  </h2>
                  <ul className="mt-2 divide-y rounded-lg border bg-card">
                    {chats.map((c) => (
                      <li key={c.id}>
                        <Link to={`/chat/${c.id}`} className="flex items-baseline justify-between gap-4 px-4 py-2.5 hover:bg-muted">
                          <span className="truncate">{c.title}</span>
                          <span className="shrink-0 text-sm text-muted-foreground">{timeAgo(c.updated_at)}</span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
            </>
          )}
        </div>
      </div>

      {ready && (
        <div className="mx-auto w-full max-w-3xl px-4 pb-4 sm:px-6">
          <Composer
            autoFocus
            documentName={name}
            busy={start.isPending}
            onSend={(text, style) => start.mutate({ text, style })}
            onAttach={() => setUploadOpen(true)}
          />
        </div>
      )}

      <UploadDialog open={uploadOpen} onOpenChange={setUploadOpen} onUploaded={(d) => navigate(`/documents/${d.id}`)} />
      <ConfirmDelete
        target={confirmDelete ? { kind: 'document', name: doc.filename } : null}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => {
          setConfirmDelete(false)
          remove.mutate()
        }}
      />
    </div>
  )
}
