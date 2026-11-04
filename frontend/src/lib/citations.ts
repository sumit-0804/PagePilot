import type { Citation } from '@/types'

/** Only http(s) links are opened, so a model-supplied URL can't run script. */
export function safeUrl(url: string | undefined): string | null {
  if (!url) return null
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : null
  } catch {
    return null
  }
}

export const hostname = (url: string) => new URL(url).hostname.replace(/^www\./, '')

/** A source's display name: the page title or host for web, e.g. "Notes · Slide 4" for documents. */
export function sourceName(c: Citation): string {
  if (c.kind === 'web') {
    const url = safeUrl(c.url)
    return c.title || (url ? hostname(url) : 'Web page')
  }
  // The API sends the unit ("Slide") and its number (page_number) separately.
  const where = c.page_number ? `${c.label || 'Page'} ${c.page_number}` : c.label || ''
  return [c.document_name, where].filter(Boolean).join(' · ') || 'Document'
}

/** The chip text: the source number, or "p. N" for older page-tagged citations. */
export const citationLabel = (c: Citation) => (c.n !== undefined ? String(c.n) : `p. ${c.page_number ?? '?'}`)
