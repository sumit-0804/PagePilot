import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { UploadDropzone } from '@/components/documents/UploadDropzone'
import type { StudyDocument } from '@/types'

export function UploadDialog({
  open,
  onOpenChange,
  onUploaded,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUploaded: (doc: StudyDocument) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="rounded-2xl border-border/80 bg-card/95 backdrop-blur-xl sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-reading text-xl">Upload a document</DialogTitle>
          <DialogDescription>It's ready to chat with once its text is read and indexed.</DialogDescription>
        </DialogHeader>
        <UploadDropzone
          compact
          onUploaded={(doc) => {
            onOpenChange(false)
            onUploaded(doc)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
