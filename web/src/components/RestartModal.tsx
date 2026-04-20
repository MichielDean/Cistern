import { useState } from 'react';
import { useDropletMutation } from '../hooks/useApi';

interface RestartModalProps {
  open: boolean;
  onClose: () => void;
  dropletId: string;
  steps: string[];
  onRestarted: () => void;
}

export function RestartModal({ open, onClose, dropletId, steps, onRestarted }: RestartModalProps) {
  const [selectedStep, setSelectedStep] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { mutate } = useDropletMutation();

  if (!open) return null;

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const body = selectedStep ? { to: selectedStep } : undefined;
      await mutate(dropletId, 'restart', body);
      onRestarted();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to restart');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="font-mono text-cistern-fg text-lg mb-3">Restart Droplet</h3>
        <p className="text-sm text-cistern-muted mb-4">
          Select which step to restart from, or leave default to restart from the beginning.
        </p>
        {steps.length > 0 && (
          <div className="mb-4">
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Cataractae Step</label>
            <select
              value={selectedStep}
              onChange={(e) => setSelectedStep(e.target.value)}
              className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            >
              <option value="">Default (first step)</option>
              {steps.map((step) => (
                <option key={step} value={step}>{step}</option>
              ))}
            </select>
          </div>
        )}
        {error && <div className="mb-3 text-sm text-cistern-red font-mono">{error}</div>}
        <div className="flex gap-2 justify-end">
          <button type="button" onClick={onClose} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg">Cancel</button>
          <button type="button" onClick={handleSubmit} disabled={submitting} className="px-3 py-1.5 text-sm rounded bg-cistern-yellow text-cistern-bg font-medium disabled:opacity-50">{submitting ? '…' : 'Restart'}</button>
        </div>
      </div>
    </div>
  );
}