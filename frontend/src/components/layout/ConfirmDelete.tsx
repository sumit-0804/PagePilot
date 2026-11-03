import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

interface Target {
  kind: 'document' | 'chat'
  name: string
}

export function ConfirmDelete({ target, onCancel, onConfirm }: { target: Target | null; onCancel: () => void; onConfirm: () => void }) {
  const isDoc = target?.kind === 'document'
  return (
    <AlertDialog open={!!target} onOpenChange={(open) => !open && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{isDoc ? 'Delete this document?' : 'Delete this chat?'}</AlertDialogTitle>
          <AlertDialogDescription>
            {isDoc
              ? `"${target?.name}" and every chat, quiz and flashcard set made from it will be deleted. This can't be undone.`
              : `"${target?.name}" will be deleted. Quizzes and flashcards from it stay in your progress.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Keep it</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            {isDoc ? 'Delete document' : 'Delete chat'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
