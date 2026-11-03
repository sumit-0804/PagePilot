import { useRef, useState, type DragEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { FileUp, Loader2 } from 'lucide-react'
import { api } from '@/services/api'
import { queryKeys } from '@/hooks/queries'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import type { StudyDocument } from '@/types'

const MAX_BYTES = 50 * 1024 * 1024

export function UploadDropzone({ onUploaded, compact = false }: { onUploaded: (doc: StudyDocument) => void; compact?: boolean }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
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
    setError(null)
    if (file.type !== 'application/pdf' && !file.name.toLowerCase().endsWith('.pdf')) {
      setError('Only PDF files can be uploaded.')
      return
    }
    if (file.size > MAX_BYTES) {
      setError('This PDF is larger than 50 MB. Try a smaller file or split it into parts.')
      return
    }
    upload.mutate(file)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    setDragging(false)
    pick(e.dataTransfer.files)
  }

  return (
    <div>
      <div
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-input bg-card text-center transition-colors',
          compact ? 'px-4 py-6' : 'px-6 py-12',
          dragging && 'border-primary bg-accent',
        )}
      >
        {upload.isPending ? (
          <>
            <Loader2 className="size-6 animate-spin text-primary" />
            <p className="mt-3 font-semibold">Uploading…</p>
          </>
        ) : (
          <>
            <FileUp className="size-6 text-muted-foreground" />
            <p className="mt-3 font-semibold">Drop a PDF here</p>
            <p className="mt-1 text-sm text-muted-foreground">Up to 50 MB. Scanned PDFs need a text layer.</p>
            <Button className="mt-4" variant="outline" onClick={() => inputRef.current?.click()}>
              Choose a file
            </Button>
          </>
        )}
        <input
          ref={inputRef}
          type="file"
          accept="application/pdf,.pdf"
          className="sr-only"
          aria-label="Choose a PDF to upload"
          onChange={(e) => {
            pick(e.target.files)
            e.target.value = ''
          }}
        />
      </div>
      {error && (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}
