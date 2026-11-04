import { FileCode, FileSpreadsheet, FileText, FileType, Presentation, type LucideProps } from 'lucide-react'
import type { DocumentFormat } from '@/types'

const ICONS: Record<DocumentFormat, typeof FileText> = {
  pdf: FileText,
  docx: FileText,
  pptx: Presentation,
  txt: FileType,
  md: FileType,
  csv: FileSpreadsheet,
  html: FileCode,
}

export function FormatIcon({ format, ...props }: { format?: DocumentFormat } & LucideProps) {
  const Icon = (format && ICONS[format]) || FileText
  return <Icon {...props} />
}
