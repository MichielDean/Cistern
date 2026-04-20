import { useState, useEffect, useRef, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useRepos, useSearchDroplets, createDroplet } from '../hooks/useApi';
import { ComplexitySelector } from './ComplexitySelector';
import type { Droplet } from '../api/types';

interface CreateDropletFormProps {
  onSuccess: (droplet: Droplet) => void;
  onCancel: () => void;
}

export function CreateDropletForm({ onSuccess, onCancel }: CreateDropletFormProps) {
  const { repos, loading: reposLoading } = useRepos();
  const { mutate: searchDroplets } = useSearchDroplets();
  const [searchParams] = useSearchParams();

  const [repo, setRepo] = useState('');
  const [title, setTitle] = useState(() => searchParams.get('title') ?? '');
  const [description, setDescription] = useState(() => searchParams.get('description') ?? '');
  const [priority, setPriority] = useState(2);
  const [complexity, setComplexity] = useState(1);
  const [dependencies, setDependencies] = useState<string[]>([]);
  const [depSearch, setDepSearch] = useState('');
  const [depResults, setDepResults] = useState<{ id: string; title: string }[]>([]);
  const [depSearchLoading, setDepSearchLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);

  const searchTimer = useRef<ReturnType<typeof setTimeout>>();
  const depWrapperRef = useRef<HTMLDivElement>(null);

  const handleClickOutsideDep = useCallback((e: MouseEvent) => {
    if (depWrapperRef.current && !depWrapperRef.current.contains(e.target as Node)) {
      setDepResults([]);
    }
  }, []);

  useEffect(() => {
    if (depResults.length > 0) {
      document.addEventListener('mousedown', handleClickOutsideDep);
      return () => document.removeEventListener('mousedown', handleClickOutsideDep);
    }
  }, [depResults.length, handleClickOutsideDep]);

  useEffect(() => {
    if (!depSearch.trim()) {
      setDepResults([]);
      return;
    }
    setDepSearchLoading(true);
    clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(async () => {
      try {
        const res = await searchDroplets(depSearch.trim());
        setDepResults(res.droplets.map((d) => ({ id: d.id, title: d.title })));
      } catch {
        setDepResults([]);
      } finally {
        setDepSearchLoading(false);
      }
    }, 300);
    return () => clearTimeout(searchTimer.current);
  }, [depSearch, searchDroplets]);

  const addDep = (id: string) => {
    if (!dependencies.includes(id)) {
      setDependencies([...dependencies, id]);
    }
    setDepSearch('');
    setDepResults([]);
    setDirty(true);
  };

  const removeDep = (id: string) => {
    setDependencies(dependencies.filter((d) => d !== id));
    setDirty(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim() || !repo) return;
    setSubmitting(true);
    setError(null);
    try {
      const droplet = await createDroplet({
        repo,
        title: title.trim(),
        description: description.trim() || undefined,
        priority,
        complexity,
        depends_on: dependencies.length > 0 ? dependencies : undefined,
      });
      onSuccess(droplet);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create droplet');
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    if (dirty) {
      if (!window.confirm('You have unsaved changes. Discard?')) return;
    }
    onCancel();
  };

  const markDirty = () => setDirty(true);

  const titleError = !title.trim() && dirty ? 'Title is required' : null;
  const repoError = !repo && dirty ? 'Repo is required' : null;

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Repo *</label>
        {reposLoading ? (
          <div className="text-sm text-cistern-muted font-mono">Loading repos…</div>
        ) : (
          <select
            value={repo}
            onChange={(e) => { setRepo(e.target.value); markDirty(); }}
            className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
            required
          >
            <option value="">Select repo</option>
            {repos.map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        )}
        {repoError && <div className="text-xs text-cistern-red font-mono mt-1">{repoError}</div>}
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Title *</label>
        <input
          type="text"
          value={title}
          onChange={(e) => { setTitle(e.target.value); markDirty(); }}
          className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
          required
          autoFocus
        />
        {titleError && <div className="text-xs text-cistern-red font-mono mt-1">{titleError}</div>}
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Description</label>
        <textarea
          value={description}
          onChange={(e) => { setDescription(e.target.value); markDirty(); }}
          placeholder="Optional description..."
          className="w-full bg-cistern-bg border border-cistern-border rounded p-2 text-sm text-cistern-fg resize-y min-h-[80px]"
        />
      </div>

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Priority</label>
        <input
          type="number"
          value={priority}
          onChange={(e) => { setPriority(Number(e.target.value)); markDirty(); }}
          min={0}
          className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
        />
      </div>

      <ComplexitySelector value={complexity} onChange={(v) => { setComplexity(v); markDirty(); }} repoName={repo || undefined} />

      <div>
        <label className="block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1">Dependencies</label>
        {dependencies.length > 0 && (
          <div className="flex flex-wrap gap-1 mb-2">
            {dependencies.map((depId) => (
              <span key={depId} className="text-xs px-2 py-1 rounded-full bg-cistern-accent/20 text-cistern-accent font-mono flex items-center gap-1">
                {depId}
                <button type="button" onClick={() => removeDep(depId)} className="hover:text-cistern-red">×</button>
              </span>
            ))}
          </div>
        )}
        <div className="relative" ref={depWrapperRef}>
          <input
            type="text"
            value={depSearch}
            onChange={(e) => setDepSearch(e.target.value)}
            placeholder="Search droplet ID…"
            className="w-full bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg font-mono"
          />
          {depSearchLoading && (
            <div className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-cistern-muted">…</div>
          )}
          {depResults.length > 0 && (
            <div className="absolute z-10 left-0 right-0 top-full mt-1 bg-cistern-surface border border-cistern-border rounded-lg shadow-lg max-h-40 overflow-y-auto">
              {depResults.map((d) => (
                <button
                  key={d.id}
                  type="button"
                  onClick={() => addDep(d.id)}
                  className="w-full text-left px-3 py-2 text-sm hover:bg-cistern-accent/10 transition-colors border-b border-cistern-border/50 last:border-b-0"
                >
                  <span className="font-mono text-cistern-accent">{d.id}</span>
                  <span className="ml-2 text-cistern-muted truncate">{d.title}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {error && <div className="text-sm text-cistern-red font-mono">{error}</div>}

      <div className="flex gap-2 justify-end pt-2">
        <button type="button" onClick={handleCancel} className="px-4 py-2 text-sm rounded border border-cistern-border text-cistern-muted hover:text-cistern-fg">Cancel</button>
        <button
          type="submit"
          disabled={submitting || !title.trim() || !repo}
          className="px-4 py-2 text-sm rounded bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50"
        >
          {submitting ? 'Creating…' : 'Create Droplet'}
        </button>
      </div>
    </form>
  );
}