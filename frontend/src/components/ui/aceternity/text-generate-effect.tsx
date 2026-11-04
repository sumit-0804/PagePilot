import { motion } from 'motion/react'
import { cn } from '@/lib/utils'

/**
 * Aceternity UI TextGenerateEffect: animates words in sequentially with blur-to-crisp transition.
 */
export function TextGenerateEffect({
  words,
  className,
  duration = 0.45,
}: {
  words: string
  className?: string
  duration?: number
}) {
  const wordsArray = words.split(' ')

  return (
    <span className={cn('inline', className)}>
      {wordsArray.map((word, idx) => (
        <motion.span
          key={`${word}-${idx}`}
          initial={{ opacity: 0, filter: 'blur(8px)', y: 6 }}
          animate={{ opacity: 1, filter: 'blur(0px)', y: 0 }}
          transition={{
            duration,
            delay: idx * 0.055,
            ease: [0.22, 1, 0.36, 1],
          }}
          className="inline-block"
        >
          {word}
          {idx < wordsArray.length - 1 ? '\u00A0' : ''}
        </motion.span>
      ))}
    </span>
  )
}

