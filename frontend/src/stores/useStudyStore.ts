import { create } from 'zustand'
import type { Conversation, Document, FlashcardSet, QuizSession } from '../types'

interface StudyStore {
  documents: Document[]
  activeDocumentId: string | null
  conversations: Conversation[]
  activeConversationId: string | null
  
  // PDF Viewer State
  isPdfViewerOpen: boolean
  targetPdfPage: number
  
  // Active Interactive Sessions
  activeQuiz: QuizSession | null
  activeFlashcards: FlashcardSet | null
  
  // UI Theme
  isDarkMode: boolean
  isSidebarOpen: boolean

  // Actions
  setDocuments: (docs: Document[]) => void
  addDocument: (doc: Document) => void
  removeDocument: (docId: string) => void
  updateDocumentStatus: (docId: string, status: Document['status'], pageCount?: number, errorMsg?: string) => void
  setActiveDocumentId: (id: string | null) => void
  
  setConversations: (convs: Conversation[]) => void
  addConversation: (conv: Conversation) => void
  removeConversation: (convId: string) => void
  setActiveConversationId: (id: string | null) => void
  
  setPdfViewerOpen: (open: boolean) => void
  jumpToPdfPage: (page: number) => void
  
  setActiveQuiz: (quiz: QuizSession | null) => void
  setActiveFlashcards: (set: FlashcardSet | null) => void
  
  toggleDarkMode: () => void
  toggleSidebar: () => void
}

export const useStudyStore = create<StudyStore>((set) => ({
  documents: [],
  activeDocumentId: null,
  conversations: [],
  activeConversationId: null,
  
  isPdfViewerOpen: false,
  targetPdfPage: 1,
  
  activeQuiz: null,
  activeFlashcards: null,
  
  isDarkMode: false,
  isSidebarOpen: true,

  setDocuments: (documents) => set({ documents }),
  addDocument: (doc) => set((state) => ({ documents: [doc, ...state.documents] })),
  removeDocument: (docId) =>
    set((state) => ({
      documents: state.documents.filter((d) => d.id !== docId),
      activeDocumentId: state.activeDocumentId === docId ? null : state.activeDocumentId,
    })),
  updateDocumentStatus: (docId, status, pageCount, errorMsg) =>
    set((state) => ({
      documents: state.documents.map((d) => {
        if (d.id === docId) {
          return {
            ...d,
            status,
            page_count: pageCount !== undefined ? pageCount : d.page_count,
            error_message: errorMsg !== undefined ? errorMsg : d.error_message,
          }
        }
        return d
      }),
    })),
  setActiveDocumentId: (activeDocumentId) => set({ activeDocumentId }),

  setConversations: (conversations) => set({ conversations }),
  addConversation: (conv) => set((state) => ({ conversations: [conv, ...state.conversations] })),
  removeConversation: (convId) =>
    set((state) => ({
      conversations: state.conversations.filter((c) => c.id !== convId),
      activeConversationId: state.activeConversationId === convId ? null : state.activeConversationId,
    })),
  setActiveConversationId: (activeConversationId) => set({ activeConversationId }),

  setPdfViewerOpen: (isPdfViewerOpen) => set({ isPdfViewerOpen }),
  jumpToPdfPage: (page) => set({ targetPdfPage: page, isPdfViewerOpen: true }),

  setActiveQuiz: (activeQuiz) => set({ activeQuiz }),
  setActiveFlashcards: (activeFlashcards) => set({ activeFlashcards }),

  toggleDarkMode: () => {
    set((state) => {
      const next = !state.isDarkMode
      if (next) {
        document.documentElement.classList.add('dark')
      } else {
        document.documentElement.classList.remove('dark')
      }
      return { isDarkMode: next }
    })
  },
  toggleSidebar: () => set((state) => ({ isSidebarOpen: !state.isSidebarOpen })),
}))
