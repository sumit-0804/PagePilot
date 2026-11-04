import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { motion } from 'motion/react'
import { BookOpenCheck, CheckCircle2, Loader2, Sparkles } from 'lucide-react'
import { toast } from 'sonner'
import { supabase } from '@/lib/supabase'
import { api } from '@/services/api'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spotlight } from '@/components/ui/aceternity/spotlight'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'

const HIGHLIGHTS = [
  'Numbered sources from your documents and the web',
  'Interactive quizzes with instant grading',
  '3D flashcards for active recall',
]

export default function AuthPage({ mode }: { mode: 'signin' | 'signup' }) {
  const { session, isAccount, isGuest } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  const from = (location.state as { from?: string } | null)?.from ?? '/'
  // While a guest logs in, the session briefly becomes the account before the claim finishes.
  if (isAccount && !busy) return <Navigate to={from} replace />

  const signingUp = mode === 'signup'

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      if (signingUp && isGuest) {
        // Upgrading the anonymous user keeps its id, so the guest's chat stays with it.
        const { data, error } = await supabase.auth.updateUser({ email, password })
        if (error) throw error
        if (data.user?.is_anonymous) {
          setNotice(`We sent a confirmation link to ${email}. Open it to finish creating your account.`)
          return
        }
        // The current access token still says is_anonymous until it is reissued.
        const { error: refreshError } = await supabase.auth.refreshSession()
        if (refreshError) throw refreshError
      } else if (signingUp) {
        const { data, error } = await supabase.auth.signUp({ email, password })
        if (error) throw error
        // With email confirmation on, Supabase returns no session until the link is clicked.
        if (!data.session) {
          setNotice(`We sent a confirmation link to ${email}. Open it, then sign in.`)
          return
        }
      } else {
        const guestToken = isGuest ? session?.access_token : undefined
        const { error } = await supabase.auth.signInWithPassword({ email, password })
        if (error) throw error
        if (guestToken) {
          try {
            await api.account.claim(guestToken)
          } catch (err) {
            toast.error(`Signed in, but your guest chat couldn't be moved: ${err instanceof Error ? err.message : 'unknown error'}`)
            navigate('/', { replace: true })
            return
          }
        }
      }
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong. Try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="relative grid min-h-dvh place-items-center overflow-hidden bg-background px-4 py-10">
      <BackgroundAmbient variant="grid" />
      <Spotlight className="-top-32 left-0 md:-top-20 md:left-44" />

      <motion.div
        initial={{ opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35 }}
        className="relative z-10 w-full max-w-md"
      >
        {/* Brand header */}
        <div className="text-center">
          <div className="mx-auto inline-flex size-12 items-center justify-center rounded-2xl bg-gradient-to-br from-primary to-primary/80 text-primary-foreground shadow-md shadow-primary/20">
            <BookOpenCheck className="size-6" />
          </div>
          <p className="mt-3 font-reading text-3xl font-semibold tracking-tight">PagePilot</p>
          <p className="mt-2 text-sm text-muted-foreground">
            {isGuest
              ? 'Create an account or sign in to keep chatting. Your chat so far comes with you.'
              : 'Chat, study and quiz yourself, with answers that cite the page or link they came from.'}
          </p>
        </div>

        <form
          onSubmit={submit}
          className="mt-7 space-y-4 rounded-2xl border border-border/80 bg-card/90 p-6 shadow-lg shadow-foreground/[0.03] backdrop-blur-xl sm:p-7"
        >
          <div className="flex items-center justify-between">
            <h1 className="font-reading text-xl font-semibold">
              {signingUp ? 'Create your account' : 'Welcome back'}
            </h1>
            <span className="inline-flex items-center gap-1 rounded-full bg-accent px-2.5 py-0.5 text-xs font-semibold text-accent-foreground">
              <Sparkles className="size-3" />
              Study Studio
            </span>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="rounded-xl"
            />
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
              className="rounded-xl"
            />
            {signingUp && <p className="text-xs text-muted-foreground">At least 8 characters.</p>}
          </div>

          {error && (
            <p role="alert" className="rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}
          {notice && (
            <p role="status" className="rounded-xl border border-success/30 bg-success/10 px-3 py-2 text-sm text-success">
              {notice}
            </p>
          )}

          <Button type="submit" size="lg" className="w-full rounded-xl shadow-sm shadow-primary/20" disabled={busy}>
            {busy && <Loader2 className="animate-spin" />}
            {signingUp ? 'Create account' : 'Sign in'}
          </Button>

          {/* Feature checklist */}
          <ul className="space-y-1.5 border-t border-border/60 pt-4 text-xs text-muted-foreground">
            {HIGHLIGHTS.map((item) => (
              <li key={item} className="flex items-center gap-2">
                <CheckCircle2 className="size-3.5 shrink-0 text-primary" />
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </form>

        <p className="mt-5 text-center text-sm text-muted-foreground">
          {signingUp ? 'Already have an account? ' : 'New to PagePilot? '}
          <Link
            className="font-semibold text-primary underline-offset-4 hover:underline"
            to={signingUp ? '/login' : '/signup'}
            state={location.state}
          >
            {signingUp ? 'Sign in' : 'Create an account'}
          </Link>
        </p>
      </motion.div>
    </main>
  )
}
