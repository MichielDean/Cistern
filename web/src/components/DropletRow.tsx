import { useNavigate } from 'react-router-dom';
import type { Droplet } from '../api/types';
import { StatusBadge } from './StatusBadge';
import { formatAge } from '../utils/formatAge';

interface DropletRowProps {
  droplet: Droplet;
  blockedBy?: string;
  onClick?: () => void;
  expanded?: boolean;
}

export function DropletRow({ droplet, blockedBy, onClick, expanded }: DropletRowProps) {
  const age = formatAge(droplet.created_at);
  const navigate = useNavigate();

  return (
    <div
      onClick={onClick}
      className={`w-full flex items-center gap-3 px-3 py-2 rounded-md transition-colors text-left ${onClick ? 'hover:bg-cistern-border/20 cursor-pointer' : 'hover:bg-cistern-border/20'}`}
    >
      {onClick && <span className="text-xs text-cistern-muted">{expanded ? '▾' : '▸'}</span>}
      <StatusBadge status={droplet.status} />
      <span
        className="font-mono text-xs text-cistern-accent hover:underline cursor-pointer"
        onClick={(e) => { e.stopPropagation(); navigate(`/app/droplets/${droplet.id}`); }}
      >{droplet.id}</span>
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