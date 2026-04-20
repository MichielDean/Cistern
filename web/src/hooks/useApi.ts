import { useState, useEffect, useCallback, useRef } from 'react';
import { getAuthHeaders } from './useAuth';
import type {
  Droplet,
  DropletListResponse,
  DropletSearchResponse,
  CataractaeNote,
  DropletIssue,
  DropletDependency,
  ActionRequest,
  AddNoteRequest,
  AddIssueRequest,
  ResolveIssueRequest,
  EditDropletRequest,
  CreateDropletRequest,
} from '../api/types';

async function apiFetch<T>(url: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const authHeaders = getAuthHeaders();
  Object.assign(headers, authHeaders);
  const resp = await fetch(url, { ...options, headers: { ...headers, ...options?.headers } });
  if (!resp.ok) {
    const body = await resp.text().catch(() => resp.statusText);
    throw new Error(`API error ${resp.status}: ${body}`);
  }
  if (resp.status === 204) return undefined as T;
  return resp.json();
}

export function useDroplets(filters: {
  status?: string;
  repo?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}) {
  const [data, setData] = useState<DropletListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const params = new URLSearchParams();
    if (filters.status) params.set('status', filters.status);
    if (filters.repo) params.set('repo', filters.repo);
    if (filters.page) params.set('page', String(filters.page));
    if (filters.per_page) params.set('per_page', String(filters.per_page));
    if (filters.sort) params.set('sort', filters.sort);
    const qs = params.toString();
    const url = qs ? `/api/droplets?${qs}` : '/api/droplets';

    let cancelled = false;
    setLoading(true);
    apiFetch<DropletListResponse>(url)
      .then((res) => { if (!cancelled) { setData(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });

    return () => { cancelled = true; };
  }, [filters.status, filters.repo, filters.page, filters.per_page, filters.sort]);

  return { data, loading, error };
}

export function useDroplet(id: string | null) {
  const [droplet, setDroplet] = useState<Droplet | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!id) { setLoading(false); return; }
    let cancelled = false;
    setLoading(true);
    apiFetch<Droplet>(`/api/droplets/${encodeURIComponent(id)}`)
      .then((res) => { if (!cancelled) { setDroplet(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [id]);

  return { droplet, loading, error };
}

export function useDropletNotes(id: string | null) {
  const [notes, setNotes] = useState<CataractaeNote[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    if (!id) { setLoading(false); return; }

    let cancelled = false;
    setLoading(true);
    apiFetch<CataractaeNote[]>(`/api/droplets/${encodeURIComponent(id)}/notes`)
      .then((res) => { if (!cancelled && mountedRef.current) { setNotes(res); setError(null); } })
      .catch((err) => { if (!cancelled && mountedRef.current) { setError(err); } })
      .finally(() => { if (!cancelled && mountedRef.current) setLoading(false); });

    const authParams = new URLSearchParams();
    const apiKey = localStorage.getItem('cistern_api_key');
    if (apiKey) authParams.set('token', apiKey);
    const eventUrl = authParams.toString()
      ? `/api/droplets/${encodeURIComponent(id)}/events?${authParams.toString()}`
      : `/api/droplets/${encodeURIComponent(id)}/events`;

    let retryCount = 0;
    const connect = () => {
      if (esRef.current) esRef.current.close();
      const es = new EventSource(eventUrl);
      es.onmessage = (e) => {
        if (!mountedRef.current) return;
        try {
          const parsed = JSON.parse(e.data);
          if (parsed.type === 'note' && parsed.note) {
            setNotes((prev) => [parsed.note, ...prev]);
          } else if (parsed.type === 'droplet_update' && parsed.droplet) {
            // handled by detail page refetch
          }
        } catch { /* ignore parse errors */ }
      };
      es.onerror = () => {
        if (!mountedRef.current) return;
        es.close();
        const delay = Math.min(1000 * Math.pow(2, retryCount), 30000);
        retryCount++;
        clearTimeout(reconnectTimer.current);
        reconnectTimer.current = setTimeout(() => { if (mountedRef.current) connect(); }, delay);
      };
      esRef.current = es;
    };

    connect();

    return () => {
      cancelled = true;
      mountedRef.current = false;
      clearTimeout(reconnectTimer.current);
      if (esRef.current) { esRef.current.close(); esRef.current = null; }
    };
  }, [id]);

  return { notes, loading, error };
}

export function useDropletIssues(id: string | null, filters?: { open?: boolean; flagged_by?: string }) {
  const [issues, setIssues] = useState<DropletIssue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!id) { setLoading(false); return; }
    const params = new URLSearchParams();
    if (filters?.open !== undefined) params.set('open', String(filters.open));
    if (filters?.flagged_by) params.set('flagged_by', filters.flagged_by);
    const qs = params.toString();
    const url = qs ? `/api/droplets/${encodeURIComponent(id)}/issues?${qs}` : `/api/droplets/${encodeURIComponent(id)}/issues`;

    let cancelled = false;
    setLoading(true);
    apiFetch<DropletIssue[]>(url)
      .then((res) => { if (!cancelled) { setIssues(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [id, filters?.open, filters?.flagged_by]);

  return { issues, loading, error };
}

export function useDropletDependencies(id: string | null) {
  const [dependencies, setDependencies] = useState<DropletDependency[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!id) { setLoading(false); return; }
    let cancelled = false;
    setLoading(true);
    apiFetch<DropletDependency[]>(`/api/droplets/${encodeURIComponent(id)}/dependencies`)
      .then((res) => { if (!cancelled) { setDependencies(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [id]);

  return { dependencies, loading, error };
}

export function useDropletMutation() {
  const mutate = useCallback(async (
    id: string,
    action: string,
    body?: ActionRequest
  ) => {
    return apiFetch<void>(`/api/droplets/${encodeURIComponent(id)}/${action}`, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    });
  }, []);

  return { mutate };
}

export function useRepos() {
  const [repos, setRepos] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    apiFetch<string[]>('/api/repos')
      .then((res) => { if (!cancelled) { setRepos(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, []);

  return { repos, loading, error };
}

export function useSearchDroplets() {
  const mutate = useCallback(async (query: string, status?: string, priority?: number) => {
    const params = new URLSearchParams();
    params.set('query', query);
    if (status) params.set('status', status);
    if (priority !== undefined) params.set('priority', String(priority));
    return apiFetch<DropletSearchResponse>(`/api/droplets/search?${params.toString()}`);
  }, []);

  return { mutate };
}

export async function addNote(id: string, body: AddNoteRequest): Promise<void> {
  return apiFetch<void>(`/api/droplets/${encodeURIComponent(id)}/notes`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function editDroplet(id: string, body: EditDropletRequest): Promise<Droplet> {
  return apiFetch<Droplet>(`/api/droplets/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  });
}

export async function createDroplet(body: CreateDropletRequest): Promise<Droplet> {
  return apiFetch<Droplet>('/api/droplets', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function addIssue(id: string, body: AddIssueRequest): Promise<DropletIssue> {
  return apiFetch<DropletIssue>(`/api/droplets/${encodeURIComponent(id)}/issues`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function resolveIssue(issueId: string, body: ResolveIssueRequest): Promise<void> {
  return apiFetch<void>(`/api/issues/${encodeURIComponent(issueId)}/resolve`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function rejectIssue(issueId: string, body: ResolveIssueRequest): Promise<void> {
  return apiFetch<void>(`/api/issues/${encodeURIComponent(issueId)}/reject`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function addDependency(id: string, dependsOn: string): Promise<void> {
  return apiFetch<void>(`/api/droplets/${encodeURIComponent(id)}/dependencies`, {
    method: 'POST',
    body: JSON.stringify({ depends_on: dependsOn }),
  });
}

export async function removeDependency(id: string, depId: string): Promise<void> {
  return apiFetch<void>(`/api/droplets/${encodeURIComponent(id)}/dependencies/${encodeURIComponent(depId)}`, {
    method: 'DELETE',
  });
}