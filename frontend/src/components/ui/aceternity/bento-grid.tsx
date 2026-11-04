import type { ReactNode } from 'react'
import { motion } from 'motion/react'
import { CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { cn } from '@/lib/utils'

export function BentoGrid({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <div className={cn('grid grid-cols-1 gap-4 md:grid-cols-12', className)}>
      {children}
    </div>
  )
}

export function BentoCard({
  className,
  title,
  description,
  header,
  icon,
  meta,
  children,
}: {
  className?: string
  title?: ReactNode
  description?: ReactNode
  header?: ReactNode
  icon?: ReactNode
  meta?: ReactNode
  children?: ReactNode
}) {
  return (
    <motion.div
      whileHover={{ y: -2 }}
      transition={{ type: 'spring', stiffness: 360, damping: 26 }}
      className={cn('row-span-1 flex flex-col', className)}
    >
      <CardSpotlight className="flex h-full flex-col justify-between p-5">
        {header}
        <div className="transition duration-200 group-hover/spotlight:translate-x-0.5">
          {(icon || meta) && (
            <div className="mb-2.5 flex items-center justify-between gap-2">
              {icon && (
                <span className="inline-flex size-8 items-center justify-center rounded-lg bg-accent text-accent-foreground">
                  {icon}
                </span>
              )}
              {meta && <span className="text-xs font-medium text-muted-foreground">{meta}</span>}
            </div>
          )}
          {title && <div className="font-sans font-semibold text-foreground">{title}</div>}
          {description && <div className="mt-1 text-sm text-muted-foreground">{description}</div>}
          {children}
        </div>
      </CardSpotlight>
    </motion.div>
  )
}
