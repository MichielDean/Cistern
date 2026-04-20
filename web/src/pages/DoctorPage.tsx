import { useDoctor } from '../api/doctor';
import type { DoctorCheck } from '../api/types';
import { HealthCheckCard } from '../components/HealthCheckCard';
import { ActionButton } from '../components/ActionButton';

export function DoctorPage() {
  const { result, loading, error, rerun, fix } = useDoctor();

  if (error && !result) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Error</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  if (loading && !result) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-cistern-muted font-mono">Running health checks…</div>
      </div>
    );
  }

  if (!result) return null;

  const categories = groupByCategory(result.checks);
  const summaryColor = result.summary.passed === result.summary.total
    ? 'text-cistern-green'
    : result.checks.some(c => c.status === 'fail')
    ? 'text-cistern-red'
    : 'text-cistern-yellow';

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-6">
      <section className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider">Health Checks</h2>
          <div className="flex gap-2">
            <ActionButton label="Re-run" onClick={rerun} />
            <ActionButton label="Fix" onClick={fix} variant="primary" />
          </div>
        </div>
        <div className={`text-lg font-mono font-bold ${summaryColor}`}>
          {result.summary.passed}/{result.summary.total} passed
        </div>
        {result.timestamp && (
          <div className="text-xs text-cistern-muted font-mono mt-1">
            Last checked: {new Date(result.timestamp).toLocaleString()}
          </div>
        )}
      </section>

      {Object.entries(categories).map(([category, checks]) => (
        <section key={category}>
          <h3 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-2">{category}</h3>
          <div className="space-y-2">
            {checks.map((check) => (
              <HealthCheckCard key={check.name} check={check} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function groupByCategory(checks: DoctorCheck[]): Record<string, DoctorCheck[]> {
  const groups: Record<string, DoctorCheck[]> = {};
  for (const check of checks) {
    const cat = check.category || 'General';
    if (!groups[cat]) groups[cat] = [];
    groups[cat].push(check);
  }
  return groups;
}