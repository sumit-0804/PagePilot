import { Link } from 'react-router-dom'
import { Compass } from 'lucide-react'
import { PageHeader } from '@/components/layout/PageHeader'
import { BackgroundAmbient } from '@/components/ui/aceternity/background-beams'

export default function NotFoundPage() {
  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <BackgroundAmbient variant="dots" className="opacity-45" />
      <PageHeader />
      <div className="relative z-10 mx-auto max-w-md px-6 py-20 text-center">
        <span className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-accent text-accent-foreground">
          <Compass className="size-6" />
        </span>
        <h1 className="mt-4 font-reading text-2xl font-semibold">Page not found</h1>
        <p className="mt-2 text-muted-foreground">There's nothing at this address.</p>
        <Link to="/" className="mt-6 inline-block font-semibold text-primary underline-offset-4 hover:underline">
          Go to your documents
        </Link>
      </div>
    </div>
  )
}
