import { useState, useCallback } from 'react';
import { useRepos } from '../hooks/useApi';
import { importIssue, fetchImportPreview } from '../api/import';
import type { Droplet, ImportRequest } from '../api/types';

const COMPLEXITY_OPTIONS = [
  { value: 1, label: 'Standard (1)' },
  { value: 2, label: 'Full (2)' },
  { value: 3, label: 'Critical (3)' },
];

interface ImportFormProps {
  onSuccess: (droplet: Droplet) => void;
}

export function ImportForm({ onSuccess }: ImportFormProps) {
  const { repos, loading: reposLoading } = useRepos();
  const [provider, setProvider] = useState('jira');
  const [key, setKey] = useState('');
  const [repo, setRepo] = useState('');
  const [complexity, setComplexity] = useState(2);
  const [priority, setPriority] = useState(2);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetching, setFetching] = useState(false);

  const handleFetchPreview = useCallback(async () => {
    if (!key.trim()) return;
    setFetching(true);
    setError(null);
    try {
      const preview = await fetchImportPreview(provider, key.trim());
      setTitle(preview.title);
      setDescription(preview.description);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch issue preview');
    } finally {
      setFetching(false);
    }
  }, [provider, key]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim() || !repo) return;
    setSubmitting(true);
    setError(null);
    try {
      const req: ImportRequest = {
        provider,
        key: key.trim(),
        repo,
        complexity,
        priority,
      };
      const droplet = await importIssue(req);
      onSuccess(droplet);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Provider</label>
        <select
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
          className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
        >
          <option value="jira">Jira</option>
        </select>
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Issue Key *</label>
        <div className="flex gap-2">
          <input
            type="text"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="e.g. PROJ-123"
            className="flex-1 bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg font-mono"
            required
          />
          <button
            type="button"
            onClick={handleFetchPreview}
            disabled={fetching || !key.trim()}
            className="px-3 py-1.5 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg disabled:opacity-30"
          >
            {fetching ? 'Fetching…' : 'Fetch'}
          </button>
        </div>
      </div>

      {title && (
        <div>
          <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Title</label>
          <div className="bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg">{title}</div>
        </div>
      )}

      {description && (
        <div>
          <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Description</label>
          <div className="bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg whitespace-pre-wrap max-h-40 overflow-y-auto">{description}</div>
        </div>
      )}

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Repo *</label>
        {reposLoading ? (
          <div className="text-sm text-cistern-muted font-mono">Loading repos…</div>
        ) : (
          <select
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            required
          >
            <option value="">Select repo</option>
            {repos.map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        )}
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Complexity</label>
        <div className="flex gap-2">
          {COMPLEXITY_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => setComplexity(opt.value)}
              className={`px-3 py-1.5 text-xs rounded border font-mono transition-colors ${
                complexity === opt.value
                  ? 'border-cistern-accent text-cistern-accent bg-cistern-accent/10'
                  : 'border-cistern-border text-cistern-muted hover:text-cistern-fg'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Priority</label>
        <input
          type="number"
          value={priority}
          onChange={(e) => setPriority(Number(e.target.value))}
          min={1}
          max={4}
          className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
        />
      </div>

      {error && (
        <div className="bg-cistern-red/10 border border-cistern-red/30 rounded p-3 text-sm text-cistern-red font-mono">
          {error}
        </div>
      )}

      <div className="flex gap-2 justify-end pt-2">
        <button
          type="submit"
          disabled={submitting || !key.trim() || !repo}
          className="px-4 py-2 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50"
        >
          {submitting ? 'Importing…' : 'Import'}
        </button>
      </div>
    </form>
  );
}