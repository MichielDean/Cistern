import { useState } from 'react';
import type { Droplet } from '../api/types';
import { editDroplet } from '../hooks/useApi';

interface EditMetadataModalProps {
  open: boolean;
  onClose: () => void;
  droplet: Droplet;
  onSaved: () => void;
}

export function EditMetadataModal({ open, onClose, droplet, onSaved }: EditMetadataModalProps) {
  const [title, setTitle] = useState(droplet.title);
  const [priority, setPriority] = useState(droplet.priority);
  const [complexity, setComplexity] = useState(droplet.complexity);
  const [description, setDescription] = useState(droplet.description);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!open) return null;

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await editDroplet(droplet.id, {
        title: title !== droplet.title ? title : undefined,
        priority: priority !== droplet.priority ? priority : undefined,
        complexity: complexity !== droplet.complexity ? complexity : undefined,
        description: description !== droplet.description ? description : undefined,
      });
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="font-mono text-cistern-fg text-lg mb-4">Edit Metadata</h3>

        <div className="space-y-3">
          <div>
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Title</label>
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            />
          </div>
          <div>
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Priority</label>
            <input
              type="number"
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value))}
              className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            />
          </div>
          <div>
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Complexity</label>
            <input
              type="number"
              value={complexity}
              onChange={(e) => setComplexity(Number(e.target.value))}
              className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            />
          </div>
          <div>
            <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Description</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[80px]"
            />
          </div>
        </div>

        {error && <div className="mt-3 text-sm text-cistern-red font-mono">{error}</div>}

        <div className="flex gap-2 justify-end mt-4">
          <button type="button" onClick={onClose} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg">Cancel</button>
          <button type="button" onClick={handleSubmit} disabled={submitting} className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50">{submitting ? '…' : 'Save'}</button>
        </div>
      </div>
    </div>
  );
}