import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { DocumentFormat, ExplanationStyle } from '@/types'

export type AccentTheme = 'blue' | 'violet' | 'cyan' | 'emerald' | 'rose' | 'amber'

function applyAccentAttribute(accent: AccentTheme) {
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.accent = accent
  }
}

export interface ViewerTarget {
  documentId: string
  /** Picks the PDF or text viewer; looked up from the document list when unknown. */
  format?: DocumentFormat
  /** Page for PDFs, section number for other formats. */
  page: number
  /** True when opened from a citation, so the viewer marks the cited page. */
  fromCitation: boolean
  /** Changes on every open so clicking the same citation twice still jumps. */
  nonce: number
}

interface OpenOptions {
  page?: number
  fromCitation?: boolean
  format?: DocumentFormat
}

/** A first message typed on a document page, sent once its new conversation opens. */
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

  viewer: ViewerTarget | null
  openViewer: (documentId: string, options?: OpenOptions) => void
  closeViewer: () => void

  /** Explanation style applied to every question unless one is picked in the composer. */
  defaultStyle: ExplanationStyle | ''
  setDefaultStyle: (style: ExplanationStyle | '') => void

  /** Vibrant color accent preset. */
  accentTheme: AccentTheme
  setAccentTheme: (accent: AccentTheme) => void

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

      viewer: null,
      openViewer: (documentId, { page = 1, fromCitation = false, format } = {}) =>
        set({ viewer: { documentId, format, page, fromCitation, nonce: Date.now() } }),
      closeViewer: () => set({ viewer: null }),

      defaultStyle: '',
      setDefaultStyle: (defaultStyle) => set({ defaultStyle }),

      accentTheme: 'blue',
      setAccentTheme: (accentTheme) => {
        applyAccentAttribute(accentTheme)
        set({ accentTheme })
      },

      pendingMessage: null,
      setPendingMessage: (pendingMessage) => set({ pendingMessage }),
    }),
    {
      name: 'pagepilot-ui',
      storage: createJSONStorage(() => localStorage),
      // Only preferences survive a reload; the viewer and pending messages are per-visit.
      partialize: (state) => ({
        sidebarOpen: state.sidebarOpen,
        defaultStyle: state.defaultStyle,
        accentTheme: state.accentTheme,
      }),
      onRehydrateStorage: () => (state) => {
        if (state?.accentTheme) {
          applyAccentAttribute(state.accentTheme)
        }
      },
    },
  ),
)
