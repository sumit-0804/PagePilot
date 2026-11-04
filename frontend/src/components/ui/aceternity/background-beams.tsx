import { motion } from 'motion/react'
import { cn } from '@/lib/utils'

/**
 * Aceternity-inspired studio backdrop with full-bleed architectural grid and vibrant multi-spectrum aurora orbs in dark mode.
 */
export function BackgroundAmbient({
  className,
  variant = 'grid',
}: {
  className?: string
  variant?: 'grid' | 'dots'
}) {
  return (
    <div className={cn('pointer-events-none absolute inset-0 overflow-hidden', className)} aria-hidden>
      <div
        className={cn(
          'absolute inset-0 opacity-75 dark:opacity-50',
          variant === 'grid' ? 'bg-grid-pattern' : 'bg-dot-pattern',
        )}
        style={{
          maskImage: 'radial-gradient(ellipse 115% 100% at 50% 45%, black 55%, transparent 100%)',
          WebkitMaskImage: 'radial-gradient(ellipse 115% 100% at 50% 45%, black 55%, transparent 100%)',
        }}
      />
      {/* Primary Electric Blue / Active Accent Orb */}
      <motion.div
        animate={{
          x: [0, 40, -25, 0],
          y: [0, -30, 20, 0],
        }}
        transition={{ duration: 20, repeat: Infinity, ease: 'easeInOut' }}
        className="absolute -top-32 left-[8%] h-[28rem] w-[36rem] rounded-full bg-primary/10 blur-[120px] dark:bg-primary/14"
      />
      {/* Cyber Violet / Secondary Accent Orb */}
      <motion.div
        animate={{
          x: [0, -35, 25, 0],
          y: [0, 30, -20, 0],
        }}
        transition={{ duration: 24, repeat: Infinity, ease: 'easeInOut' }}
        className="absolute top-1/4 right-[6%] h-[26rem] w-[34rem] rounded-full bg-purple-500/8 blur-[120px] dark:bg-purple-500/12"
      />
      {/* Luminous Cyan / Bottom Ambient Orb */}
      <motion.div
        animate={{
          x: [0, 25, -20, 0],
          y: [0, -20, 25, 0],
        }}
        transition={{ duration: 26, repeat: Infinity, ease: 'easeInOut' }}
        className="absolute -bottom-32 left-1/3 h-[24rem] w-[38rem] rounded-full bg-cyan-500/8 blur-[130px] dark:bg-cyan-400/10"
      />
    </div>
  )
}
