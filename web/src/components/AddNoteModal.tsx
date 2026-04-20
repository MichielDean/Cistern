import { useState } from 'react';
import { addNote } from '../hooks/useApi';

interface AddNoteModalProps {
  open: boolean;
  onClose: () => void;
  dropletId: string;
  onSaved: () => void;
}

export function AddNoteModal({ open, onClose, dropletId, onSaved }: AddNoteModalProps) {
  const [content, setContent] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!open) return null;

  const handleSubmit = async () => {
    if (!content.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await addNote(dropletId, { cataractae: 'manual', content: content.trim() });
      setContent('');
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add note');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="font-mono text-cistern-fg text-lg mb-3">Add Note</h3>
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="Note content..."
          className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[100px] mb-3"
          autoFocus
        />
        {error && <div className="mb-3 text-sm text-cistern-red font-mono">{error}</div>}
        <div className="flex gap-2 justify-end">
          <button type="button" onClick={onClose} className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg">Cancel</button>
          <button type="button" onClick={handleSubmit} disabled={submitting || !content.trim()} className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50">{submitting ? '…' : 'Add Note'}</button>
        </div>
      </div>
    </div>
  );
}