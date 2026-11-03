import { Link } from 'react-router-dom'

export default function NotFoundPage() {
  return (
    <div className="mx-auto max-w-md px-6 py-24">
      <h1 className="font-reading text-2xl font-semibold">This page doesn't exist</h1>
      <p className="mt-2 text-muted-foreground">The chat or document may have been deleted.</p>
      <Link to="/" className="mt-6 inline-block font-semibold text-primary underline-offset-4 hover:underline">
        Go to your documents
      </Link>
    </div>
  )
}
