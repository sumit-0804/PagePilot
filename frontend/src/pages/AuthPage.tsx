import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { supabase } from '@/lib/supabase'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export default function AuthPage({ mode }: { mode: 'signin' | 'signup' }) {
  const { session } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  const from = (location.state as { from?: string } | null)?.from ?? '/'
  if (session) return <Navigate to={from} replace />

  const signingUp = mode === 'signup'

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      if (signingUp) {
        const { data, error } = await supabase.auth.signUp({ email, password })
        if (error) throw error
        // With email confirmation on, Supabase returns no session until the link is clicked.
        if (!data.session) {
          setNotice(`We sent a confirmation link to ${email}. Open it, then sign in.`)
          return
        }
      } else {
        const { error } = await supabase.auth.signInWithPassword({ email, password })
        if (error) throw error
      }
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-background px-4 py-10">
      <div className="w-full max-w-sm">
        <p className="font-reading text-3xl font-semibold tracking-tight">PagePilot</p>
        <p className="mt-2 text-muted-foreground">
          Upload a PDF, ask it questions, and get answers that point to the page they came from.
        </p>

        <form onSubmit={submit} className="mt-8 space-y-4 rounded-xl border bg-card p-6">
          <h1 className="text-lg font-semibold">{signingUp ? 'Create your account' : 'Sign in'}</h1>
          <div className="space-y-1.5">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete={signingUp ? 'new-password' : 'current-password'}
              required
              minLength={signingUp ? 8 : undefined}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {signingUp && <p className="text-xs text-muted-foreground">At least 8 characters.</p>}
          </div>

          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          {notice && (
            <p role="status" className="text-sm text-success">
              {notice}
            </p>
          )}

          <Button type="submit" size="lg" className="w-full" disabled={busy}>
            {busy && <Loader2 className="animate-spin" />}
            {signingUp ? 'Create account' : 'Sign in'}
          </Button>
        </form>

        <p className="mt-4 text-sm text-muted-foreground">
          {signingUp ? 'Already have an account? ' : 'New to PagePilot? '}
          <Link className="font-semibold text-primary underline-offset-4 hover:underline" to={signingUp ? '/login' : '/signup'} state={location.state}>
            {signingUp ? 'Sign in' : 'Create an account'}
          </Link>
        </p>
      </div>
    </main>
  )
}
