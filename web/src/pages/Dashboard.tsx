import { useState, useMemo } from 'react';
import { useDashboard } from '../context/DashboardContext';
import { AqueductArch, DropletRow, PeekPanel } from '../components';
import { SkeletonCard, SkeletonLine } from '../components/LoadingSkeleton';
import type { DashboardData, CataractaeInfo, FlowActivity, Droplet } from '../api/types';

export function Dashboard() {
  const { data, error } = useDashboard();
  const [peekAqueduct, setPeekAqueduct] = useState<string | null>(null);

  const activityMap = useMemo(() => {
    const map: Record<string, FlowActivity> = {};
    if (data) {
      for (const act of data.flow_activities ?? []) {
        map[act.droplet_id] = act;
      }
    }
    return map;
  }, [data]);

  if (error && !data) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-cistern-red text-lg font-mono mb-2">Connection Error</div>
          <div className="text-cistern-muted text-sm">{error.message}</div>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex-1 p-4 md:p-6 space-y-6">
        <SkeletonLine width="40%" />
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <SkeletonCard />
          <SkeletonCard />
          <SkeletonCard />
        </div>
      </div>
    );
  }

  const flowingIds = new Set((data.cataractae ?? []).filter(c => c.droplet_id).map(c => c.droplet_id));

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-6">
      <AqueductSection
        cataractae={data.cataractae ?? []}
        flowingIds={flowingIds}
        activityMap={activityMap}
        onPeek={setPeekAqueduct}
      />

      <SummarySection data={data} />

      {peekAqueduct && (
        <PeekPanel aqueductName={peekAqueduct} onClose={() => setPeekAqueduct(null)} />
      )}
    </div>
  );
}

function AqueductSection({
  cataractae,
  flowingIds,
  activityMap,
  onPeek,
}: {
  cataractae: CataractaeInfo[];
  flowingIds: Set<string>;
  activityMap: Record<string, FlowActivity>;
  onPeek: (name: string) => void;
}) {
  const active = cataractae.filter(c => c.droplet_id && flowingIds.has(c.droplet_id));
  const idle = cataractae.filter(c => !c.droplet_id);

  return (
    <section>
      <h2 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Aqueducts</h2>
      {active.length > 0 && (
        <div className="space-y-3 mb-4">
          {active.map((cat) => (
            <AqueductArch
              key={cat.name}
              cataractae={cat}
              activity={activityMap[cat.droplet_id || '']}
              isFlowing={true}
              onPeek={onPeek}
            />
          ))}
        </div>
      )}
      {idle.length > 0 && (
        <div className="space-y-2">
          {idle.map((cat) => (
            <AqueductArch
              key={cat.name}
              cataractae={cat}
              isFlowing={false}
              onPeek={onPeek}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function SummarySection({
  data,
}: {
  data: DashboardData;
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      <CisternCountCard data={data} />
      <QueueSection items={data.cistern_items ?? []} blockedByMap={data.blocked_by_map ?? {}} />
      <PooledSection items={data.pooled_items ?? []} />
      <UnassignedSection items={data.unassigned_items ?? []} />
      <RecentSection items={data.recent_items ?? []} />
    </div>
  );
}

function CisternCountCard({ data }: { data: DashboardData }) {
  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
      <h3 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">Cistern</h3>
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-sm text-cistern-fg">Flowing</span>
          <span className="font-mono text-cistern-green">{data.flowing_count}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-cistern-fg">Queued</span>
          <span className="font-mono text-cistern-yellow">{data.queued_count}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-cistern-fg">Delivered</span>
          <span className="font-mono text-cistern-accent">{data.done_count}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-cistern-fg">Pooled</span>
          <span className="font-mono text-cistern-red">{(data.pooled_items ?? []).length}</span>
        </div>
      </div>
      <div className="mt-3 pt-3 border-t border-cistern-border">
        <div className="flex items-center gap-2">
          <div className={`w-2 h-2 rounded-full ${data.farm_running ? 'bg-cistern-green' : 'bg-cistern-muted'}`} />
          <span className="text-xs text-cistern-muted">
            Castellarius {data.farm_running ? 'Running' : 'Stopped'}
          </span>
        </div>
      </div>
    </div>
  );
}

function QueueSection({
  items,
  blockedByMap,
}: {
  items: Droplet[];
  blockedByMap: Record<string, string>;
}) {
  if (items.length === 0) return null;
  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
      <h3 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">
        Queue ({items.length})
      </h3>
      <div className="space-y-1 max-h-64 overflow-y-auto">
        {items.map((d) => (
          <DropletRow key={d.id} droplet={d} blockedBy={blockedByMap[d.id]} />
        ))}
      </div>
    </div>
  );
}

function PooledSection({
  items,
}: {
  items: Droplet[];
}) {
  if (items.length === 0) return null;
  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
      <h3 className="text-sm font-mono text-cistern-red uppercase tracking-wider mb-3">
        Pooled ({items.length})
      </h3>
      <div className="space-y-1 max-h-64 overflow-y-auto">
        {items.map((d) => (
          <DropletRow key={d.id} droplet={d} />
        ))}
      </div>
    </div>
  );
}

function UnassignedSection({
  items,
}: {
  items: Droplet[];
}) {
  if (items.length === 0) return null;
  return (
    <div className="bg-cistern-surface border border-cistern-yellow/30 rounded-lg p-4">
      <h3 className="text-sm font-mono text-cistern-yellow uppercase tracking-wider mb-3 flex items-center gap-2">
        <span>&#x26A0;</span> Orphaned ({items.length})
      </h3>
      <div className="space-y-1 max-h-64 overflow-y-auto">
        {items.map((d) => (
          <DropletRow key={d.id} droplet={d} />
        ))}
      </div>
    </div>
  );
}

function RecentSection({
  items,
}: {
  items: Droplet[];
}) {
  if (items.length === 0) return null;
  return (
    <div className="bg-cistern-surface border border-cistern-border rounded-lg p-4">
      <h3 className="text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3">
        Recent Flow
      </h3>
      <div className="space-y-1 max-h-64 overflow-y-auto">
        {items.slice(0, 5).map((d) => (
          <DropletRow key={d.id} droplet={d} />
        ))}
      </div>
    </div>
  );
}