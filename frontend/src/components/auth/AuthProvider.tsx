import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Session } from '@supabase/supabase-js'
import { useQueryClient } from '@tanstack/react-query'
import { supabase } from '@/lib/supabase'
import { AuthContext } from '@/hooks/useAuth'

/** Identifies who the cached data belongs to: a guest becoming an account counts as a change. */
const identity = (s: Session | null) => (s ? `${s.user.id}:${s.user.is_anonymous ? 'guest' : 'account'}` : '')

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)
  const queryClient = useQueryClient()
  const lastIdentity = useRef('')

  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => {
      lastIdentity.current = identity(data.session)
      setSession(data.session)
      setLoading(false)
    })
    const { data } = supabase.auth.onAuthStateChange((event, next) => {
      setSession(next)
      // Never show one account's cached documents to the next account.
      if (event === 'SIGNED_OUT') queryClient.clear()
      else if (identity(next) !== lastIdentity.current) void queryClient.invalidateQueries()
      lastIdentity.current = identity(next)
    })
    return () => data.subscription.unsubscribe()
  }, [queryClient])

  const signOut = async () => {
    const { error } = await supabase.auth.signOut()
    if (error) throw error
  }

  const isAccount = !!session && !session.user.is_anonymous
  const isGuest = !!session && !!session.user.is_anonymous

  return <AuthContext.Provider value={{ session, loading, isAccount, isGuest, signOut }}>{children}</AuthContext.Provider>
}
