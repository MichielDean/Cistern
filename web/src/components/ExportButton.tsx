import { useState } from 'react';


interface ExportButtonProps {
  status?: string;
  repo?: string;
  priority?: number;
}

function getApiKey(): string | null {
  try {
    return localStorage.getItem('cistern_api_key');
  } catch {
    return null;
  }
}

export function ExportButton({ status, repo, priority }: ExportButtonProps) {
  const [open, setOpen] = useState(false);

  const buildUrl = (fmt: string) => {
    const params = new URLSearchParams();
    params.set('format', fmt);
    if (status) params.set('status', status);
    if (repo) params.set('repo', repo);
    if (priority !== undefined && priority > 0) params.set('priority', String(priority));
    const apiKey = getApiKey();
    if (apiKey) params.set('token', apiKey);
    const qs = params.toString();
    return `/api/droplets/export?${qs}`;
  };

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg transition-colors"
      >
        Export
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 bg-cistern-surface border border-cistern-border rounded-lg shadow-lg z-10 min-w-[160px]">
          <div className="p-2 space-y-1">
            <button
              type="button"
              onClick={() => { window.open(buildUrl('json'), '_blank'); setOpen(false); }}
              className="w-full text-left px-3 py-2 text-sm text-cistern-fg hover:bg-cistern-accent/10 rounded transition-colors"
            >
              JSON
            </button>
            <button
              type="button"
              onClick={() => { window.open(buildUrl('csv'), '_blank'); setOpen(false); }}
              className="w-full text-left px-3 py-2 text-sm text-cistern-fg hover:bg-cistern-accent/10 rounded transition-colors"
            >
              CSV
            </button>
          </div>
        </div>
      )}
    </div>
  );
}