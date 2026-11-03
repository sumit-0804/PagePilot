import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { ArrowUp, Check, FileText, Paperclip, SlidersHorizontal, Square } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useStudyStore } from '@/stores/useStudyStore'
import { STYLE_OPTIONS, styleLabel } from '@/lib/format'
import type { ExplanationStyle } from '@/types'

interface ComposerProps {
  documentName: string
  onSend: (text: string, style: ExplanationStyle | '') => void
  onStop?: () => void
  onAttach: () => void
  busy: boolean
  disabled?: boolean
  autoFocus?: boolean
}

export function Composer({ documentName, onSend, onStop, onAttach, busy, disabled = false, autoFocus = false }: ComposerProps) {
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const [text, setText] = useState('')
  const [style, setStyle] = useState<ExplanationStyle | ''>(defaultStyle)
  const ref = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 220)}px`
  }, [text])

  const canSend = text.trim().length > 0 && !busy && !disabled

  function send() {
    if (!canSend) return
    onSend(text.trim(), style)
    setText('')
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter adds a line. Ignore Enter while an IME is composing.
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      send()
    }
  }

  return (
    <div className="rounded-xl border bg-card shadow-sm focus-within:border-ring">
      <label htmlFor="composer" className="sr-only">
        Ask about {documentName}
      </label>
      <textarea
        id="composer"
        ref={ref}
        rows={1}
        autoFocus={autoFocus}
        value={text}
        disabled={disabled}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={`Ask about ${documentName}…`}
        className="block max-h-56 w-full resize-none bg-transparent px-4 pt-3.5 pb-2 text-base outline-none placeholder:text-muted-foreground disabled:opacity-60"
      />
      <div className="flex items-center gap-1 px-2 pb-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Upload another PDF" onClick={onAttach}>
              <Paperclip />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Upload another PDF</TooltipContent>
        </Tooltip>

        <span className="flex min-w-0 items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground" title={documentName}>
          <FileText className="size-3.5 shrink-0" />
          <span className="truncate">{documentName}</span>
        </span>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="ml-auto text-muted-foreground" aria-label={`Explanation style: ${styleLabel(style)}`}>
              <SlidersHorizontal />
              {styleLabel(style)}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60">
            <DropdownMenuLabel>How should answers explain?</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setStyle('')}>
              <span className="flex-1">Default</span>
              {style === '' && <Check />}
            </DropdownMenuItem>
            {STYLE_OPTIONS.map((opt) => (
              <DropdownMenuItem key={opt.value} onSelect={() => setStyle(opt.value)}>
                <span className="flex-1">
                  <span className="block">{opt.label}</span>
                  <span className="block text-xs text-muted-foreground">{opt.hint}</span>
                </span>
                {style === opt.value && <Check />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {busy && onStop ? (
          <Button size="icon" variant="secondary" aria-label="Stop generating" onClick={onStop}>
            <Square className="fill-current" />
          </Button>
        ) : (
          <Button size="icon" aria-label="Send" disabled={!canSend} onClick={send}>
            <ArrowUp />
          </Button>
        )}
      </div>
    </div>
  )
}
