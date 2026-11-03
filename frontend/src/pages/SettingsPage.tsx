import { useTheme } from 'next-themes'
import { toast } from 'sonner'
import { Monitor, Moon, Sun } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { useStudyStore } from '@/stores/useStudyStore'
import { STYLE_OPTIONS } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/layout/PageHeader'

const THEMES = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'Match system', icon: Monitor },
]

export default function SettingsPage() {
  const { theme, setTheme } = useTheme()
  const { session, signOut } = useAuth()
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const setDefaultStyle = useStudyStore((s) => s.setDefaultStyle)

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Settings" />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl space-y-10 px-4 py-10 sm:px-6">
          <h1 className="font-reading text-3xl font-semibold tracking-tight">Settings</h1>

          <fieldset>
            <legend className="font-semibold">Appearance</legend>
            <div className="mt-3 flex flex-wrap gap-2">
              {THEMES.map(({ value, label, icon: Icon }) => (
                <Button key={value} variant={theme === value ? 'default' : 'outline'} aria-pressed={theme === value} onClick={() => setTheme(value)}>
                  <Icon />
                  {label}
                </Button>
              ))}
            </div>
          </fieldset>

          <fieldset>
            <legend className="font-semibold">Default explanation style</legend>
            <p className="mt-1 text-sm text-muted-foreground">Used for every question unless you pick a different style in the message box.</p>
            <div className="mt-3 grid gap-2 sm:grid-cols-2">
              {[{ value: '' as const, label: 'Default', hint: 'Balanced explanations' }, ...STYLE_OPTIONS].map((opt) => (
                <button
                  key={opt.value || 'default'}
                  aria-pressed={defaultStyle === opt.value}
                  onClick={() => setDefaultStyle(opt.value)}
                  className={cn('rounded-lg border bg-card px-3 py-2 text-left hover:border-primary/60', defaultStyle === opt.value && 'border-primary bg-accent')}
                >
                  <span className="block font-semibold">{opt.label}</span>
                  <span className="block text-sm text-muted-foreground">{opt.hint}</span>
                </button>
              ))}
            </div>
          </fieldset>

          <section aria-labelledby="account">
            <h2 id="account" className="font-semibold">
              Account
            </h2>
            <p className="mt-1 text-muted-foreground">Signed in as {session?.user.email}</p>
            <Button variant="outline" className="mt-3" onClick={() => signOut().catch((e: Error) => toast.error(e.message))}>
              Sign out
            </Button>
          </section>
        </div>
      </div>
    </div>
  )
}
