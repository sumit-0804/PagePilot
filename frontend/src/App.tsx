import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { AppShell } from '@/components/layout/AppShell'
import AuthPage from '@/pages/AuthPage'
import HomePage from '@/pages/HomePage'
import DocumentPage from '@/pages/DocumentPage'
import ChatPage from '@/pages/ChatPage'
import HistoryPage from '@/pages/HistoryPage'
import SettingsPage from '@/pages/SettingsPage'
import NotFoundPage from '@/pages/NotFoundPage'

/** Pages for real accounts; a guest (anonymous) session counts as signed out. */
function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAccount, loading } = useAuth()
  const location = useLocation()
  if (loading) {
    return (
      <div className="grid h-full place-items-center text-muted-foreground" role="status">
        <Loader2 className="size-5 animate-spin" aria-label="Loading" />
      </div>
    )
  }
  if (!isAccount) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthPage mode="signin" />} />
      <Route path="/signup" element={<AuthPage mode="signup" />} />
      <Route element={<AppShell />}>
        <Route index element={<HomePage />} />
        <Route path="chat/:conversationId" element={<ChatPage />} />
        <Route path="documents/:documentId" element={<RequireAuth><DocumentPage /></RequireAuth>} />
        <Route path="history" element={<RequireAuth><HistoryPage /></RequireAuth>} />
        <Route path="settings" element={<RequireAuth><SettingsPage /></RequireAuth>} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
