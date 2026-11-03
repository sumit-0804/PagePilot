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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload a PDF</DialogTitle>
          <DialogDescription>It's ready to chat with once its pages are read and indexed.</DialogDescription>
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
