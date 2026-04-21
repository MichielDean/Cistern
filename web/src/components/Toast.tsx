export { ToastProvider, useToast } from '../context/ToastContext';
export type { Toast } from '../context/ToastContext';

import { useToast } from '../context/ToastContext';
import type { Toast } from '../context/ToastContext';

const typeStyles: Record<Toast['type'], string> = {
  success: 'bg-cistern-green/20 border-cistern-green/40 text-cistern-green',
  error: 'bg-cistern-red/20 border-cistern-red/40 text-cistern-red',
  info: 'bg-cistern-accent/20 border-cistern-accent/40 text-cistern-accent',
};

export function ToastOutlet() {
  const { toasts, removeToast } = useToast();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-[100] space-y-2 max-w-sm" role="status" aria-live="polite">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`flex items-center gap-2 px-4 py-3 rounded-lg border text-sm font-mono ${typeStyles[toast.type]}`}
        >
          <span className="flex-1">{toast.message}</span>
          <button
            onClick={() => removeToast(toast.id)}
            className="opacity-60 hover:opacity-100 transition-opacity text-lg leading-none"
            aria-label="Dismiss"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}