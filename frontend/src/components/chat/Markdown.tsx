import { memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import { CitationChip, PendingCitation, UnverifiedCitation } from '@/components/chat/CitationChip'
import type { Citation } from '@/types'
import { cn } from '@/lib/utils'

// [1], [1, 3] and [1][2]; not markdown links like [1](url).
const NUMBERED = /\[(\d{1,3}(?:\s*,\s*\d{1,3})*)\](?!\()/g
// Summaries and older messages: [Page 3], 【Page 3】, [Pages 3, 5], [Pages 3 and 5], and a doubled [[Page 3]].
const PAGES = /\[?\s*[[【［]\s*pages?\s*(\d+(?:\s*(?:,|and|&)\s*\d+)*)\s*[\]】］]\s*\]?/gi
// Code fences and inline code, which must keep their brackets (e.g. arr[0]).
const CODE = /(```[\s\S]*?(?:```|$)|`[^`\n]*`)/

const links = (nums: string, prefix: string, label: string) =>
  (nums.match(/\d+/g) ?? []).map((n) => `[${label}${n}](#${prefix}-${n})`).join(' ')

/** Turns citation markers outside code into markdown links that the renderer swaps for source chips. */
function linkCitations(text: string) {
  return text
    .split(CODE)
    .map((part, i) =>
      i % 2 === 1
        ? part
        : part
            .replace(PAGES, (_, nums: string) => links(nums, 'page', 'p. '))
            .replace(NUMBERED, (_, nums: string) => links(nums, 'cite', '')),
    )
    .join('')
}

interface MarkdownProps {
  text: string
  citations: Citation[]
  /** While streaming, sources aren't known yet, so every marker renders as a plain chip. */
  streaming?: boolean
  className?: string
}

export const Markdown = memo(function Markdown({ text, citations, streaming = false, className }: MarkdownProps) {
  const byNumber = new Map(citations.filter((c) => c.n !== undefined).map((c) => [c.n, c]))
  const byPage = new Map(citations.filter((c) => c.n === undefined).map((c) => [c.page_number, c]))
  return (
    <div className={cn('reading', className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex, rehypeHighlight]}
        components={{
          a: ({ href, children }) => {
            const marker = href?.match(/^#(cite|page)-(\d+)$/)
            if (marker) {
              const num = Number(marker[2])
              const isPage = marker[1] === 'page'
              const citation = isPage ? byPage.get(num) : byNumber.get(num)
              if (citation) return <CitationChip citation={citation} />
              const label = isPage ? `p. ${num}` : String(num)
              return streaming ? <PendingCitation label={label} /> : <UnverifiedCitation label={label} />
            }
            return (
              <a href={href} target="_blank" rel="noopener noreferrer">
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
