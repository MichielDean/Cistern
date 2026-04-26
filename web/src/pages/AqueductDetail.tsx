import { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAqueductDetail } from '../hooks/useApi';
import { PeekPanel } from '../components/PeekPanel';
import { SkeletonCard, SkeletonLine } from '../components/LoadingSkeleton';
import { formatElapsed } from '../utils/formatElapsed';
import { formatAge } from '../utils/formatAge';
import type { AqueductDetail, AqueductWorkflowStep, CataractaeNote } from '../api/types';

export function AqueductDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { detail, loading, error, refetch } = useAqueductDetail(name ?? null);
  const [peeking, setPeeking] = useState(false);
  const [elapsed, setElapsed] = useState<string>('--');
  const startTimeRef = useRef(Date.now());

  useEffect(() => {
    if (!detail) return;
    startTimeRef.current = Date.now() - detail.elapsed / 1e6;
    setElapsed(formatElapsed(detail.elapsed));
  }, [detail?.elapsed]);

  useEffect(() => {
    if (!detail || detail.status !== 'flowing') return;
    const interval = setInterval(() => {
      const currentElapsed = Date.now() - startTimeRef.current;
      setElapsed(formatElapsed(currentElapsed * 1e6));
    }, 1000);
    return () => clearInterval(interval);
  }, [detail?.status]);

  if (error && !detail) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Aqueduct Not Found</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
          <button
            type="button"
            onClick={() => navigate('/app/')}
            className="mt-4 text-xs px-3 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
          >
            Back to Dashboard
          </button>
        </div>
      </div>
    );
  }

  if (loading || !detail) {
    return (
      <div className="flex-1 p-4 md:p-6 space-y-6">
        <SkeletonLine width="48%" />
        <SkeletonCard lines={6} />
      </div>
    );
  }

  const progressPercent = detail.total_cataractae > 0
    ? (detail.cataractae_index / detail.total_cataractae) * 100
    : 0;

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-6">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="space-y-1 min-w-0">
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="font-mono font-bold text-cistern-fg text-xl">{detail.name}</h1>
            <span className={`text-xs font-mono px-2 py-0.5 rounded-full ${
              detail.status === 'flowing'
                ? 'bg-cistern-green/20 text-cistern-green'
                : 'bg-cistern-muted/20 text-cistern-muted'
            }`}>
              {detail.status}
            </span>
          </div>
          <div className="flex items-center gap-2 text-xs text-cistern-muted font-mono">
            <span className="bg-cistern-border/30 px-1.5 py-0.5 rounded text-cistern-accent">{detail.repo_name}</span>
            {detail.status === 'flowing' && (
              <>
                <span className="text-cistern-border">|</span>
                <span className="text-cistern-fg">{elapsed}</span>
              </>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {detail.status === 'flowing' && (
            <button
              type="button"
              onClick={() => setPeeking(true)}
              className="px-3 py-1.5 text-sm rounded border border-cistern-accent/40 text-cistern-accent hover:bg-cistern-accent/10 transition-colors"
            >
              Peek
            </button>
          )}
          <button
            type="button"
            onClick={() => refetch()}
            className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
          >
            Refresh
          </button>
          <button
            type="button"
            onClick={() => navigate('/app/')}
            className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
          >
            Back
          </button>
        </div>
      </div>

      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
        <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Pipeline</h2>
        <PipelineStepsDetail
          steps={detail.steps}
          currentIndex={detail.cataractae_index - 1}
          isFlowing={detail.status === 'flowing'}
        />
        {progressPercent > 0 && (
          <div className="mt-3">
            <div className="h-1.5 bg-cistern-border rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-500 ${
                  detail.status === 'flowing' ? 'bg-cistern-accent' : 'bg-cistern-muted'
                }`}
                style={{ width: `${progressPercent}%` }}
              />
            </div>
          </div>
        )}
      </section>

      {detail.droplet_id && detail.status === 'flowing' && (
        <section className="bg-cistern-surface border border-cistern-accent/30 rounded-lg p-4">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Current Droplet</h2>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="font-mono text-cistern-green text-sm cursor-pointer hover:underline" onClick={() => navigate(`/app/droplets/${detail.droplet_id}`)}>
                {detail.droplet_id}
              </span>
              <span className="text-cistern-muted mx-1">·</span>
              <span className="text-cistern-fg text-sm">{detail.droplet_title}</span>
            </div>
            {detail.current_step && (
              <div className="text-xs text-cistern-muted font-mono">
                Current step: <span className="text-cistern-accent">{detail.current_step}</span>
              </div>
            )}
          </div>
        </section>
      )}

      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Notes</h2>
          {detail.droplet_id && (
            <button
              type="button"
              onClick={() => navigate(`/app/droplets/${detail.droplet_id}`)}
              className="text-xs px-2 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
            >
              View Droplet
            </button>
          )}
        </div>
        <AqueductNotes notes={detail.notes} />
      </section>

      {peeking && (
        <PeekPanel aqueductName={detail.name} onClose={() => setPeeking(false)} />
      )}
    </div>
  );
}

function PipelineStepsDetail({ steps, currentIndex, isFlowing }: { steps: AqueductWorkflowStep[]; currentIndex: number; isFlowing: boolean }) {
  if (steps.length === 0) {
    return <div className="text-cistern-muted text-sm font-mono">No pipeline steps defined</div>;
  }

  return (
    <div className="space-y-2">
      {steps.map((step, i) => {
        const isCurrent = i === currentIndex && isFlowing;
        const isCompleted = i < currentIndex;
        const isFuture = i > currentIndex;

        return (
          <div
            key={step.name}
            className={`flex items-center gap-3 px-3 py-2 rounded text-sm font-mono ${
              isCurrent
                ? 'water-flow-active text-cistern-bg font-bold'
                : isCompleted
                ? 'bg-cistern-green/20 text-cistern-green'
                : isFuture && isFlowing
                ? 'bg-cistern-accent/10 text-cistern-accent/50'
                : 'bg-cistern-border/30 text-cistern-muted'
            }`}
          >
            <div className="flex items-center gap-2 min-w-0 flex-1">
              <span className="font-bold">{step.name}</span>
              <span className="text-xs opacity-70">({step.type})</span>
              {step.identity && (
                <span className="text-xs opacity-50 truncate">{step.identity}</span>
              )}
            </div>
            {isCurrent && (
              <span className="w-2 h-2 bg-cistern-accent rounded-full animate-ping shrink-0" />
            )}
            {isCompleted && (
              <span className="text-cist-green shrink-0">✓</span>
            )}
          </div>
        );
      })}
    </div>
  );
}

function AqueductNotes({ notes }: { notes: CataractaeNote[] }) {
  if (notes.length === 0) {
    return <div className="text-center py-4 text-cistern-muted font-mono text-sm">No notes yet</div>;
  }

  return (
    <div className="space-y-2">
      {notes.map((note) => (
        <FullNoteCard key={note.id} note={note} />
      ))}
    </div>
  );
}

function FullNoteCard({ note }: { note: CataractaeNote }) {
  return (
    <div className="bg-cistern-bg border border-cistern-border rounded-lg p-3">
      <div className="flex items-center gap-2 mb-1">
        <span className="text-xs font-mono font-bold text-cistern-accent">{note.cataractae_name}</span>
        <span className="text-xs text-cistern-muted">{formatAge(note.created_at)}</span>
      </div>
      <div className="text-sm text-cistern-fg whitespace-pre-wrap break-words">
        {note.content}
      </div>
    </div>
  );
}