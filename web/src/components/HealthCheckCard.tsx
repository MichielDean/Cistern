import type { DoctorCheck } from '../api/types';

interface HealthCheckCardProps {
  check: DoctorCheck;
}

const statusIcons: Record<string, string> = {
  pass: '✓',
  fail: '✕',
  warn: '⚠',
};

const statusColors: Record<string, string> = {
  pass: 'text-cistern-green',
  fail: 'text-cistern-red',
  warn: 'text-cistern-yellow',
};

const borderAccents: Record<string, string> = {
  pass: '',
  fail: 'border-l-2 border-l-cistern-red',
  warn: 'border-l-2 border-l-cistern-yellow',
};

export function HealthCheckCard({ check }: HealthCheckCardProps) {
  return (
    <div className={`bg-cistern-surface border border-cistern-border rounded-lg p-3 ${borderAccents[check.status]}`}>
      <div className="flex items-start gap-2">
        <span className={`text-lg ${statusColors[check.status]}`}>
          {statusIcons[check.status]}
        </span>
        <div className="min-w-0">
          <div className="font-mono text-sm text-cistern-fg">{check.name}</div>
          {check.message && (
            <div className="text-xs text-cistern-muted mt-0.5">{check.message}</div>
          )}
        </div>
      </div>
    </div>
  );
}