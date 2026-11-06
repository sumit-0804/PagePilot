import { useRef, useState, type DragEvent } from 'react'
import { isFileDrag } from '@/lib/upload'

/** Drop-zone handlers for a container. A depth counter keeps the overlay steady while dragging over child elements. */
export function useFileDrop(onFiles: (files: File[]) => void, enabled = true) {
  const [dragging, setDragging] = useState(false)
  const depth = useRef(0)

  const accepts = (e: DragEvent) => enabled && isFileDrag(e.dataTransfer)

  const handlers = {
    onDragEnter: (e: DragEvent) => {
      if (!accepts(e)) return
      e.preventDefault()
      depth.current += 1
      setDragging(true)
    },
    onDragOver: (e: DragEvent) => {
      if (!accepts(e)) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'copy'
    },
    onDragLeave: (e: DragEvent) => {
      if (!accepts(e)) return
      depth.current = Math.max(0, depth.current - 1)
      if (depth.current === 0) setDragging(false)
    },
    onDrop: (e: DragEvent) => {
      if (!accepts(e)) return
      e.preventDefault()
      depth.current = 0
      setDragging(false)
      const files = Array.from(e.dataTransfer.files)
      if (files.length > 0) onFiles(files)
    },
  }

  return { dragging: enabled && dragging, handlers }
}
