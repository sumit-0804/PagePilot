import { useTheme } from 'next-themes'
import { motion } from 'motion/react'
import { toast } from 'sonner'
import {
  BookOpen,
  Check,
  Code2,
  Compass,
  Feather,
  GraduationCap,
  Layers,
  Lightbulb,
  LogOut,
  Microscope,
  Monitor,
  Moon,
  Palette,
  Sparkles,
  Sun,
  User,
} from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { useStudyStore, type AccentTheme } from '@/stores/useStudyStore'
import { STYLE_OPTIONS } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'
import { CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { PageHeader } from '@/components/layout/PageHeader'
import type { ExplanationStyle } from '@/types'

const THEMES = [
  { value: 'light', label: 'Light', hint: 'Crisp white & cobalt blue', icon: Sun },
  { value: 'dark', label: 'Dark', hint: 'True black & vibrant neon', icon: Moon },
  { value: 'system', label: 'Match system', hint: 'Follows device setting', icon: Monitor },
]

const ACCENT_PALETTES: { value: AccentTheme; label: string; dotClass: string; ringClass: string }[] = [
  { value: 'blue', label: 'Electric Blue', dotClass: 'bg-blue-500', ringClass: 'border-blue-500/60 bg-blue-500/10 text-blue-600 dark:text-blue-400' },
  { value: 'violet', label: 'Cyber Violet', dotClass: 'bg-purple-500', ringClass: 'border-purple-500/60 bg-purple-500/10 text-purple-600 dark:text-purple-400' },
  { value: 'cyan', label: 'Neon Cyan', dotClass: 'bg-cyan-400', ringClass: 'border-cyan-400/60 bg-cyan-400/10 text-cyan-600 dark:text-cyan-300' },
  { value: 'emerald', label: 'Emerald Mint', dotClass: 'bg-emerald-500', ringClass: 'border-emerald-500/60 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' },
  { value: 'rose', label: 'Crimson Rose', dotClass: 'bg-rose-500', ringClass: 'border-rose-500/60 bg-rose-500/10 text-rose-600 dark:text-rose-400' },
  { value: 'amber', label: 'Solar Amber', dotClass: 'bg-amber-500', ringClass: 'border-amber-500/60 bg-amber-500/10 text-amber-600 dark:text-amber-400' },
]

const STYLE_VISUALS: Record<
  ExplanationStyle | 'default',
  {
    icon: typeof Sparkles
    badgeClass: string
    activeBorderClass: string
    spotlight: string
  }
> = {
  default: {
    icon: Sparkles,
    badgeClass: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
    activeBorderClass: 'border-blue-500 bg-blue-500/10 ring-1 ring-blue-500/35',
    spotlight: 'rgba(59, 130, 246, 0.18)',
  },
  simple: {
    icon: Feather,
    badgeClass: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    activeBorderClass: 'border-emerald-500 bg-emerald-500/10 ring-1 ring-emerald-500/35',
    spotlight: 'rgba(16, 185, 129, 0.18)',
  },
  beginner: {
    icon: BookOpen,
    badgeClass: 'bg-teal-500/15 text-teal-600 dark:text-teal-400',
    activeBorderClass: 'border-teal-500 bg-teal-500/10 ring-1 ring-teal-500/35',
    spotlight: 'rgba(20, 184, 166, 0.18)',
  },
  step_by_step: {
    icon: Layers,
    badgeClass: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-300',
    activeBorderClass: 'border-cyan-400 bg-cyan-500/10 ring-1 ring-cyan-400/35',
    spotlight: 'rgba(34, 211, 238, 0.18)',
  },
  detailed: {
    icon: Microscope,
    badgeClass: 'bg-purple-500/15 text-purple-600 dark:text-purple-400',
    activeBorderClass: 'border-purple-500 bg-purple-500/10 ring-1 ring-purple-500/35',
    spotlight: 'rgba(168, 85, 247, 0.18)',
  },
  technical: {
    icon: Code2,
    badgeClass: 'bg-indigo-500/15 text-indigo-600 dark:text-indigo-400',
    activeBorderClass: 'border-indigo-500 bg-indigo-500/10 ring-1 ring-indigo-500/35',
    spotlight: 'rgba(99, 102, 241, 0.18)',
  },
  analogy: {
    icon: Compass,
    badgeClass: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    activeBorderClass: 'border-amber-500 bg-amber-500/10 ring-1 ring-amber-500/35',
    spotlight: 'rgba(245, 158, 11, 0.18)',
  },
  example: {
    icon: Lightbulb,
    badgeClass: 'bg-orange-500/15 text-orange-600 dark:text-orange-400',
    activeBorderClass: 'border-orange-500 bg-orange-500/10 ring-1 ring-orange-500/35',
    spotlight: 'rgba(249, 115, 22, 0.18)',
  },
  exam: {
    icon: GraduationCap,
    badgeClass: 'bg-rose-500/15 text-rose-600 dark:text-rose-400',
    activeBorderClass: 'border-rose-500 bg-rose-500/10 ring-1 ring-rose-500/35',
    spotlight: 'rgba(244, 63, 94, 0.18)',
  },
}

export default function SettingsPage() {
  const { theme, setTheme } = useTheme()
  const { session, signOut } = useAuth()
  const defaultStyle = useStudyStore((s) => s.defaultStyle)
  const setDefaultStyle = useStudyStore((s) => s.setDefaultStyle)
  const accentTheme = useStudyStore((s) => s.accentTheme)
  const setAccentTheme = useStudyStore((s) => s.setAccentTheme)

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <BackgroundAmbient variant="dots" />
      <PageHeader title="Settings" />
      <div className="relative z-10 min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-6xl space-y-8 px-6 py-10 lg:px-10">
          <div>
            <div className="inline-flex items-center gap-1.5 rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
              <Sparkles className="size-3.5" />
              Preferences
            </div>
            <h1 className="mt-2.5 font-reading text-3xl font-semibold tracking-tight sm:text-4xl">Settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Customize your study studio theme, vibrant accent palette, default AI explanation mode, and account settings.
            </p>
          </div>

          {/* Top row: Appearance (8 cols) + Account (4 cols) */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-12">
            {/* Appearance & Accent Palette */}
            <fieldset className="rounded-2xl border border-border/80 bg-card/85 p-6 shadow-xs backdrop-blur-sm lg:col-span-8">
              <legend className="px-1 text-base font-semibold">Appearance</legend>
              <p className="mt-1 text-sm text-muted-foreground">
                Choose how PagePilot looks across your workspace and PDF reader.
              </p>
              <div className="mt-4 grid grid-cols-1 gap-3.5 sm:grid-cols-3">
                {THEMES.map(({ value, label, hint, icon: Icon }) => {
                  const active = theme === value
                  return (
                    <motion.button
                      key={value}
                      type="button"
                      whileHover={{ y: -2 }}
                      whileTap={{ scale: 0.98 }}
                      aria-pressed={active}
                      onClick={() => setTheme(value)}
                      className={cn(
                        'flex flex-col items-start gap-3 rounded-xl border bg-card p-4 text-left transition-all',
                        active
                          ? 'border-primary bg-accent/60 shadow-sm ring-1 ring-primary/35'
                          : 'border-border/80 hover:border-primary/50',
                      )}
                    >
                      <div className="flex w-full items-center justify-between">
                        <span className="flex size-9 items-center justify-center rounded-xl bg-secondary text-primary">
                          <Icon className="size-4" />
                        </span>
                        {active && <Check className="size-4 text-primary" />}
                      </div>
                      <div>
                        <span className="block font-semibold">{label}</span>
                        <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span>
                      </div>
                    </motion.button>
                  )
                })}
              </div>

              {/* Vibrant Accent Color Switcher */}
              <div className="mt-6 border-t border-border/70 pt-5">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  <Palette className="size-4 text-primary" />
                  <span>Vibrant accent color</span>
                </div>
                <div className="mt-3 flex flex-wrap gap-2.5">
                  {ACCENT_PALETTES.map((p) => {
                    const active = accentTheme === p.value
                    return (
                      <motion.button
                        key={p.value}
                        type="button"
                        whileHover={{ y: -1, scale: 1.02 }}
                        whileTap={{ scale: 0.97 }}
                        aria-pressed={active}
                        onClick={() => setAccentTheme(p.value)}
                        className={cn(
                          'inline-flex items-center gap-2 rounded-full border px-3.5 py-1.5 text-xs font-semibold transition-all',
                          active ? p.ringClass : 'border-border/80 bg-card text-muted-foreground hover:text-foreground',
                        )}
                      >
                        <span className={cn('size-2.5 rounded-full shadow-xs', p.dotClass)} />
                        <span>{p.label}</span>
                        {active && <Check className="size-3.5" />}
                      </motion.button>
                    )
                  })}
                </div>
              </div>
            </fieldset>

            {/* Account */}
            <section
              aria-labelledby="account"
              className="flex flex-col justify-between rounded-2xl border border-border/80 bg-card/85 p-6 shadow-xs backdrop-blur-sm lg:col-span-4"
            >
              <div>
                <div className="flex items-center gap-2.5">
                  <span className="flex size-9 items-center justify-center rounded-xl bg-accent text-accent-foreground">
                    <User className="size-4" />
                  </span>
                  <h2 id="account" className="text-base font-semibold">
                    Account
                  </h2>
                </div>
                <p className="mt-4 text-xs font-medium tracking-wider text-muted-foreground uppercase">Signed in as</p>
                <p className="mt-1 truncate font-medium text-foreground">{session?.user.email}</p>
              </div>
              <Button
                variant="outline"
                className="mt-6 w-full rounded-xl"
                onClick={() => signOut().catch((e: Error) => toast.error(e.message))}
              >
                <LogOut className="size-4" />
                Sign out
              </Button>
            </section>
          </div>

          {/* Default explanation style: full 3-column spread with vibrant per-mode accents */}
          <fieldset className="rounded-2xl border border-border/80 bg-card/85 p-6 shadow-xs backdrop-blur-sm">
            <legend className="px-1 text-base font-semibold">Default explanation style</legend>
            <p className="mt-1 text-sm text-muted-foreground">
              Used for every question unless you pick a different style in the message box. Each mode is color-coded by cognitive depth.
            </p>
            <div className="mt-4 grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
              {[{ value: '' as const, label: 'Default', hint: 'Balanced explanations' }, ...STYLE_OPTIONS].map((opt) => {
                const active = defaultStyle === opt.value
                const visual = STYLE_VISUALS[opt.value || 'default']
                const Icon = visual.icon
                return (
                  <button
                    key={opt.value || 'default'}
                    type="button"
                    aria-pressed={active}
                    onClick={() => setDefaultStyle(opt.value)}
                    className="text-left"
                  >
                    <CardSpotlight
                      spotlightColor={visual.spotlight}
                      className={cn(
                        'h-full rounded-xl px-4 py-4',
                        active && visual.activeBorderClass,
                      )}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2.5">
                          <span className={cn('flex size-8 items-center justify-center rounded-lg', visual.badgeClass)}>
                            <Icon className="size-4" />
                          </span>
                          <span className="font-semibold">{opt.label}</span>
                        </div>
                        {active && <Check className="size-4 text-foreground" />}
                      </div>
                      <span className="mt-2 block text-xs text-muted-foreground">{opt.hint}</span>
                    </CardSpotlight>
                  </button>
                )
              })}
            </div>
          </fieldset>
        </div>
      </div>
    </div>
  )
}
