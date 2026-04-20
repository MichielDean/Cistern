interface StatusBadgeProps {
  status: string;
  size?: 'sm' | 'md';
}

const statusMap: Record<string, { bg: string; text: string; label: string }> = {
  open: { bg: 'bg-cistern-yellow/20', text: 'text-cistern-yellow', label: 'Open' },
  in_progress: { bg: 'bg-cistern-accent/20', text: 'text-cistern-accent', label: 'In Progress' },
  done: { bg: 'bg-cistern-green/20', text: 'text-cistern-green', label: 'Done' },
  pooled: { bg: 'bg-cistern-red/20', text: 'text-cistern-red', label: 'Pooled' },
  closed: { bg: 'bg-cistern-muted/20', text: 'text-cistern-muted', label: 'Closed' },
};

export function StatusBadge({ status, size = 'sm' }: StatusBadgeProps) {
  const config = statusMap[status] || { bg: 'bg-cistern-muted/20', text: 'text-cistern-muted', label: status };
  const sizeClasses = size === 'sm' ? 'text-xs px-1.5 py-0.5' : 'text-sm px-2 py-1';

  return (
    <span className={`inline-flex items-center rounded-full font-mono font-medium ${config.bg} ${config.text} ${sizeClasses}`}>
      {config.label}
    </span>
  );
}