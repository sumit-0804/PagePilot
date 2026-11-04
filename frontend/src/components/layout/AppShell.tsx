import { lazy, Suspense } from 'react'
import { Outlet } from 'react-router-dom'
import { AnimatePresence, motion } from 'motion/react'
import { Loader2 } from 'lucide-react'
import { useStudyStore, type ViewerTarget } from '@/stores/useStudyStore'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { useDocuments } from '@/hooks/queries'
import { TextPanel } from '@/components/documents/TextPanel'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { Sidebar } from '@/components/layout/Sidebar'

// pdf.js is large; load it the first time a PDF is opened.
const PdfPanel = lazy(() => import('@/components/pdf/PdfPanel').then((m) => ({ default: m.PdfPanel })))

/** PDFs open in the page viewer; every other format opens in the text viewer. */
function ViewerPanel({ target }: { target: ViewerTarget }) {
  const documents = useDocuments()
  const format = target.format ?? documents.data?.find((d) => d.id === target.documentId)?.format
  if (!format && documents.isLoading) return null
  if (format && format !== 'pdf') return <TextPanel target={target} />
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center bg-muted/60" role="status">
          <div className="flex flex-col items-center gap-2">
            <Loader2 className="size-5 animate-spin text-primary" aria-label="Loading PDF viewer" />
            <span className="text-xs text-muted-foreground">Opening PDF viewer…</span>
          </div>
        </div>
      }
    >
      <PdfPanel target={target} />
    </Suspense>
  )
}

export function AppShell() {
  const sidebarOpen = useStudyStore((s) => s.sidebarOpen)
  const setSidebarOpen = useStudyStore((s) => s.setSidebarOpen)
  const mobileNavOpen = useStudyStore((s) => s.mobileNavOpen)
  const setMobileNavOpen = useStudyStore((s) => s.setMobileNavOpen)
  const viewer = useStudyStore((s) => s.viewer)
  const wide = useMediaQuery('(min-width: 1024px)')
  const tablet = useMediaQuery('(min-width: 768px)')

  const main = (
    <main className="relative h-full min-w-0 flex-1 overflow-hidden bg-background">
      <Outlet />
    </main>
  )

  return (
    <div className="flex h-dvh overflow-hidden bg-background">
      <AnimatePresence initial={false}>
        {sidebarOpen && tablet && (
          <motion.aside
            key="desktop-sidebar"
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: 288, opacity: 1 }}
            exit={{ width: 0, opacity: 0 }}
            transition={{ type: 'spring', stiffness: 380, damping: 34 }}
            className="shrink-0 overflow-hidden border-r border-sidebar-border bg-sidebar"
          >
            <div className="h-full w-72">
              <Sidebar onHide={() => setSidebarOpen(false)} />
            </div>
          </motion.aside>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {mobileNavOpen && !tablet && (
          <div className="fixed inset-0 z-40 flex">
            <motion.aside
              initial={{ x: '-100%' }}
              animate={{ x: 0 }}
              exit={{ x: '-100%' }}
              transition={{ type: 'spring', stiffness: 360, damping: 32 }}
              className="relative z-10 w-[min(20rem,85vw)] border-r border-sidebar-border bg-sidebar shadow-2xl"
            >
              <Sidebar onHide={() => setMobileNavOpen(false)} onNavigate={() => setMobileNavOpen(false)} />
            </motion.aside>
            <motion.button
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              aria-label="Close menu"
              className="flex-1 bg-foreground/35 backdrop-blur-xs"
              onClick={() => setMobileNavOpen(false)}
            />
          </div>
        )}
      </AnimatePresence>

      {viewer && wide ? (
        <ResizablePanelGroup orientation="horizontal" className="min-w-0 flex-1">
          <ResizablePanel defaultSize="56%" minSize="35%">
            {main}
          </ResizablePanel>
          <ResizableHandle withHandle aria-label="Resize the document panel" className="transition-colors hover:bg-primary/40" />
          <ResizablePanel defaultSize="44%" minSize="25%">
            <ViewerPanel target={viewer} />
          </ResizablePanel>
        </ResizablePanelGroup>
      ) : (
        main
      )}

      <AnimatePresence>
        {viewer && !wide && (
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 16 }}
            transition={{ duration: 0.2 }}
            className="fixed inset-0 z-50 bg-background"
          >
            <ViewerPanel target={viewer} />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
