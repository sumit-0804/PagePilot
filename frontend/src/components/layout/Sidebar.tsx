import { useState } from 'react'
import { Link, NavLink, useMatch, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ChartNoAxesColumn,
  FileText,
  LogOut,
  MoreHorizontal,
  PanelLeftClose,
  Plus,
  Settings,
  SquarePen,
  Trash2,
  AlertCircle,
} from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys, useConversations, useDocuments } from '@/hooks/queries'
import { useAuth } from '@/hooks/useAuth'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName, isProcessing, processingPercent, statusLabel, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Progress } from '@/components/ui/progress'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { ConfirmDelete } from '@/components/layout/ConfirmDelete'
import { UploadDialog } from '@/components/documents/UploadDialog'

export function Sidebar({ onHide, onNavigate }: { onHide: () => void; onNavigate?: () => void }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { session, signOut } = useAuth()
  const closePdf = useStudyStore((s) => s.closePdf)
  const documents = useDocuments()
  const conversations = useConversations()
  const docMatch = useMatch('/documents/:documentId')
  const chatMatch = useMatch('/chat/:conversationId')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [toDelete, setToDelete] = useState<{ kind: 'document' | 'chat'; id: string; name: string } | null>(null)

  const activeChat = conversations.data?.find((c) => c.id === chatMatch?.params.conversationId)
  const activeDocId = docMatch?.params.documentId ?? activeChat?.document_id

  const remove = useMutation({
    mutationFn: (target: { kind: 'document' | 'chat'; id: string }) =>
      target.kind === 'document' ? api.documents.remove(target.id) : api.conversations.remove(target.id),
    onSuccess: async (_, target) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.documents }),
        queryClient.invalidateQueries({ queryKey: queryKeys.conversations }),
      ])
      const viewingIt =
        (target.kind === 'chat' && chatMatch?.params.conversationId === target.id) ||
        (target.kind === 'document' && activeDocId === target.id)
      if (viewingIt) {
        closePdf()
        navigate('/')
      }
      toast.success(target.kind === 'document' ? 'Document deleted' : 'Chat deleted')
    },
    onError: (err) => toast.error(err.message),
  })

  const go = (to: string) => {
    navigate(to)
    onNavigate?.()
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between px-4 pt-4 pb-3">
        <Link to="/" onClick={onNavigate} className="font-reading text-xl font-semibold tracking-tight">
          PagePilot
        </Link>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Hide sidebar" onClick={onHide}>
              <PanelLeftClose />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Hide sidebar</TooltipContent>
        </Tooltip>
      </div>

      <div className="px-3">
        <Button className="w-full justify-start" size="lg" onClick={() => go(activeDocId ? `/documents/${activeDocId}` : '/')}>
          <SquarePen />
          New chat
        </Button>
      </div>

      <ScrollArea className="mt-4 min-h-0 flex-1">
        <section className="px-3" aria-labelledby="sidebar-docs">
          <div className="flex items-center justify-between px-1 pb-1">
            <h2 id="sidebar-docs" className="text-sm font-semibold text-muted-foreground">
              Documents
            </h2>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon-xs" aria-label="Upload a PDF" onClick={() => setUploadOpen(true)}>
                  <Plus />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Upload a PDF</TooltipContent>
            </Tooltip>
          </div>
          {documents.isLoading && <p className="px-1 py-2 text-sm text-muted-foreground">Loading…</p>}
          {documents.error && <p className="px-1 py-2 text-sm text-destructive">{documents.error.message}</p>}
          {documents.data?.length === 0 && <p className="px-1 py-2 text-sm text-muted-foreground">No documents yet.</p>}
          <ul className="space-y-0.5">
            {documents.data?.map((doc) => (
              <li key={doc.id} className="group relative">
                <button
                  onClick={() => go(`/documents/${doc.id}`)}
                  className={cn(
                    'flex w-full items-start gap-2 rounded-md px-2 py-1.5 pr-8 text-left text-sm hover:bg-sidebar-accent',
                    activeDocId === doc.id && 'bg-sidebar-accent font-semibold',
                  )}
                >
                  {doc.status === 'FAILED' ? (
                    <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
                  ) : (
                    <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{displayName(doc.filename)}</span>
                    {isProcessing(doc.status) && (
                      <span className="mt-1 block space-y-1">
                        <span className="block text-xs font-normal text-muted-foreground">{statusLabel(doc)}</span>
                        <Progress value={processingPercent(doc)} className="h-1" />
                      </span>
                    )}
                    {doc.status === 'FAILED' && <span className="block text-xs font-normal text-destructive">Processing failed</span>}
                  </span>
                </button>
                <ItemMenu label={`Options for ${doc.filename}`} onDelete={() => setToDelete({ kind: 'document', id: doc.id, name: doc.filename })} />
              </li>
            ))}
          </ul>
        </section>

        <section className="mt-5 px-3 pb-4" aria-labelledby="sidebar-chats">
          <h2 id="sidebar-chats" className="px-1 pb-1 text-sm font-semibold text-muted-foreground">
            Chats
          </h2>
          {conversations.data?.length === 0 && <p className="px-1 py-2 text-sm text-muted-foreground">Chats you start appear here.</p>}
          <ul className="space-y-0.5">
            {conversations.data?.map((c) => (
              <li key={c.id} className="group relative">
                <button
                  onClick={() => go(`/chat/${c.id}`)}
                  className={cn(
                    'block w-full rounded-md px-2 py-1.5 pr-8 text-left text-sm hover:bg-sidebar-accent',
                    chatMatch?.params.conversationId === c.id && 'bg-sidebar-accent font-semibold',
                  )}
                >
                  <span className="block truncate">{c.title}</span>
                  <span className="block text-xs font-normal text-muted-foreground">{timeAgo(c.updated_at)}</span>
                </button>
                <ItemMenu label={`Options for ${c.title}`} onDelete={() => setToDelete({ kind: 'chat', id: c.id, name: c.title })} />
              </li>
            ))}
          </ul>
        </section>
      </ScrollArea>

      <nav className="space-y-0.5 border-t px-3 py-2" aria-label="Account">
        <SidebarLink to="/history" icon={<ChartNoAxesColumn />} label="Study progress" onNavigate={onNavigate} />
        <SidebarLink to="/settings" icon={<Settings />} label="Settings" onNavigate={onNavigate} />
        <div className="flex items-center gap-2 px-2 pt-2">
          <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground" title={session?.user.email}>
            {session?.user.email}
          </span>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Sign out" onClick={() => signOut().catch((e: Error) => toast.error(e.message))}>
                <LogOut />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Sign out</TooltipContent>
          </Tooltip>
        </div>
      </nav>

      <UploadDialog open={uploadOpen} onOpenChange={setUploadOpen} onUploaded={(doc) => go(`/documents/${doc.id}`)} />
      <ConfirmDelete
        target={toDelete}
        onCancel={() => setToDelete(null)}
        onConfirm={() => {
          if (toDelete) remove.mutate(toDelete)
          setToDelete(null)
        }}
      />
    </div>
  )
}

function SidebarLink({ to, icon, label, onNavigate }: { to: string; icon: React.ReactNode; label: string; onNavigate?: () => void }) {
  return (
    <NavLink
      to={to}
      onClick={onNavigate}
      className={({ isActive }) =>
        cn('flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-sidebar-accent [&_svg]:size-4', isActive && 'bg-sidebar-accent font-semibold')
      }
    >
      {icon}
      {label}
    </NavLink>
  )
}

function ItemMenu({ label, onDelete }: { label: string; onDelete: () => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={label}
          className="absolute top-1.5 right-1 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 aria-expanded:opacity-100"
        >
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem variant="destructive" onSelect={onDelete}>
          <Trash2 />
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
