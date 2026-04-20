interface StatusIndicatorProps {
  status: 'running' | 'stopped' | 'error';
  label: string;
  size?: 'sm' | 'lg';
}

const sizeClasses = {
  sm: { dot: 'w-2 h-2', text: 'text-sm' },
  lg: { dot: 'w-4 h-4', text: 'text-lg' },
};

const statusClasses: Record<string, string> = {
  running: 'bg-cistern-green',
  stopped: 'bg-cistern-muted',
  error: 'bg-cistern-red',
};

const labelClasses: Record<string, string> = {
  running: 'text-cistern-green',
  stopped: 'text-cistern-muted',
  error: 'text-cistern-red',
};

export function StatusIndicator({ status, label, size = 'sm' }: StatusIndicatorProps) {
  const s = sizeClasses[size];
  return (
    <div className="flex items-center gap-2">
      <div
        className={`${s.dot} rounded-full ${statusClasses[status]} ${status === 'running' ? 'pulse-glow' : ''}`}
      />
      <span className={`font-mono ${s.text} ${labelClasses[status]}`}>{label}</span>
    </div>
  );
}