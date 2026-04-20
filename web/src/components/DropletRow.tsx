import type { Droplet } from '../api/types';
import { StatusBadge } from './StatusBadge';
import { formatAge } from '../utils/formatAge';

interface DropletRowProps {
  droplet: Droplet;
  blockedBy?: string;
}

export function DropletRow({ droplet, blockedBy }: DropletRowProps) {
  const age = formatAge(droplet.created_at);

  return (
    <div
      className="w-full flex items-center gap-3 px-3 py-2 rounded-md hover:bg-cistern-border/20 transition-colors text-left"
    >
      <StatusBadge status={droplet.status} />
      <span className="font-mono text-xs text-cistern-accent">{droplet.id}</span>
      <span className="text-sm text-cistern-fg truncate flex-1">{droplet.title}</span>
      {blockedBy && (
        <span className="text-xs text-cistern-yellow" title={`Blocked by ${blockedBy}`}>
          ⛏ {blockedBy}
        </span>
      )}
      <span className="text-xs text-cistern-muted whitespace-nowrap">{age}</span>
    </div>
  );
}