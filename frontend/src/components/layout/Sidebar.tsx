import { useState } from 'react'
import { Link, NavLink, useMatch, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { toast } from 'sonner'
import {
  AlertCircle,
  BookOpenCheck,
  ChartNoAxesColumn,
  LogIn,
  LogOut,
  MessageSquare,
  MoreHorizontal,
  PanelLeftClose,
  Plus,
  Settings,
  Sparkles,
  SquarePen,
  Trash2,
  UserPlus,
} from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys, useConversations, useDocuments } from '@/hooks/queries'
import { useAuth } from '@/hooks/useAuth'
import { useStudyStore } from '@/stores/useStudyStore'
import { displayName, isProcessing, processingPercent, sectionNoun, statusLabel, timeAgo } from '@/lib/format'
import { FormatIcon } from '@/components/documents/FormatIcon'
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
  const { session, isAccount, signOut } = useAuth()
  const closeViewer = useStudyStore((s) => s.closeViewer)
  const documents = useDocuments()
  const conversations = useConversations()
  const docMatch = useMatch('/documents/:documentId')
  const chatMatch = useMatch('/chat/:conversationId')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [toDelete, setToDelete] = useState<{ kind: 'document' | 'chat'; id: string; name: string } | null>(null)

  const activeDocId = docMatch?.params.documentId

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
        closeViewer()
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

  const userInitial = session?.user.email?.[0]?.toUpperCase() ?? 'P'

  return (
    <div className="flex h-full flex-col bg-sidebar text-sidebar-foreground">
      {/* Brand header */}
      <div className="flex items-center justify-between px-4 pt-4 pb-3">
        <Link to="/" onClick={onNavigate} className="group flex items-center gap-2.5">
          <span className="flex size-8 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-primary/80 text-primary-foreground shadow-sm shadow-primary/20 transition-transform duration-200 group-hover:scale-105">
            <BookOpenCheck className="size-4" />
          </span>
          <span className="font-reading text-xl font-semibold tracking-tight">PagePilot</span>
        </Link>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Hide sidebar" onClick={onHide} className="text-muted-foreground hover:text-foreground">
              <PanelLeftClose />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Hide sidebar</TooltipContent>
        </Tooltip>
      </div>

      {/* Primary New Chat CTA */}
      <div className="px-3 pt-1">
        <motion.div whileHover={{ scale: 1.01 }} whileTap={{ scale: 0.98 }}>
          <Button
            className="w-full justify-between rounded-xl bg-primary text-primary-foreground shadow-sm shadow-primary/15 hover:bg-primary/90"
            size="lg"
            onClick={() => go('/')}
          >
            <span className="flex items-center gap-2">
              <SquarePen className="size-4" />
              New chat
            </span>
            <Sparkles className="size-3.5 opacity-75" />
          </Button>
        </motion.div>
      </div>

      {!isAccount ? (
        <div className="mt-6 flex-1 px-3">
          <div className="rounded-2xl border border-sidebar-border/80 bg-card/60 p-4">
            <p className="text-sm font-semibold">Save your chats</p>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              Log in to keep chatting, see your history and upload documents to study.
            </p>
            <div className="mt-3 flex flex-col gap-2">
              <Button asChild size="sm" className="rounded-xl">
                <Link to="/signup" onClick={onNavigate}>
                  <UserPlus />
                  Create account
                </Link>
              </Button>
              <Button asChild size="sm" variant="outline" className="rounded-xl">
                <Link to="/login" onClick={onNavigate}>
                  <LogIn />
                  Log in
                </Link>
              </Button>
            </div>
          </div>
        </div>
      ) : (
      <>
      <ScrollArea className="mt-4 min-h-0 flex-1">
        {/* Documents section */}
        <section className="px-3" aria-labelledby="sidebar-docs">
          <div className="flex items-center justify-between px-2 pb-1.5">
            <h2 id="sidebar-docs" className="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
              Documents
            </h2>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label="Upload a document"
                  onClick={() => setUploadOpen(true)}
                  className="rounded-lg text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
                >
                  <Plus />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Upload a document</TooltipContent>
            </Tooltip>
          </div>
          {documents.isLoading && <p className="px-2 py-2 text-sm text-muted-foreground">Loading…</p>}
          {documents.error && <p className="px-2 py-2 text-sm text-destructive">{documents.error.message}</p>}
          {documents.data?.length === 0 && (
            <p className="px-2 py-2 text-xs text-muted-foreground">No documents uploaded yet.</p>
          )}
          <ul className="space-y-1">
            {documents.data?.map((doc) => {
              const isActive = activeDocId === doc.id
              return (
                <li key={doc.id} className="group relative">
                  <button
                    onClick={() => go(`/documents/${doc.id}`)}
                    className={cn(
                      'relative flex w-full items-start gap-2.5 rounded-xl px-2.5 py-2 pr-8 text-left text-sm transition-colors hover:bg-sidebar-accent/70',
                      isActive && 'bg-sidebar-accent font-semibold text-sidebar-accent-foreground',
                    )}
                  >
                    {isActive && (
                      <motion.span
                        layoutId="sidebar-doc-active"
                        className="absolute top-2 bottom-2 left-0 w-1 rounded-r-full bg-primary"
                        transition={{ type: 'spring', stiffness: 380, damping: 30 }}
                      />
                    )}
                    {doc.status === 'FAILED' ? (
                      <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
                    ) : (
                      <FormatIcon
                        format={doc.format}
                        className={cn('mt-0.5 size-4 shrink-0 text-muted-foreground transition-colors', isActive && 'text-primary')}
                      />
                    )}
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{displayName(doc.filename)}</span>
                      {isProcessing(doc.status) && (
                        <span className="mt-1.5 block space-y-1">
                          <span className="block text-xs font-normal text-primary">{statusLabel(doc)}</span>
                          <Progress value={processingPercent(doc)} className="h-1" />
                        </span>
                      )}
                      {doc.status === 'READY' && doc.page_count > 0 && (
                        <span className="block text-xs font-normal text-muted-foreground">
                          {doc.page_count} {sectionNoun(doc.format, doc.page_count)}
                        </span>
                      )}
                      {doc.status === 'FAILED' && (
                        <span className="block text-xs font-normal text-destructive">Processing failed</span>
                      )}
                    </span>
                  </button>
                  <ItemMenu
                    label={`Options for ${doc.filename}`}
                    onDelete={() => setToDelete({ kind: 'document', id: doc.id, name: doc.filename })}
                  />
                </li>
              )
            })}
          </ul>
        </section>

        {/* Chats section */}
        <section className="mt-6 px-3 pb-4" aria-labelledby="sidebar-chats">
          <h2 id="sidebar-chats" className="px-2 pb-1.5 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
            Recent Chats
          </h2>
          {conversations.data?.length === 0 && (
            <p className="px-2 py-2 text-xs text-muted-foreground">Chats you start appear here.</p>
          )}
          <ul className="space-y-1">
            {conversations.data?.map((c) => {
              const isChatActive = chatMatch?.params.conversationId === c.id
              return (
                <li key={c.id} className="group relative">
                  <button
                    onClick={() => go(`/chat/${c.id}`)}
                    className={cn(
                      'relative flex w-full items-start gap-2.5 rounded-xl px-2.5 py-2 pr-8 text-left text-sm transition-colors hover:bg-sidebar-accent/70',
                      isChatActive && 'bg-sidebar-accent font-semibold text-sidebar-accent-foreground',
                    )}
                  >
                    <MessageSquare className={cn('mt-0.5 size-3.5 shrink-0 text-muted-foreground', isChatActive && 'text-primary')} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{c.title}</span>
                      <span className="block text-xs font-normal text-muted-foreground">{timeAgo(c.updated_at)}</span>
                    </span>
                  </button>
                  <ItemMenu label={`Options for ${c.title}`} onDelete={() => setToDelete({ kind: 'chat', id: c.id, name: c.title })} />
                </li>
              )
            })}
          </ul>
        </section>
      </ScrollArea>

      {/* Bottom account & navigation bar */}
      <nav className="space-y-1 border-t border-sidebar-border bg-sidebar/80 p-3" aria-label="Account">
        <SidebarLink to="/history" icon={<ChartNoAxesColumn />} label="Study progress" onNavigate={onNavigate} />
        <SidebarLink to="/settings" icon={<Settings />} label="Settings" onNavigate={onNavigate} />
        <div className="mt-2 flex items-center gap-2.5 rounded-xl border border-sidebar-border/80 bg-card/50 px-2.5 py-2">
          <span
            className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent text-xs font-bold text-accent-foreground"
            aria-hidden
          >
            {userInitial}
          </span>
          <span className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground" title={session?.user.email}>
            {session?.user.email}
          </span>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="Sign out"
                onClick={() => signOut().catch((e: Error) => toast.error(e.message))}
                className="text-muted-foreground hover:text-destructive"
              >
                <LogOut />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Sign out</TooltipContent>
          </Tooltip>
        </div>
      </nav>
      </>
      )}

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
        cn(
          'flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm transition-colors hover:bg-sidebar-accent/70 [&_svg]:size-4 [&_svg]:text-muted-foreground',
          isActive && 'bg-sidebar-accent font-semibold text-sidebar-accent-foreground [&_svg]:text-primary',
        )
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
          className="absolute top-2 right-1.5 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 aria-expanded:opacity-100"
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
