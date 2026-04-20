import { useState } from 'react';
import type { CataractaeNote } from '../api/types';
import { formatAge } from '../utils/formatAge';

interface NotesTimelineProps {
  notes: CataractaeNote[];
  loading: boolean;
}

export function NotesTimeline({ notes, loading }: NotesTimelineProps) {
  if (loading) {
    return <div className="text-center py-4 text-cistern-muted font-mono text-sm">Loading notes…</div>;
  }

  if (notes.length === 0) {
    return <div className="text-center py-4 text-cistern-muted font-mono text-sm">No notes yet</div>;
  }

  return (
    <div className="space-y-2">
      {notes.map((note) => (
        <NoteCard key={note.id} note={note} />
      ))}
    </div>
  );
}

function NoteCard({ note }: { note: CataractaeNote }) {
  const [expanded, setExpanded] = useState(false);
  const isLong = note.content.length > 200;
  const displayContent = isLong && !expanded
    ? note.content.slice(0, 200) + '…'
    : note.content;

  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-3">
      <div className="flex items-center gap-2 mb-1">
        <span className="text-xs font-mono font-bold text-cistern-accent">{note.cataractae_name}</span>
        <span className="text-xs text-cistern-muted">{formatAge(note.created_at)}</span>
      </div>
      <div className="text-sm text-cistern-fg whitespace-pre-wrap break-words">
        {displayContent}
      </div>
      {isLong && (
        <button
          type="button"
          onClick={() => setExpanded(!expanded)}
          className="text-xs text-cistern-accent hover:underline mt-1"
        >
          {expanded ? 'Show less' : 'Show more'}
        </button>
      )}
    </div>
  );
}