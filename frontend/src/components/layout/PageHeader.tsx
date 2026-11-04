import type { ReactNode } from 'react'
import { Menu, PanelLeftOpen } from 'lucide-react'
import { useStudyStore } from '@/stores/useStudyStore'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { Button } from '@/components/ui/button'

/** Top bar of the main column; offers the menu on phones and the sidebar toggle when it's hidden. */
export function PageHeader({ title, actions }: { title?: ReactNode; actions?: ReactNode }) {
  const tablet = useMediaQuery('(min-width: 768px)')
  const sidebarOpen = useStudyStore((s) => s.sidebarOpen)
  const setSidebarOpen = useStudyStore((s) => s.setSidebarOpen)
  const setMobileNavOpen = useStudyStore((s) => s.setMobileNavOpen)
  return (
    <header className="glass-panel sticky top-0 z-20 flex h-13 shrink-0 items-center gap-2.5 border-b border-border/70 px-4">
      {!tablet ? (
        <Button variant="ghost" size="icon-sm" aria-label="Open menu" onClick={() => setMobileNavOpen(true)}>
          <Menu />
        </Button>
      ) : (
        !sidebarOpen && (
          <Button variant="ghost" size="icon-sm" aria-label="Show sidebar" onClick={() => setSidebarOpen(true)}>
            <PanelLeftOpen />
          </Button>
        )
      )}
      <div className="min-w-0 flex-1 truncate text-sm font-semibold tracking-tight text-foreground/90">{title}</div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  )
}
