import { useParams } from 'react-router-dom'
import { ChatView } from '@/components/chat/ChatView'

export default function ChatPage() {
  const { conversationId = '' } = useParams()
  return <ChatView key={conversationId} conversationId={conversationId} />
}
