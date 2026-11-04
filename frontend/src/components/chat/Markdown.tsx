import { memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import { CitationChip, UnverifiedPage } from '@/components/chat/CitationChip'
import type { Citation } from '@/types'
import { cn } from '@/lib/utils'

// Same forms the backend accepts: [Page 3], 【Page 3】, [Pages 3, 5], [Pages 3 and 5], and a doubled [[Page 3]].
const CITATION = /\[?\s*[[【［]\s*pages?\s*(\d+(?:\s*(?:,|and|&)\s*\d+)*)\s*[\]】］]\s*\]?/gi

/** Turns page citations into markdown links that the renderer swaps for highlighter chips. */
function linkCitations(text: string) {
  return text.replace(CITATION, (_, nums: string) =>
    (nums.match(/\d+/g) ?? []).map((n) => `[p. ${n}](#page-${n})`).join(' '),
  )
}

interface MarkdownProps {
  text: string
  documentId: string
  citations: Citation[]
  /** While streaming, citations aren't verified yet, so every page renders as a chip. */
  streaming?: boolean
  className?: string
}

export const Markdown = memo(function Markdown({ text, documentId, citations, streaming = false, className }: MarkdownProps) {
  const byPage = new Map(citations.map((c) => [c.page_number, c]))
  return (
    <div className={cn('reading', className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex, rehypeHighlight]}
        components={{
          a: ({ href, children }) => {
            const page = href?.startsWith('#page-') ? Number(href.slice(6)) : null
            if (page !== null) {
              const citation = byPage.get(page)
              if (citation || streaming) return <CitationChip documentId={documentId} page={page} snippet={citation?.snippet} />
              return <UnverifiedPage page={page} />
            }
            return (
              <a href={href} target="_blank" rel="noreferrer">
                {children}
              </a>
            )
          },
        }}
      >
        {linkCitations(text)}
      </ReactMarkdown>
    </div>
  )
})
