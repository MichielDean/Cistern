import { useState } from 'react';
import type { ActionRequest } from '../api/types';

interface ActionDialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  action: string;
  dropletId: string;
  showNotes?: boolean;
  showTargetSelector?: boolean;
  steps?: string[];
  onConfirm: (dropletId: string, action: string, body?: ActionRequest) => Promise<void>;
}

export function ActionDialog({ open, onClose, title, action, dropletId, showNotes, showTargetSelector, steps, onConfirm }: ActionDialogProps) {
  const [notes, setNotes] = useState('');
  const [target, setTarget] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!open) return null;

  const handleConfirm = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const body: ActionRequest = {};
      if (notes.trim()) body.notes = notes.trim();
      if (target) body.to = target;
      await onConfirm(dropletId, action, Object.keys(body).length > 0 ? body : undefined);
      setNotes('');
      setTarget('');
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="font-mono text-cistern-fg text-lg mb-2">{title}</h3>
        <p className="text-sm text-cistern-muted mb-4">
          Confirm <span className="text-cistern-accent">{action}</span> for droplet <span className="font-mono text-cistern-accent">{dropletId}</span>?
        </p>

        {showTargetSelector && steps && steps.length > 0 && (
          <div className="mb-4">
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Target Step</label>
            <select
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            >
              <option value="">Current step</option>
              {steps.map((step) => (
                <option key={step} value={step}>{step}</option>
              ))}
            </select>
          </div>
        )}

        {showNotes && (
          <div className="mb-4">
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Notes</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Optional notes..."
              className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[80px]"
            />
          </div>
        )}

        {error && (
          <div className="mb-3 text-sm text-cistern-red font-mono">{error}</div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg"
          >Cancel</button>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={submitting}
            className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50"
          >{submitting ? '…' : 'Confirm'}</button>
        </div>
      </div>
    </div>
  );
}