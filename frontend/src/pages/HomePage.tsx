import { Link, useNavigate } from 'react-router-dom'
import { FileText } from 'lucide-react'
import { useConversations, useDocuments } from '@/hooks/queries'
import { displayName, isProcessing, statusLabel, timeAgo } from '@/lib/format'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/layout/PageHeader'
import { UploadDropzone } from '@/components/documents/UploadDropzone'

export default function HomePage() {
  const navigate = useNavigate()
  const documents = useDocuments()
  const conversations = useConversations()
  const hasDocs = (documents.data?.length ?? 0) > 0
  const recentChats = conversations.data?.slice(0, 5) ?? []
  const docName = (id: string) => {
    const d = documents.data?.find((doc) => doc.id === id)
    return d ? displayName(d.filename) : ''
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl px-4 py-12 sm:px-6">
          {documents.isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-10 w-3/4" />
              <Skeleton className="h-40 w-full" />
            </div>
          ) : !hasDocs ? (
            <>
              <h1 className="max-w-xl font-reading text-4xl leading-tight font-semibold tracking-tight">Study any PDF by asking it questions.</h1>
              <p className="mt-4 max-w-xl text-lg text-muted-foreground">
                Every answer marks the page it came from. When you're ready, quiz yourself or turn the key ideas into flashcards.
              </p>
              <div className="mt-8">
                <UploadDropzone onUploaded={(doc) => navigate(`/documents/${doc.id}`)} />
              </div>
            </>
          ) : (
            <>
              <h1 className="font-reading text-3xl font-semibold tracking-tight">Pick up where you left off</h1>

              {recentChats.length > 0 && (
                <section className="mt-8" aria-labelledby="recent-chats">
                  <h2 id="recent-chats" className="font-semibold">
                    Recent chats
                  </h2>
                  <ul className="mt-2 divide-y rounded-lg border bg-card">
                    {recentChats.map((c) => (
                      <li key={c.id}>
                        <Link to={`/chat/${c.id}`} className="flex items-baseline gap-4 px-4 py-3 hover:bg-muted">
                          <span className="min-w-0 flex-1">
                            <span className="block truncate">{c.title}</span>
                            <span className="block truncate text-sm text-muted-foreground">{docName(c.document_id)}</span>
                          </span>
                          <span className="shrink-0 text-sm text-muted-foreground">{timeAgo(c.updated_at)}</span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </section>
              )}

              <section className="mt-8" aria-labelledby="home-docs">
                <h2 id="home-docs" className="font-semibold">
                  Your documents
                </h2>
                <ul className="mt-2 divide-y rounded-lg border bg-card">
                  {documents.data?.map((d) => (
                    <li key={d.id}>
                      <Link to={`/documents/${d.id}`} className="flex items-center gap-3 px-4 py-3 hover:bg-muted">
                        <FileText className="size-4 shrink-0 text-muted-foreground" />
                        <span className="min-w-0 flex-1 truncate">{displayName(d.filename)}</span>
                        <span className={d.status === 'FAILED' ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>
                          {isProcessing(d.status) || d.status === 'FAILED' ? statusLabel(d) : `${d.page_count} pages`}
                        </span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>

              <section className="mt-8" aria-labelledby="home-upload">
                <h2 id="home-upload" className="mb-2 font-semibold">
                  Add another PDF
                </h2>
                <UploadDropzone compact onUploaded={(doc) => navigate(`/documents/${doc.id}`)} />
              </section>
            </>
          )}
          {documents.error && (
            <p role="alert" className="mt-6 text-destructive">
              {documents.error.message}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
