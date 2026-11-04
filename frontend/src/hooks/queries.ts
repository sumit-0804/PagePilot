import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import { isProcessing } from '@/lib/format'
import { useAuth } from '@/hooks/useAuth'

export const queryKeys = {
  section: (documentId: string, n: number) => ['section', documentId, n] as const,
  progressAll: ['progress'] as const,
  documents: ['documents'] as const,
  conversations: ['conversations'] as const,
  conversation: (id: string) => ['conversations', id] as const,
  quiz: (id: string) => ['quiz', id] as const,
  flashcards: (id: string) => ['flashcards', id] as const,
  progress: (documentId: string) => ['progress', documentId] as const,
  fileUrl: (documentId: string) => ['file-url', documentId] as const,
}

/** All documents; polls while any of them is still being processed. Accounts only. */
export function useDocuments() {
  const { isAccount } = useAuth()
  return useQuery({
    queryKey: queryKeys.documents,
    queryFn: api.documents.list,
    enabled: isAccount,
    refetchInterval: (query) => (query.state.data?.some((d) => isProcessing(d.status)) ? 1500 : false),
  })
}

export function useDocument(id: string | undefined) {
  const docs = useDocuments()
  return { ...docs, data: docs.data?.find((d) => d.id === id) }
}

/** Saved chats; guests can't list theirs. */
export function useConversations() {
  const { isAccount } = useAuth()
  return useQuery({ queryKey: queryKeys.conversations, queryFn: api.conversations.list, enabled: isAccount })
}
