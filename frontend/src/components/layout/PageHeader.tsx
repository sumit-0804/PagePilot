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
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3">
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
      <div className="min-w-0 flex-1 truncate text-sm font-semibold">{title}</div>
      {actions}
    </header>
  )
}
