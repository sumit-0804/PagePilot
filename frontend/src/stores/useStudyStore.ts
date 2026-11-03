import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { ExplanationStyle } from '@/types'

interface PdfTarget {
  documentId: string
  page: number
  /** True when opened from a citation, so the viewer marks the cited page. */
  fromCitation: boolean
  /** Changes on every open so clicking the same citation twice still jumps. */
  nonce: number
}

interface PendingMessage {
  conversationId: string
  text: string
  style?: ExplanationStyle
}

interface StudyState {
  /** Desktop sidebar; remembered between visits. */
  sidebarOpen: boolean
  setSidebarOpen: (open: boolean) => void
  /** Phone menu overlay; always starts closed. */
  mobileNavOpen: boolean
  setMobileNavOpen: (open: boolean) => void

  pdf: PdfTarget | null
  openPdf: (documentId: string, page?: number, fromCitation?: boolean) => void
  closePdf: () => void

  /** Explanation style applied to every question unless one is picked in the composer. */
  defaultStyle: ExplanationStyle | ''
  setDefaultStyle: (style: ExplanationStyle | '') => void

  /** First message typed on a document page, sent once its new conversation opens. */
  pendingMessage: PendingMessage | null
  setPendingMessage: (message: PendingMessage | null) => void
}

export const useStudyStore = create<StudyState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      mobileNavOpen: false,
      setMobileNavOpen: (mobileNavOpen) => set({ mobileNavOpen }),

      pdf: null,
      openPdf: (documentId, page = 1, fromCitation = false) =>
        set({ pdf: { documentId, page, fromCitation, nonce: Date.now() } }),
      closePdf: () => set({ pdf: null }),

      defaultStyle: '',
      setDefaultStyle: (defaultStyle) => set({ defaultStyle }),

      pendingMessage: null,
      setPendingMessage: (pendingMessage) => set({ pendingMessage }),
    }),
    {
      name: 'pagepilot-ui',
      storage: createJSONStorage(() => localStorage),
      // Only preferences survive a reload; the PDF panel and pending messages are per-visit.
      partialize: (state) => ({ sidebarOpen: state.sidebarOpen, defaultStyle: state.defaultStyle }),
    },
  ),
)
