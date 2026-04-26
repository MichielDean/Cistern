import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import type { CataractaeInfo, FlowActivity } from '../api/types';
import { formatElapsed } from '../utils/formatElapsed';

interface AqueductArchProps {
  cataractae: CataractaeInfo;
  activity?: FlowActivity;
  isFlowing: boolean;
  onPeek: (name: string) => void;
}

export function AqueductArch({ cataractae, activity, isFlowing, onPeek }: AqueductArchProps) {
  const navigate = useNavigate();
  const [elapsed, setElapsed] = useState(formatElapsed(cataractae.elapsed));
  const startTimeRef = useRef(Date.now() - cataractae.elapsed / 1e6);
  const prevIsFlowing = useRef(isFlowing);

  useEffect(() => {
    startTimeRef.current = Date.now() - cataractae.elapsed / 1e6;
    setElapsed(formatElapsed(cataractae.elapsed));
  }, [cataractae.elapsed]);

  useEffect(() => {
    if (isFlowing && !prevIsFlowing.current) {
      startTimeRef.current = Date.now() - cataractae.elapsed / 1e6;
    }
    prevIsFlowing.current = isFlowing;
  }, [isFlowing, cataractae.elapsed]);

  useEffect(() => {
    if (!isFlowing) return;
    const interval = setInterval(() => {
      const currentElapsed = Date.now() - startTimeRef.current;
      setElapsed(formatElapsed(currentElapsed * 1e6));
    }, 1000);
    return () => clearInterval(interval);
  }, [isFlowing]);

  const progressPercent = cataractae.total_cataractae > 0
    ? (cataractae.cataractae_index / cataractae.total_cataractae) * 100
    : 0;

  return (
    <div
      className={`group border rounded-lg p-4 transition-all cursor-pointer ${
        isFlowing
          ? 'border-cistern-accent/40 bg-cistern-surface/80 hover:border-cistern-accent pulse-glow'
          : 'border-cistern-border bg-cistern-surface/40 opacity-60 hover:opacity-80'
      }`}
      onClick={() => navigate(`/app/aqueducts/${encodeURIComponent(cataractae.name)}`)}
    >
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="font-mono font-bold text-cistern-fg">{cataractae.name}</span>
          <span className="text-xs text-cistern-muted">{cataractae.repo_name}</span>
        </div>
        <div className="flex items-center gap-2">
          {isFlowing && (
            <span className="text-xs font-mono text-cistern-accent">{elapsed}</span>
          )}
          <button
            onClick={(e) => { e.stopPropagation(); onPeek(cataractae.name); }}
            className="text-xs px-2 py-1 rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg hover:border-cistern-muted transition-colors"
          >
            Peek
          </button>
        </div>
      </div>

      {cataractae.droplet_id && isFlowing && (
        <div className="mb-3 text-sm">
          <span className="font-mono text-cistern-green">{cataractae.droplet_id}</span>
          <span className="text-cistern-muted mx-2">·</span>
          <span className="text-cistern-fg">{cataractae.title}</span>
        </div>
      )}

      <PipelineSteps
        steps={cataractae.steps}
        currentIndex={cataractae.cataractae_index - 1}
        isFlowing={isFlowing}
      />

      {progressPercent > 0 && (
        <div className="mt-3">
          <div className="h-1.5 bg-cistern-border rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                isFlowing ? 'bg-cistern-accent' : 'bg-cistern-muted'
              }`}
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>
      )}

      {activity && (activity.recent_notes ?? []).length > 0 && (
        <div className="mt-3 space-y-1">
          {(activity.recent_notes ?? []).slice(0, 3).map((note) => (
            <FlowNote key={note.id} note={note} />
          ))}
        </div>
      )}
    </div>
  );
}

function PipelineSteps({ steps, currentIndex, isFlowing }: { steps: string[]; currentIndex: number; isFlowing: boolean }) {
  return (
    <div className="flex items-center gap-0 overflow-x-auto pb-1">
      {steps.map((step, i) => {
        const isCurrent = i === currentIndex && isFlowing;
        const isCompleted = i < currentIndex;
        const isFuture = i > currentIndex;
        return (
          <div key={step} className="flex items-center shrink-0">
            {i > 0 && (
              <div className={`h-0.5 w-3 ${
                isCompleted ? 'bg-cistern-green' : isCurrent ? 'bg-cistern-accent' : 'bg-cistern-border'
              }`} />
            )}
            <div
              className={`relative px-2 py-1 rounded text-xs font-mono whitespace-nowrap ${
                isCurrent
                  ? 'water-flow-active text-cistern-bg font-bold'
                  : isCompleted
                  ? 'bg-cistern-green/20 text-cistern-green'
                  : isFuture && isFlowing
                  ? 'bg-cistern-accent/10 text-cistern-accent/50'
                  : 'bg-cistern-border/30 text-cistern-muted'
              }`}
            >
              {step}
              {isCurrent && (
                <span className="absolute -top-1 -right-1 w-2 h-2 bg-cistern-accent rounded-full animate-ping" />
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function FlowNote({ note }: { note: { id: number; cataractae_name: string; content: string; created_at: string } }) {
  return (
    <div className="text-xs text-cistern-muted pl-2 border-l border-cistern-border">
      <span className="text-cistern-accent font-mono">{note.cataractae_name}</span>
      <span className="mx-1">·</span>
      <span>{note.content.length > 100 ? note.content.slice(0, 100) + '…' : note.content}</span>
    </div>
  );
}

