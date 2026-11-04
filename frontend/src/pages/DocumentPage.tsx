import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { toast } from 'sonner'
import { ArrowRight, BookOpen, Clock, MessageSquare, ScanText } from 'lucide-react'
import { FormatIcon } from '@/components/documents/FormatIcon'
import { api } from '@/services/api'
import { queryKeys, useConversations, useDocument } from '@/hooks/queries'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName, isProcessing, processingPercent, sectionNoun, statusLabel, timeAgo } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'
import { CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { MultiStepLoader } from '@/components/ui/aceternity/multi-step-loader'
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
  const openViewer = useStudyStore((s) => s.openViewer)
  const setPending = useStudyStore((s) => s.setPendingMessage)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const start = useMutation({
    mutationFn: ({ text }: { text: string; style: ExplanationStyle | '' }) =>
      api.conversations.create({ document_ids: [documentId] }).then((conv) => ({ conv, text })),
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
        <div className="mx-auto w-full max-w-3xl space-y-4 px-6 py-12">
          <Skeleton className="h-10 w-2/3 rounded-xl" />
          <Skeleton className="h-5 w-44 rounded-lg" />
          <Skeleton className="mt-6 h-40 w-full rounded-2xl" />
        </div>
      </div>
    )
  }

  if (error || !doc) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader />
        <div className="mx-auto max-w-md px-6 py-16 text-center">
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
  const chats = conversations.data?.filter((c) => c.documents?.some((d) => d.id === doc.id)) ?? []
  const ready = doc.status === 'READY'

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <BackgroundAmbient variant="dots" className="opacity-45" />

      <PageHeader
        title={name}
        actions={
          ready && (
            <Button
              variant="outline"
              size="sm"
              className="rounded-xl border-primary/30 bg-card/80 hover:border-primary hover:bg-accent"
              onClick={() => openViewer(doc.id, { format: doc.format })}
            >
              <BookOpen className="text-primary" />
              Open document
            </Button>
          )
        }
      />

      <div className="relative z-10 min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-6xl px-6 py-10 lg:px-10">
          {/* Document Hero Banner */}
          <motion.div
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            className="rounded-2xl border border-border/80 bg-card/85 p-6 shadow-xs backdrop-blur-sm"
          >
            <div className="flex flex-wrap items-center gap-2 text-xs font-medium text-muted-foreground">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-accent px-2.5 py-0.5 font-semibold text-accent-foreground">
                <FormatIcon format={doc.format} className="size-3.5" />
                {doc.page_count
                  ? `${doc.page_count} ${sectionNoun(doc.format, doc.page_count)}`
                  : `${(doc.format ?? 'pdf').toUpperCase()} document`}
              </span>
              <span className="inline-flex items-center gap-1">
                <Clock className="size-3" />
                Uploaded {timeAgo(doc.created_at)}
              </span>
            </div>

            <h1 className="mt-3 font-reading text-3xl font-semibold tracking-tight sm:text-4xl">{name}</h1>

            {ready && doc.ocr_pages > 0 && (
              <div className="mt-4 flex items-start gap-2.5 rounded-xl border border-terracotta/30 bg-terracotta/8 px-3.5 py-2.5 text-xs text-foreground/90">
                <ScanText className="mt-0.5 size-4 shrink-0 text-terracotta" />
                <p>
                  {doc.ocr_pages === doc.page_count
                    ? 'This PDF is scanned, so its text was read with OCR.'
                    : `${doc.ocr_pages} of ${doc.page_count} pages are scanned, so their text was read with OCR.`}{' '}
                  Check exact wording and numbers against the page.
                </p>
              </div>
            )}
          </motion.div>

          {/* Processing Multi-step loader */}
          {isProcessing(doc.status) && (
            <div className="mt-6 max-w-xl">
              <MultiStepLoader
                status={doc.status}
                statusText={statusLabel(doc)}
                percent={processingPercent(doc)}
              />
            </div>
          )}

          {/* Failed state */}
          {doc.status === 'FAILED' && (
            <div className="mt-6 max-w-lg rounded-2xl border border-destructive/30 bg-destructive/8 p-5" role="alert">
              <p className="font-semibold text-destructive">This document couldn't be processed</p>
              <p className="mt-1 text-sm text-muted-foreground">{doc.error_message}</p>
              <div className="mt-4 flex gap-2">
                <Button variant="outline" size="sm" className="rounded-xl" onClick={() => setUploadOpen(true)}>
                  Upload a different file
                </Button>
                <Button variant="destructive" size="sm" className="rounded-xl" onClick={() => setConfirmDelete(true)}>
                  Delete it
                </Button>
              </div>
            </div>
          )}

          {/* Ready state: Starter prompts & Past conversations */}
          {ready && (
            <>
              <div className="mt-8">
                <SuggestedPrompts variant="document" disabled={start.isPending} onPick={(text) => start.mutate({ text, style: '' })} />
              </div>

              {chats.length > 0 && (
                <section className="mt-10" aria-labelledby="doc-chats">
                  <h2 id="doc-chats" className="mb-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
                    Your chats about this document
                  </h2>
                  <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    {chats.map((c) => (
                      <li key={c.id}>
                        <Link to={`/chat/${c.id}`} className="block">
                          <CardSpotlight className="rounded-xl px-4 py-3">
                            <div className="flex items-center justify-between gap-4">
                              <span className="flex min-w-0 items-center gap-2.5">
                                <MessageSquare className="size-4 shrink-0 text-primary" />
                                <span className="truncate font-medium group-hover/spotlight:text-primary">
                                  {c.title}
                                </span>
                              </span>
                              <span className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                                {timeAgo(c.updated_at)}
                                <ArrowRight className="size-3.5 transition-transform group-hover/spotlight:translate-x-0.5" />
                              </span>
                            </div>
                          </CardSpotlight>
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
        <div className="relative z-10 mx-auto w-full max-w-6xl px-6 pt-2 pb-4 lg:px-10">
          <Composer
            autoFocus
            documents={[doc]}
            busy={start.isPending}
            onSend={(text, style) => start.mutate({ text, style })}
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
