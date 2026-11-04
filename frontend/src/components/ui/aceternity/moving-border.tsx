import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/**
 * Subtle Aceternity-style glowing border container that highlights interactive surfaces.
 */
export function GlowContainer({
  children,
  className,
  active = false,
}: {
  children: ReactNode
  className?: string
  active?: boolean
}) {
  return (
    <div className={cn('group relative rounded-2xl p-[1px] transition-all duration-300', className)}>
      <div
        className={cn(
          'pointer-events-none absolute -inset-[1px] rounded-2xl bg-gradient-to-r from-primary/35 via-terracotta/25 to-primary/35 opacity-0 blur-[2px] transition-opacity duration-500 group-focus-within:opacity-100 group-hover:opacity-65',
          active && 'opacity-90 animate-pulse',
        )}
        aria-hidden
      />
      <div className="relative rounded-2xl bg-card">{children}</div>
    </div>
  )
}
