import { Link } from 'react-router-dom'
import { motion } from 'motion/react'
import { ArrowRight, BookOpenCheck, BrainCircuit, Clock, Globe, Layers, MessageSquare, Sparkles } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { useConversations, useDocuments } from '@/hooks/queries'
import { displayName, isProcessing, sectionNoun, statusLabel, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Spotlight, CardSpotlight } from '@/components/ui/aceternity/spotlight'
import { TextGenerateEffect } from '@/components/ui/aceternity/text-generate-effect'
import { ChatView, type SendFn } from '@/components/chat/ChatView'
import { SuggestedPrompts } from '@/components/chat/Suggestions'
import { FormatIcon } from '@/components/documents/FormatIcon'

const FEATURES = [
  { icon: BookOpenCheck, label: 'Cited sources' },
  { icon: Globe, label: 'Web search' },
  { icon: BrainCircuit, label: 'Adaptive quizzes' },
  { icon: Layers, label: '3D flashcards' },
]

export default function HomePage() {
  return <ChatView emptyState={HomeHero} />
}

function HomeHero({ onPick }: { onPick: SendFn }) {
  const { isAccount } = useAuth()
  return (
    <div className="relative">
      <div className="pointer-events-none absolute -inset-x-6 -top-8 bottom-0 overflow-hidden" aria-hidden>
        <Spotlight className="-top-28 left-0 md:-top-20 md:left-48" />
      </div>
      <div className="relative mx-auto max-w-4xl pt-4 text-center sm:pt-8">
        <motion.div
          initial={{ opacity: 0, y: -8 }}
          animate={{ opacity: 1, y: 0 }}
          className="mb-5 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-accent/80 px-3.5 py-1 text-xs font-semibold text-accent-foreground shadow-2xs"
        >
          <Sparkles className="size-3.5 text-primary" />
          <span>Your AI study partner</span>
        </motion.div>

        <h1 className="font-reading text-4xl leading-tight font-semibold tracking-tight sm:text-5xl">
          <TextGenerateEffect words="What do you want to learn today?" />
        </h1>

        <motion.p
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.25 }}
          className="mx-auto mt-4 max-w-xl text-base leading-relaxed text-muted-foreground sm:text-lg"
        >
          Ask anything, attach your notes and slides, and get answers that cite where they came from. Then quiz yourself or
          make flashcards.
        </motion.p>

        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.35 }}
          className="mt-6 flex flex-wrap items-center justify-center gap-2.5"
        >
          {FEATURES.map(({ icon: Icon, label }) => (
            <span
              key={label}
              className="inline-flex items-center gap-1.5 rounded-full border border-border/80 bg-card/75 px-3 py-1 text-xs font-medium text-muted-foreground backdrop-blur-xs"
            >
              <Icon className="size-3.5 text-primary" />
              {label}
            </span>
          ))}
        </motion.div>
      </div>

      <div className="relative mt-10">
        <SuggestedPrompts onPick={(text) => onPick(text)} />
      </div>

      {isAccount && <RecentWork />}
    </div>
  )
}

/** Recent chats and documents, for signed-in accounts. */
function RecentWork() {
  const documents = useDocuments()
  const conversations = useConversations()
  const recentChats = conversations.data?.slice(0, 6) ?? []
  const recentDocs = documents.data?.slice(0, 6) ?? []

  return (
    <div className="relative mt-10 space-y-10">
      {recentChats.length > 0 && (
        <section aria-labelledby="recent-chats">
          <h2 id="recent-chats" className="mb-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
            Recent chats
          </h2>
          <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {recentChats.map((c, idx) => (
              <motion.li key={c.id} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: idx * 0.04 }}>
                <Link to={`/chat/${c.id}`} className="block h-full">
                  <CardSpotlight className="h-full p-4">
                    <div className="flex items-start justify-between gap-3">
                      <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-xl bg-accent text-accent-foreground">
                        <MessageSquare className="size-4" />
                      </span>
                      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                        <Clock className="size-3" />
                        {timeAgo(c.updated_at)}
                      </span>
                    </div>
                    <p className="mt-3 truncate font-semibold text-foreground transition-colors group-hover/spotlight:text-primary">
                      {c.title}
                    </p>
                    {c.documents && c.documents.length > 0 && (
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">
                        {c.documents.map((d) => displayName(d.filename)).join(', ')}
                      </p>
                    )}
                  </CardSpotlight>
                </Link>
              </motion.li>
            ))}
          </ul>
        </section>
      )}

      {recentDocs.length > 0 && (
        <section aria-labelledby="home-docs">
          <h2 id="home-docs" className="mb-3 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
            Your documents
          </h2>
          <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {recentDocs.map((d, idx) => {
              const failed = d.status === 'FAILED'
              return (
                <motion.li key={d.id} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: idx * 0.04 }}>
                  <Link to={`/documents/${d.id}`} className="block h-full">
                    <CardSpotlight className="h-full p-4">
                      <div className="flex items-center gap-3">
                        <span
                          className={cn(
                            'flex size-10 shrink-0 items-center justify-center rounded-xl',
                            failed ? 'bg-destructive/15 text-destructive' : 'bg-secondary text-primary',
                          )}
                        >
                          <FormatIcon format={d.format} className="size-5" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <p className="truncate font-semibold text-foreground transition-colors group-hover/spotlight:text-primary">
                            {displayName(d.filename)}
                          </p>
                          <p className={cn('mt-0.5 text-xs', failed ? 'text-destructive' : 'text-muted-foreground')}>
                            {isProcessing(d.status) || failed
                              ? statusLabel(d)
                              : `${d.page_count} ${sectionNoun(d.format, d.page_count)} · Ready`}
                          </p>
                        </div>
                        <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover/spotlight:translate-x-1 group-hover/spotlight:text-primary" />
                      </div>
                    </CardSpotlight>
                  </Link>
                </motion.li>
              )
            })}
          </ul>
        </section>
      )}

      {documents.error && (
        <p role="alert" className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          {documents.error.message}
        </p>
      )}
    </div>
  )
}
