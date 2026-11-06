import { UPLOAD_EXTENSIONS } from '@/lib/format'

export const MAX_UPLOAD_BYTES = 50 * 1024 * 1024

/** Returns a user-facing error for a file the server won't accept, or null if it's fine. */
export function validateUpload(file: File): string | null {
  const name = file.name.toLowerCase()
  if (!UPLOAD_EXTENSIONS.some((ext) => name.endsWith(ext))) {
    return 'This file type isn’t supported. Upload a PDF, Word (.docx), PowerPoint (.pptx), TXT, Markdown, CSV or HTML file.'
  }
  if (file.size > MAX_UPLOAD_BYTES) {
    return 'This file is larger than 50 MB. Try a smaller file or split it into parts.'
  }
  return null
}

/** True when a drag carries files (not dragged text, links or images from the page). */
export function isFileDrag(dt: Pick<DataTransfer, 'types'> | null): boolean {
  return !!dt && Array.from(dt.types).includes('Files')
}
