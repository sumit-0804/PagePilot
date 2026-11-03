import React, { useEffect, useRef, useState } from 'react'
import * as pdfjsLib from 'pdfjs-dist'
import {
  ChevronLeft,
  ChevronRight,
  ZoomIn,
  ZoomOut,
  RotateCw,
  X,
  FileText,
  Loader2,
} from 'lucide-react'

// Set worker source
pdfjsLib.GlobalWorkerOptions.workerSrc = `https://cdnjs.cloudflare.com/ajax/libs/pdf.js/${pdfjsLib.version}/pdf.worker.min.mjs`

interface PDFViewerProps {
  url: string
  title: string
  targetPage?: number
  onClose: () => void
}

export const PDFViewer: React.FC<PDFViewerProps> = ({
  url,
  title,
  targetPage,
  onClose,
}) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const [pdfDoc, setPdfDoc] = useState<pdfjsLib.PDFDocumentProxy | null>(null)
  const [pageNum, setPageNum] = useState<number>(1)
  const [totalPages, setTotalPages] = useState<number>(0)
  const [scale, setScale] = useState<number>(1.2)
  const [loading, setLoading] = useState<boolean>(true)
  const [error, setError] = useState<string | null>(null)
  const renderTaskRef = useRef<any>(null)

  // Load PDF Document
  useEffect(() => {
    let isCancelled = false
    setLoading(true)
    setError(null)

    const loadingTask = pdfjsLib.getDocument(url)
    loadingTask.promise
      .then((doc) => {
        if (isCancelled) return
        setPdfDoc(doc)
        setTotalPages(doc.numPages)
        setPageNum(targetPage && targetPage <= doc.numPages ? targetPage : 1)
        setLoading(false)
      })
      .catch((err) => {
        if (isCancelled) return
        console.error('PDF load error:', err)
        setError('Failed to load PDF document. Please verify the file is available.')
        setLoading(false)
      })

    return () => {
      isCancelled = true
    }
  }, [url])

  // Jump to page if targetPage changes
  useEffect(() => {
    if (targetPage && targetPage >= 1 && totalPages > 0 && targetPage <= totalPages) {
      setPageNum(targetPage)
    }
  }, [targetPage, totalPages])

  // Render current page onto canvas
  useEffect(() => {
    if (!pdfDoc || !canvasRef.current) return

    if (renderTaskRef.current) {
      renderTaskRef.current.cancel()
    }

    pdfDoc.getPage(pageNum).then((page) => {
      const canvas = canvasRef.current
      if (!canvas) return
      const context = canvas.getContext('2d')
      if (!context) return

      const viewport = page.getViewport({ scale })
      canvas.height = viewport.height
      canvas.width = viewport.width

      const renderContext = {
        canvasContext: context,
        viewport: viewport,
      }

      const task = page.render(renderContext)
      renderTaskRef.current = task

      task.promise.catch((err) => {
        if (err?.name !== 'RenderingCancelledException') {
          console.error('Render page error:', err)
        }
      })
    })
  }, [pdfDoc, pageNum, scale])

  const changePage = (offset: number) => {
    setPageNum((prev) => {
      const next = prev + offset
      if (next >= 1 && next <= totalPages) return next
      return prev
    })
  }

  return (
    <div className="flex flex-col h-full bg-slate-900 text-slate-100 border-l border-slate-750 select-none shadow-2xl">
      {/* Viewer Header */}
      <div className="flex items-center justify-between px-3 py-2.5 bg-slate-850 border-b border-slate-700 text-xs">
        <div className="flex items-center space-x-2 truncate max-w-[200px]">
          <FileText className="w-4 h-4 text-indigo-400 shrink-0" />
          <span className="font-medium truncate text-slate-200" title={title}>
            {title}
          </span>
        </div>

        {/* Toolbar Controls */}
        <div className="flex items-center space-x-1">
          {/* Zoom Out */}
          <button
            onClick={() => setScale((s) => Math.max(0.6, s - 0.2))}
            title="Zoom out"
            className="p-1 hover:bg-slate-700 rounded transition text-slate-300"
          >
            <ZoomOut className="w-3.5 h-3.5" />
          </button>
          <span className="text-[11px] font-mono text-slate-400 px-1">
            {Math.round(scale * 100)}%
          </span>
          {/* Zoom In */}
          <button
            onClick={() => setScale((s) => Math.min(2.5, s + 0.2))}
            title="Zoom in"
            className="p-1 hover:bg-slate-700 rounded transition text-slate-300"
          >
            <ZoomIn className="w-3.5 h-3.5" />
          </button>
          {/* Reset Scale */}
          <button
            onClick={() => setScale(1.2)}
            title="Reset zoom"
            className="p-1 hover:bg-slate-700 rounded transition text-slate-300 ml-1"
          >
            <RotateCw className="w-3.5 h-3.5" />
          </button>

          <div className="h-4 w-px bg-slate-700 mx-1" />

          {/* Close Viewer */}
          <button
            onClick={onClose}
            title="Close viewer"
            className="p-1 hover:bg-red-500/20 hover:text-red-400 rounded transition text-slate-400"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Page Navigation Bar */}
      <div className="flex items-center justify-between px-4 py-1.5 bg-slate-800/80 border-b border-slate-700 text-xs text-slate-300">
        <button
          onClick={() => changePage(-1)}
          disabled={pageNum <= 1}
          className="flex items-center space-x-1 px-2 py-1 rounded hover:bg-slate-700 disabled:opacity-40 disabled:hover:bg-transparent transition"
        >
          <ChevronLeft className="w-3.5 h-3.5" />
          <span>Prev</span>
        </button>

        <div className="flex items-center space-x-1.5">
          <input
            type="number"
            min={1}
            max={totalPages || 1}
            value={pageNum}
            onChange={(e) => {
              const val = parseInt(e.target.value, 10)
              if (!isNaN(val) && val >= 1 && val <= totalPages) {
                setPageNum(val)
              }
            }}
            className="w-10 text-center bg-slate-900 border border-slate-600 rounded py-0.5 text-xs text-white focus:outline-none focus:border-indigo-500"
          />
          <span className="text-slate-400 font-mono">/ {totalPages || 1}</span>
        </div>

        <button
          onClick={() => changePage(1)}
          disabled={pageNum >= totalPages}
          className="flex items-center space-x-1 px-2 py-1 rounded hover:bg-slate-700 disabled:opacity-40 disabled:hover:bg-transparent transition"
        >
          <span>Next</span>
          <ChevronRight className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Canvas Viewport */}
      <div className="flex-1 overflow-auto p-4 flex justify-center items-start bg-slate-950/70">
        {loading && (
          <div className="flex flex-col items-center justify-center h-64 space-y-3 text-slate-400">
            <Loader2 className="w-8 h-8 animate-spin text-indigo-500" />
            <span className="text-sm">Rendering PDF...</span>
          </div>
        )}

        {error && (
          <div className="p-4 bg-red-950/40 border border-red-800 rounded-lg text-red-200 text-xs max-w-xs text-center">
            {error}
          </div>
        )}

        <canvas
          ref={canvasRef}
          className={`shadow-2xl rounded border border-slate-700 transition-opacity duration-200 ${
            loading ? 'opacity-0' : 'opacity-100'
          }`}
        />
      </div>
    </div>
  )
}
