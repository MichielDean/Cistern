import type { Droplet } from '../api/types';
import { StatusBadge } from './StatusBadge';
import { formatAge } from '../utils/formatAge';

interface DropletTableProps {
  droplets: Droplet[];
  onRowClick: (id: string) => void;
}

export function DropletTable({ droplets, onRowClick }: DropletTableProps) {
  if (droplets.length === 0) {
    return (
      <div className="text-center py-12 text-cistern-muted">
        <div className="text-4xl mb-3 opacity-30">≋</div>
        <div className="font-mono text-sm">No droplets found</div>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr className="border-b border-cistern-border text-left">
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">ID</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Title</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Status</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Step</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Priority</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Complexity</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Age</th>
            <th className="px-3 py-2 text-xs font-mono text-cistern-muted uppercase tracking-wider">Repo</th>
          </tr>
        </thead>
        <tbody>
          {droplets.map((d) => (
            <tr
              key={d.id}
              onClick={() => onRowClick(d.id)}
              className="border-b border-cistern-border/50 hover:bg-cistern-border/20 cursor-pointer transition-colors"
            >
              <td className="px-3 py-2 font-mono text-xs text-cistern-accent">{d.id}</td>
              <td className="px-3 py-2 text-sm text-cistern-fg truncate max-w-[200px]">{d.title}</td>
              <td className="px-3 py-2"><StatusBadge status={d.status} /></td>
              <td className="px-3 py-2 text-xs text-cistern-muted font-mono">{d.current_cataractae || '--'}</td>
              <td className="px-3 py-2 text-xs text-cistern-fg font-mono">{d.priority}</td>
              <td className="px-3 py-2 text-xs text-cistern-fg font-mono">{d.complexity}</td>
              <td className="px-3 py-2 text-xs text-cistern-muted font-mono">{formatAge(d.created_at)}</td>
              <td className="px-3 py-2 text-xs text-cistern-muted font-mono truncate max-w-[120px]">{d.repo}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}