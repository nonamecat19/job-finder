import * as Dialog from '@radix-ui/react-dialog';
import { Button } from '../../../components/ui';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

// Shared confirmation gate for destructive actions (delete entry/section,
// FR-011) and the config-reupload overwrite warning (FR-010). Every path
// that can lose resume data routes through this one component so the
// confirmation behavior stays consistent (SC-005).
export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  danger = true,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Dialog.Root open={open} onOpenChange={(next) => !next && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/50" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-xl border border-border bg-surface p-5 shadow-xl shadow-black/40">
          <Dialog.Title className="font-semibold text-fg">{title}</Dialog.Title>
          <Dialog.Description className="mt-1.5 text-sm text-muted">{description}</Dialog.Description>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="secondary" onClick={onCancel}>
              Cancel
            </Button>
            <Button variant={danger ? 'danger' : 'primary'} onClick={onConfirm}>
              {confirmLabel}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
