import { useState, useCallback, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useDroplets, useRepos, useSearchDroplets } from '../hooks/useApi';
import { DropletTable } from '../components/DropletTable';
import { ExportButton } from '../components/ExportButton';
import type { DropletSearchResponse } from '../api/types';

const STATUS_TABS = [
  { key: '', label: 'All' },
  { key: 'open', label: 'Open' },
  { key: 'in_progress', label: 'In Progress' },
  { key: 'pooled', label: 'Pooled' },
  { key: 'delivered', label: 'Delivered' },
  { key: 'cancelled', label: 'Cancelled' },
];

const SORT_OPTIONS = [
  { key: 'priority', label: 'Priority' },
  { key: 'created_at', label: 'Created' },
  { key: 'updated_at', label: 'Updated' },
  { key: 'title', label: 'Title' },
];

export function DropletsList() {
  const navigate = useNavigate();
  const [status, setStatus] = useState('');
  const [repo, setRepo] = useState('');
  const [sort, setSort] = useState('priority');
  const [page, setPage] = useState(1);
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const searchTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const perPage = 50;

  const { repos } = useRepos();
  const { mutate: searchMutate } = useSearchDroplets();

  const [searchResults, setSearchResults] = useState<DropletSearchResponse | null>(null);

  const isSearching = debouncedSearch.length > 0;

  useEffect(() => {
    return () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    };
  }, []);

  const { data, error } = useDroplets({
    status: status || undefined,
    repo: repo || undefined,
    page,
    per_page: perPage,
    sort,
  });

  const handleSearchChange = useCallback((value: string) => {
    setSearchQuery(value);
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    if (!value.trim()) {
      setDebouncedSearch('');
      setSearchResults(null);
      return;
    }
    searchTimerRef.current = setTimeout(async () => {
      setDebouncedSearch(value.trim());
      try {
        const res = await searchMutate(value.trim(), status || undefined);
        setSearchResults(res as never);
      } catch {
        setSearchResults(null);
      }
    }, 300);
  }, [searchMutate, status]);

  const displayedDroplets = isSearching && searchResults
    ? searchResults.droplets
    : data?.droplets ?? [];

  const handleRowClick = (id: string) => {
    navigate(`/app/droplets/${id}`);
  };

  return (
    <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-mono font-bold text-cistern-fg">Droplets</h1>
        <div className="flex items-center gap-2">
          <ExportButton status={status || undefined} repo={repo || undefined} />
          <button
            type="button"
            onClick={() => navigate('/app/droplets/new')}
            className="px-3 py-1.5 text-sm rounded bg-cistern-accent text-cistern-bg font-medium hover:bg-cistern-accent/90 transition-colors"
          >
            + New Droplet
          </button>
        </div>
      </div>

      <div className="flex items-center gap-2 flex-wrap">
        {STATUS_TABS.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => { setStatus(tab.key); setPage(1); }}
            className={`text-xs px-3 py-1.5 rounded-full border transition-colors ${
              status === tab.key
                ? 'border-cistern-accent text-cistern-accent bg-cistern-accent/10'
                : 'border-cistern-border text-cistern-muted hover:text-cistern-fg'
            }`}
          >{tab.label}</button>
        ))}
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex-1 min-w-[200px]">
          <input
            value={searchQuery}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search droplets..."
            className="w-full bg-cistern-surface border border-cistern-border rounded px-3 py-1.5 text-sm text-cistern-fg placeholder-cistern-muted"
          />
        </div>
        <select
          value={repo}
          onChange={(e) => { setRepo(e.target.value); setPage(1); }}
          className="bg-cistern-surface border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
        >
          <option value="">All Repos</option>
          {repos.map((r) => (
            <option key={r.name} value={r.name}>{r.name}</option>
          ))}
        </select>
        <select
          value={sort}
          onChange={(e) => { setSort(e.target.value); setPage(1); }}
          className="bg-cistern-surface border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg"
        >
          {SORT_OPTIONS.map((opt) => (
            <option key={opt.key} value={opt.key}>{opt.label}</option>
          ))}
        </select>
      </div>

      {error && (
        <div className="bg-cistern-red/10 border border-cistern-red/30 rounded p-3 text-sm text-cistern-red font-mono">
          {error.message}
        </div>
      )}

      <div className="bg-cistern-surface border border-cistern-border rounded-lg overflow-hidden">
        <DropletTable droplets={displayedDroplets} onRowClick={handleRowClick} />
      </div>

      {!isSearching && data && data.total > perPage && (
        <div className="flex items-center justify-between text-sm text-cistern-muted font-mono">
          <span>Page {page} of {Math.ceil(data.total / perPage)} ({data.total} total)</span>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
              className="px-3 py-1 rounded border border-cistern-border hover:text-cistern-fg disabled:opacity-30"
            >Prev</button>
            <button
              type="button"
              disabled={page >= Math.ceil(data.total / perPage)}
              onClick={() => setPage((p) => p + 1)}
              className="px-3 py-1 rounded border border-cistern-border hover:text-cistern-fg disabled:opacity-30"
            >Next</button>
          </div>
        </div>
      )}
    </div>
  );
}