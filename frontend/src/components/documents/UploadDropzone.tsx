import { useRef, useState, type DragEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { FileText, FileUp, Loader2, Sparkles } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys } from '@/hooks/queries'
import { cn } from '@/lib/utils'
import { UPLOAD_EXTENSIONS } from '@/lib/format'
import { validateUpload } from '@/lib/upload'
import { Button } from '@/components/ui/button'
import type { StudyDocument } from '@/types'

export function UploadDropzone({ onUploaded, compact = false }: { onUploaded: (doc: StudyDocument) => void; compact?: boolean }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
  const [hovered, setHovered] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const upload = useMutation({
    mutationFn: api.documents.upload,
    onSuccess: async (doc) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.documents })
      onUploaded(doc)
    },
    onError: (err) => setError(err.message),
  })

  function pick(files: FileList | null) {
    const file = files?.[0]
    if (!file) return
    const invalid = validateUpload(file)
    setError(invalid)
    if (!invalid) upload.mutate(file)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    setDragging(false)
    pick(e.dataTransfer.files)
  }

  const active = dragging || hovered

  return (
    <div>
      <motion.div
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        onClick={() => !upload.isPending && inputRef.current?.click()}
        whileHover={{ scale: 1.005 }}
        transition={{ type: 'spring', stiffness: 360, damping: 28 }}
        className={cn(
          'group/dropzone relative flex cursor-pointer flex-col items-center justify-center overflow-hidden rounded-2xl border-2 border-dashed border-border bg-card/85 text-center shadow-sm backdrop-blur-sm transition-all duration-300',
          compact ? 'px-5 py-7' : 'px-6 py-12',
          dragging
            ? 'border-primary bg-accent/60 shadow-lg shadow-primary/10'
            : 'hover:border-primary/60 hover:bg-card hover:shadow-md hover:shadow-primary/5',
        )}
      >
        {/* Aceternity subtle grid pattern background */}
        <div
          className="pointer-events-none absolute inset-0 bg-grid-pattern opacity-45 dark:opacity-25"
          style={{
            maskImage: 'radial-gradient(ellipse at center, white, transparent 80%)',
            WebkitMaskImage: 'radial-gradient(ellipse at center, white, transparent 80%)',
          }}
          aria-hidden
        />

        {upload.isPending ? (
          <div className="relative z-10 flex flex-col items-center py-2">
            <div className="relative flex size-14 items-center justify-center rounded-2xl bg-primary/15 text-primary">
              <Loader2 className="size-7 animate-spin" />
              <span className="absolute -inset-1 animate-ping rounded-2xl bg-primary/15" />
            </div>
            <p className="mt-4 font-reading text-lg font-semibold">Uploading your document…</p>
            <p className="mt-1 text-sm text-muted-foreground">Preparing pages for instant citations and study tools</p>
          </div>
        ) : (
          <div className="relative z-10 flex flex-col items-center">
            {/* Aceternity animated floating file stack illustration */}
            <div className="relative mb-4 flex h-16 w-24 items-center justify-center" aria-hidden>
              <motion.div
                animate={
                  active
                    ? { x: -18, y: -4, rotate: -12, scale: 1.04 }
                    : { x: -8, y: 2, rotate: -6, scale: 0.95 }
                }
                transition={{ type: 'spring', stiffness: 300, damping: 20 }}
                className="absolute flex h-14 w-11 items-center justify-center rounded-xl border border-border bg-secondary/90 text-muted-foreground shadow-xs"
              >
                <FileText className="size-4 opacity-60" />
              </motion.div>
              <motion.div
                animate={
                  active
                    ? { x: 18, y: -4, rotate: 12, scale: 1.04 }
                    : { x: 8, y: 2, rotate: 6, scale: 0.95 }
                }
                transition={{ type: 'spring', stiffness: 300, damping: 20 }}
                className="absolute flex h-14 w-11 items-center justify-center rounded-xl border border-border bg-secondary/90 text-muted-foreground shadow-xs"
              >
                <Sparkles className="size-4 text-terracotta opacity-70" />
              </motion.div>
              <motion.div
                animate={active ? { y: -6, scale: 1.08 } : { y: 0, scale: 1 }}
                transition={{ type: 'spring', stiffness: 320, damping: 20 }}
                className="relative z-10 flex h-15 w-12 items-center justify-center rounded-xl border border-primary/35 bg-card text-primary shadow-md shadow-primary/10"
              >
                <FileUp className="size-6" />
              </motion.div>
            </div>

            <p className="font-reading text-lg font-semibold text-foreground">
              {dragging ? 'Release to upload your document' : 'Drop a document here, or browse'}
            </p>
            <p className="mt-1 max-w-sm text-sm text-muted-foreground">
              PDF, DOCX, PPTX, TXT, MD, CSV or HTML · Up to 50 MB · Scanned PDFs read with OCR
            </p>
            <Button
              type="button"
              className="mt-5 rounded-xl shadow-xs"
              variant="outline"
              onClick={(e) => {
                e.stopPropagation()
                inputRef.current?.click()
              }}
            >
              Choose a file
            </Button>
          </div>
        )}

        <input
          ref={inputRef}
          type="file"
          accept={UPLOAD_EXTENSIONS.join(',')}
          className="sr-only"
          aria-label="Choose a document to upload"
          onChange={(e) => {
            pick(e.target.files)
            e.target.value = ''
          }}
        />
      </motion.div>
      {error && (
        <motion.p
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          role="alert"
          className="mt-2.5 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        >
          {error}
        </motion.p>
      )}
    </div>
  )
}
