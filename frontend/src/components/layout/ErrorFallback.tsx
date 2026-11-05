import { TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'

/** Shown when rendering crashes. It sits outside the router, so it uses plain links. */
export function ErrorFallback({ resetError }: { resetError: () => void }) {
  return (
    <div className="flex min-h-svh items-center justify-center bg-background px-6 text-foreground">
      <div className="max-w-md text-center">
        <span className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-accent text-accent-foreground">
          <TriangleAlert className="size-6" />
        </span>
        <h1 className="mt-4 font-reading text-2xl font-semibold">Something went wrong</h1>
        <p className="mt-2 text-muted-foreground">
          This page hit an unexpected error. Your documents and chats are safe. Try again, or go back to the start.
        </p>
        <div className="mt-6 flex justify-center gap-3">
          <Button onClick={resetError}>Try again</Button>
          <Button variant="outline" asChild>
            <a href="/">Go to the start</a>
          </Button>
        </div>
      </div>
    </div>
  )
}
