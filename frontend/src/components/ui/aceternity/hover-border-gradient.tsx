import { useEffect, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { motion } from 'motion/react'
import { cn } from '@/lib/utils'

type Direction = 'TOP' | 'LEFT' | 'BOTTOM' | 'RIGHT'

const movingMap: Record<Direction, string> = {
  TOP: 'radial-gradient(25% 55% at 50% 0%, var(--primary) 0%, transparent 100%)',
  LEFT: 'radial-gradient(20% 50% at 0% 50%, var(--primary) 0%, transparent 100%)',
  BOTTOM: 'radial-gradient(25% 55% at 50% 100%, var(--terracotta) 0%, transparent 100%)',
  RIGHT: 'radial-gradient(20% 50% at 100% 50%, var(--terracotta) 0%, transparent 100%)',
}

const highlight =
  'radial-gradient(75% 180% at 50% 50%, var(--primary) 0%, var(--terracotta) 50%, transparent 100%)'

/**
 * Aceternity UI HoverBorderGradient: animated orbiting gradient border for interactive buttons and pills.
 */
export function HoverBorderGradient({
  children,
  containerClassName,
  className,
  duration = 1.2,
  clockwise = true,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode
  containerClassName?: string
  duration?: number
  clockwise?: boolean
}) {
  const [hovered, setHovered] = useState(false)
  const [direction, setDirection] = useState<Direction>('TOP')

  useEffect(() => {
    if (hovered) return
    const directions: Direction[] = ['TOP', 'LEFT', 'BOTTOM', 'RIGHT']
    const interval = setInterval(() => {
      setDirection((prev) => {
        const currentIndex = directions.indexOf(prev)
        const nextIndex = clockwise
          ? (currentIndex - 1 + directions.length) % directions.length
          : (currentIndex + 1) % directions.length
        return directions[nextIndex]
      })
    }, duration * 1000)
    return () => clearInterval(interval)
  }, [hovered, duration, clockwise])

  return (
    <button
      type="button"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className={cn(
        'relative flex content-center items-center justify-center overflow-visible rounded-full border border-border/60 bg-card/60 p-px transition duration-500 hover:bg-card/90 disabled:pointer-events-none disabled:opacity-50',
        containerClassName,
      )}
      {...props}
    >
      <div
        className={cn(
          'z-10 flex items-center gap-2 rounded-[inherit] bg-card px-4 py-2 text-sm font-medium text-foreground transition-colors',
          className,
        )}
      >
        {children}
      </div>
      <motion.div
        className="pointer-events-none absolute inset-0 z-0 overflow-hidden rounded-[inherit]"
        style={{ filter: 'blur(2px)' }}
        initial={{ background: movingMap[direction] }}
        animate={{
          background: hovered ? [movingMap[direction], highlight] : movingMap[direction],
        }}
        transition={{ ease: 'linear', duration: duration ?? 1 }}
        aria-hidden
      />
      <div className="pointer-events-none absolute inset-[1.5px] z-[1] rounded-[inherit] bg-card" aria-hidden />
    </button>
  )
}

