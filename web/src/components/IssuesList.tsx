import { useState } from 'react';
import type { DropletIssue } from '../api/types';

type IssueFilter = 'all' | 'open' | string;

interface IssuesListProps {
  issues: DropletIssue[];
  loading: boolean;
  onResolve: (issueId: string, evidence: string) => Promise<void>;
  onReject: (issueId: string, evidence: string) => Promise<void>;
}

export function IssuesList({ issues, loading, onResolve, onReject }: IssuesListProps) {
  const [filter, setFilter] = useState<IssueFilter>('open');
  const [modalIssue, setModalIssue] = useState<DropletIssue | null>(null);
  const [modalMode, setModalMode] = useState<'resolve' | 'reject'>('resolve');
  const [evidence, setEvidence] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const flaggedBySet = new Set(issues.map((i) => i.flagged_by));
  const filteredIssues = issues.filter((i) => {
    if (filter === 'all') return true;
    if (filter === 'open') return i.status === 'open';
    return i.flagged_by === filter;
  });

  if (loading) {
    return <div className="text-center py-4 text-cistern-muted font-mono text-sm">Loading issues…</div>;
  }

  const handleSubmit = async () => {
    if (!modalIssue || !evidence.trim()) return;
    setSubmitting(true);
    try {
      if (modalMode === 'resolve') {
        await onResolve(modalIssue.id, evidence);
      } else {
        await onReject(modalIssue.id, evidence);
      }
      setModalIssue(null);
      setEvidence('');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-3 flex-wrap">
        <FilterBtn active={filter === 'all'} onClick={() => setFilter('all')}>All</FilterBtn>
        <FilterBtn active={filter === 'open'} onClick={() => setFilter('open')}>Open</FilterBtn>
        {[...flaggedBySet].map((fb) => (
          <FilterBtn key={fb} active={filter === fb} onClick={() => setFilter(fb)}>{fb}</FilterBtn>
        ))}
      </div>

      {filteredIssues.length === 0 ? (
        <div className="text-center py-4 text-cistern-muted font-mono text-sm">No issues</div>
      ) : (
        <div className="space-y-2">
          {filteredIssues.map((issue) => (
            <div key={issue.id} className="bg-cistern-surface border border-cistern-border rounded-lg p-3">
              <div className="flex items-center gap-2 mb-1">
                <span className={`text-xs px-1.5 py-0.5 rounded-full font-mono font-medium ${
                  issue.status === 'open' ? 'bg-cistern-red/20 text-cistern-red' :
                  issue.status === 'resolved' ? 'bg-cistern-green/20 text-cistern-green' :
                  'bg-cistern-muted/20 text-cistern-muted'
                }`}>{issue.status}</span>
                <span className="text-xs font-mono text-cistern-accent">{issue.flagged_by}</span>
              </div>
              <div className="text-sm text-cistern-fg mb-2">{issue.description}</div>
              {issue.evidence && (
                <div className="text-xs text-cistern-muted mb-2">{issue.evidence}</div>
              )}
              {issue.status === 'open' && (
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => { setModalIssue(issue); setModalMode('resolve'); setEvidence(''); }}
                    className="text-xs px-2 py-1 rounded border border-cistern-green/40 text-cistern-green hover:bg-cistern-green/10 transition-colors"
                  >Resolve</button>
                  <button
                    type="button"
                    onClick={() => { setModalIssue(issue); setModalMode('reject'); setEvidence(''); }}
                    className="text-xs px-2 py-1 rounded border border-cistern-yellow/40 text-cistern-yellow hover:bg-cistern-yellow/10 transition-colors"
                  >Reject</button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {modalIssue && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setModalIssue(null)}>
          <div className="bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4" onClick={(e) => e.stopPropagation()}>
            <h3 className="font-mono text-cistern-fg mb-3">{modalMode === 'resolve' ? 'Resolve' : 'Reject'} Issue</h3>
            <p className="text-sm text-cistern-muted mb-3">{modalIssue.description}</p>
            <textarea
              value={evidence}
              onChange={(e) => setEvidence(e.target.value)}
              placeholder="Evidence..."
              className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[80px] mb-3"
            />
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => setModalIssue(null)}
                className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg"
              >Cancel</button>
              <button
                type="button"
                onClick={handleSubmit}
                disabled={submitting || !evidence.trim()}
                className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50"
              >{submitting ? '…' : modalMode === 'resolve' ? 'Resolve' : 'Reject'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function FilterBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`text-xs px-2 py-1 rounded-full border transition-colors ${
        active ? 'border-cistern-accent text-cistern-accent bg-cistern-accent/10' : 'border-cistern-border text-cistern-muted hover:text-cistern-fg'
      }`}
    >{children}</button>
  );
}