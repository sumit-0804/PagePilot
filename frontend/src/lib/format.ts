import type { DocumentStatus, ExplanationStyle, StudyDocument } from '@/types'

const PROCESSING: DocumentStatus[] = ['UPLOADED', 'EXTRACTING', 'CHUNKING', 'EMBEDDING', 'INDEXING']

export const isProcessing = (status: DocumentStatus) => PROCESSING.includes(status)

/** Plain-language processing status for a document. */
export function statusLabel(doc: Pick<StudyDocument, 'status' | 'chunks_total' | 'chunks_embedded'>): string {
  switch (doc.status) {
    case 'UPLOADED':
      return 'Waiting to start'
    case 'EXTRACTING':
      return 'Reading pages'
    case 'CHUNKING':
      return 'Splitting into passages'
    case 'EMBEDDING':
      return doc.chunks_total ? `Indexing passages ${doc.chunks_embedded} of ${doc.chunks_total}` : 'Indexing passages'
    case 'INDEXING':
      return 'Saving'
    case 'READY':
      return 'Ready'
    case 'FAILED':
      return 'Failed'
  }
}

export function processingPercent(doc: Pick<StudyDocument, 'status' | 'chunks_total' | 'chunks_embedded'>): number {
  switch (doc.status) {
    case 'UPLOADED':
      return 3
    case 'EXTRACTING':
      return 10
    case 'CHUNKING':
      return 20
    case 'EMBEDDING':
      return doc.chunks_total ? 20 + Math.round((70 * doc.chunks_embedded) / doc.chunks_total) : 25
    case 'INDEXING':
      return 95
    default:
      return 100
  }
}

export function displayName(filename: string) {
  return filename.replace(/\.pdf$/i, '')
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function timeAgo(iso: string): string {
  const seconds = (new Date(iso).getTime() - Date.now()) / 1000
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31536000],
    ['month', 2592000],
    ['week', 604800],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ]
  for (const [unit, size] of steps) {
    if (Math.abs(seconds) >= size) return rtf.format(Math.round(seconds / size), unit)
  }
  return 'just now'
}

export const STYLE_OPTIONS: { value: ExplanationStyle; label: string; hint: string }[] = [
  { value: 'simple', label: 'Simple', hint: 'Short sentences, no jargon' },
  { value: 'beginner', label: 'Beginner', hint: 'Builds up from the basics' },
  { value: 'step_by_step', label: 'Step by step', hint: 'One idea per step' },
  { value: 'detailed', label: 'Detailed', hint: 'Covers the nuances' },
  { value: 'technical', label: 'Technical', hint: 'Precise, textbook language' },
  { value: 'analogy', label: 'Analogy', hint: 'Through a real-world comparison' },
  { value: 'example', label: 'Examples', hint: 'Through worked examples' },
  { value: 'exam', label: 'Exam focus', hint: 'Key points and likely questions' },
]

export const styleLabel = (style: ExplanationStyle | '') => STYLE_OPTIONS.find((s) => s.value === style)?.label ?? 'Default'
