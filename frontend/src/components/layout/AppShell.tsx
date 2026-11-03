import { lazy, Suspense } from 'react'
import { Outlet } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { useStudyStore } from '@/stores/useStudyStore'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { Sidebar } from '@/components/layout/Sidebar'

// pdf.js is large; load it the first time a PDF is opened.
const PdfPanel = lazy(() => import('@/components/pdf/PdfPanel').then((m) => ({ default: m.PdfPanel })))

function LazyPdfPanel() {
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center bg-muted" role="status">
          <Loader2 className="size-5 animate-spin text-muted-foreground" aria-label="Loading PDF viewer" />
        </div>
      }
    >
      <PdfPanel />
    </Suspense>
  )
}

export function AppShell() {
  const sidebarOpen = useStudyStore((s) => s.sidebarOpen)
  const setSidebarOpen = useStudyStore((s) => s.setSidebarOpen)
  const mobileNavOpen = useStudyStore((s) => s.mobileNavOpen)
  const setMobileNavOpen = useStudyStore((s) => s.setMobileNavOpen)
  const pdf = useStudyStore((s) => s.pdf)
  const wide = useMediaQuery('(min-width: 1024px)')
  const tablet = useMediaQuery('(min-width: 768px)')

  const main = (
    <main className="h-full min-w-0 flex-1 overflow-hidden">
      <Outlet />
    </main>
  )

  return (
    <div className="flex h-dvh overflow-hidden bg-background">
      {sidebarOpen && tablet && (
        <aside className="w-72 shrink-0 border-r bg-sidebar">
          <Sidebar onHide={() => setSidebarOpen(false)} />
        </aside>
      )}
      {mobileNavOpen && !tablet && (
        <div className="fixed inset-0 z-40 flex">
          <aside className="w-[min(20rem,85vw)] border-r bg-sidebar shadow-xl">
            <Sidebar onHide={() => setMobileNavOpen(false)} onNavigate={() => setMobileNavOpen(false)} />
          </aside>
          <button aria-label="Close menu" className="flex-1 bg-foreground/30" onClick={() => setMobileNavOpen(false)} />
        </div>
      )}

      {pdf && wide ? (
        <ResizablePanelGroup orientation="horizontal" className="min-w-0 flex-1">
          <ResizablePanel defaultSize="56%" minSize="35%">
            {main}
          </ResizablePanel>
          <ResizableHandle withHandle aria-label="Resize the PDF panel" />
          <ResizablePanel defaultSize="44%" minSize="25%">
            <LazyPdfPanel />
          </ResizablePanel>
        </ResizablePanelGroup>
      ) : (
        main
      )}

      {pdf && !wide && (
        <div className="fixed inset-0 z-50 bg-background">
          <LazyPdfPanel />
        </div>
      )}
    </div>
  )
}
