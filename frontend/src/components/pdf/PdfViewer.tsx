import { useEffect, useRef, useState } from 'react'
import * as pdfjs from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { ChevronLeft, ChevronRight, FileText, Loader2, Minus, Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// Bundled locally so the viewer works offline and doesn't depend on a CDN.
pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

interface Target {
  page: number
  fromCitation: boolean
  nonce: number
}

const ZOOMS = [0.6, 0.8, 1, 1.25, 1.5, 2]

export function PdfViewer({
  url,
  urlError,
  title,
  target,
  onClose,
}: {
  url?: string
  urlError?: string
  title: string
  target: Target
  onClose: () => void
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const viewportRef = useRef<HTMLDivElement>(null)
  const [doc, setDoc] = useState<pdfjs.PDFDocumentProxy | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [page, setPage] = useState(target.page)
  const [pageInput, setPageInput] = useState(String(target.page))
  const [zoomIndex, setZoomIndex] = useState(2) // 1 = fit width
  const [width, setWidth] = useState(0)
  const [rendering, setRendering] = useState(true)
  const [seenNonce, setSeenNonce] = useState(target.nonce)

  // A citation click (new nonce) jumps to its page, even if it's the page already shown.
  if (target.nonce !== seenNonce) {
    setSeenNonce(target.nonce)
    setPage(target.page)
    setPageInput(String(target.page))
  }

  useEffect(() => {
    if (!url) return
    const run = { cancelled: false }
    const task = pdfjs.getDocument({ url })
    task.promise.then(
      (loaded) => !run.cancelled && setDoc(loaded),
      (err: Error) => !run.cancelled && setLoadError(`Couldn't open this PDF: ${err.message}`),
    )
    return () => {
      run.cancelled = true
      void task.destroy()
    }
  }, [url])

  useEffect(() => {
    const el = viewportRef.current
    if (!el) return
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const pageCount = doc?.numPages ?? 0
  const shownPage = pageCount ? Math.min(Math.max(page, 1), pageCount) : page

  useEffect(() => {
    if (!doc || !canvasRef.current || width === 0) return
    const run: { cancelled: boolean; task: pdfjs.RenderTask | null } = { cancelled: false, task: null }
    doc.getPage(shownPage).then((pdfPage) => {
      if (run.cancelled || !canvasRef.current) return
      setRendering(true)
      const base = pdfPage.getViewport({ scale: 1 })
      const scale = ((width - 32) / base.width) * ZOOMS[zoomIndex]
      const dpr = window.devicePixelRatio || 1
      const viewport = pdfPage.getViewport({ scale: scale * dpr })
      const canvas = canvasRef.current
      canvas.width = Math.floor(viewport.width)
      canvas.height = Math.floor(viewport.height)
      canvas.style.width = `${Math.floor(viewport.width / dpr)}px`
      canvas.style.height = `${Math.floor(viewport.height / dpr)}px`
      run.task = pdfPage.render({ canvas, viewport })
      run.task.promise.then(
        () => !run.cancelled && setRendering(false),
        (err: Error) => {
          if (err.name !== 'RenderingCancelledException' && !run.cancelled)
            setLoadError(`Couldn't draw page ${shownPage}: ${err.message}`)
        },
      )
    })
    return () => {
      run.cancelled = true
      run.task?.cancel()
    }
  }, [doc, shownPage, width, zoomIndex])

  const goTo = (n: number) => {
    const next = Math.min(Math.max(n, 1), pageCount || 1)
    setPage(next)
    setPageInput(String(next))
  }

  const error = urlError || loadError
  const isCitedPage = target.fromCitation && shownPage === target.page

  return (
    <section className="flex h-full flex-col bg-muted/60" aria-label={`PDF viewer: ${title}`}>
      <header className="glass-panel flex h-13 items-center gap-2 border-b border-border/80 px-3">
        <FileText className="size-4 shrink-0 text-primary" />
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold" title={title}>
          {title}
        </h2>
        <div className="flex items-center gap-0.5 rounded-lg border border-border/70 bg-background/70 p-0.5">
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Zoom out"
            disabled={zoomIndex === 0}
            onClick={() => setZoomIndex((z) => z - 1)}
          >
            <Minus />
          </Button>
          <span className="w-11 text-center text-xs font-medium tabular-nums text-muted-foreground">
            {Math.round(ZOOMS[zoomIndex] * 100)}%
          </span>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Zoom in"
            disabled={zoomIndex === ZOOMS.length - 1}
            onClick={() => setZoomIndex((z) => z + 1)}
          >
            <Plus />
          </Button>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label="Close PDF" onClick={onClose}>
          <X />
        </Button>
      </header>

      <nav
        className="flex items-center justify-center gap-2 border-b border-border/70 bg-card/85 px-3 py-1.5 text-sm backdrop-blur-xs"
        aria-label="Page navigation"
      >
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Previous page"
          disabled={shownPage <= 1}
          onClick={() => goTo(shownPage - 1)}
        >
          <ChevronLeft />
        </Button>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            const n = parseInt(pageInput, 10)
            if (!Number.isNaN(n)) goTo(n)
          }}
          className="flex items-center gap-1.5"
        >
          <label htmlFor="pdf-page" className="text-xs font-medium text-muted-foreground">
            Page
          </label>
          <input
            id="pdf-page"
            inputMode="numeric"
            value={pageInput}
            onChange={(e) => setPageInput(e.target.value)}
            onBlur={() => setPageInput(String(shownPage))}
            className={cn(
              'w-12 rounded-lg border px-1.5 py-0.5 text-center text-xs font-semibold tabular-nums transition-all',
              // The highlighter brings its own background; a bg-* utility here would override it.
              isCitedPage ? 'highlighter border-transparent font-bold' : 'bg-background',
            )}
          />
          <span className="text-xs text-muted-foreground">of {pageCount || '…'}</span>
        </form>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Next page"
          disabled={!pageCount || shownPage >= pageCount}
          onClick={() => goTo(shownPage + 1)}
        >
          <ChevronRight />
        </Button>
      </nav>

      <div ref={viewportRef} className="relative min-h-0 flex-1 overflow-auto p-4">
        {error ? (
          <p
            role="alert"
            className="mx-auto max-w-sm rounded-xl border border-destructive/30 bg-card p-4 text-sm text-destructive shadow-sm"
          >
            {error}
          </p>
        ) : (
          <>
            {(!doc || rendering) && (
              <div className="absolute inset-0 grid place-items-center" role="status">
                <Loader2 className="size-5 animate-spin text-primary" aria-label="Loading page" />
              </div>
            )}
            <canvas
              ref={canvasRef}
              className={cn(
                'mx-auto block rounded-lg bg-white shadow-md transition-shadow',
                isCitedPage && 'ring-4 ring-highlight shadow-lg',
              )}
            />
          </>
        )}
      </div>
    </section>
  )
}
