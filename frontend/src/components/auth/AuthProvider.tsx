import { useEffect, useState, type ReactNode } from 'react'
import type { Session } from '@supabase/supabase-js'
import { useQueryClient } from '@tanstack/react-query'
import { supabase } from '@/lib/supabase'
import { AuthContext } from '@/hooks/useAuth'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)
  const queryClient = useQueryClient()

  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => {
      setSession(data.session)
      setLoading(false)
    })
    const { data } = supabase.auth.onAuthStateChange((event, next) => {
      setSession(next)
      // Never show one account's cached documents to the next account.
      if (event === 'SIGNED_OUT') queryClient.clear()
    })
    return () => data.subscription.unsubscribe()
  }, [queryClient])

  const signOut = async () => {
    const { error } = await supabase.auth.signOut()
    if (error) throw error
  }

  return <AuthContext.Provider value={{ session, loading, signOut }}>{children}</AuthContext.Provider>
}
