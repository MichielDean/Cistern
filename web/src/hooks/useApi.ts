import { useState, useEffect, useCallback, useRef } from 'react';
import { getAuthParams } from './useAuth';
import { apiFetch } from '../api/shared';
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
  AqueductDetail,
} from '../api/types';

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
  const fetchCounter = useRef(0);

  const refetch = useCallback(() => {
    fetchCounter.current += 1;
    const current = fetchCounter.current;
    if (!id) return;
    let cancelled = false;
    setLoading(true);
    apiFetch<Droplet>(`/api/droplets/${encodeURIComponent(id)}`)
      .then((res) => { if (!cancelled && fetchCounter.current === current) { setDroplet(res); setError(null); } })
      .catch((err) => { if (!cancelled && fetchCounter.current === current) { setError(err); } })
      .finally(() => { if (!cancelled && fetchCounter.current === current) setLoading(false); });
  }, [id]);

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

  return { droplet, loading, error, refetch };
}

export function useDropletNotes(id: string | null, onDropletUpdate?: (d: Droplet) => void) {
  const [notes, setNotes] = useState<CataractaeNote[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);
  const callbackRef = useRef(onDropletUpdate);
  callbackRef.current = onDropletUpdate;
  const fetchCounter = useRef(0);

  const refetch = useCallback(() => {
    fetchCounter.current += 1;
    const current = fetchCounter.current;
    if (!id) return;
    let cancelled = false;
    setLoading(true);
    apiFetch<CataractaeNote[]>(`/api/droplets/${encodeURIComponent(id)}/notes`)
      .then((res) => { if (!cancelled && mountedRef.current && fetchCounter.current === current) { setNotes(res); setError(null); } })
      .catch((err) => { if (!cancelled && mountedRef.current && fetchCounter.current === current) { setError(err); } })
      .finally(() => { if (!cancelled && mountedRef.current && fetchCounter.current === current) setLoading(false); });
  }, [id]);

  useEffect(() => {
    mountedRef.current = true;
    if (!id) { setLoading(false); return; }

    let cancelled = false;
    setLoading(true);
    apiFetch<CataractaeNote[]>(`/api/droplets/${encodeURIComponent(id)}/notes`)
      .then((res) => { if (!cancelled && mountedRef.current) { setNotes(res); setError(null); } })
      .catch((err) => { if (!cancelled && mountedRef.current) { setError(err); } })
      .finally(() => { if (!cancelled && mountedRef.current) setLoading(false); });

    const authParams = getAuthParams();
    const eventUrl = authParams
      ? `/api/droplets/${encodeURIComponent(id)}/events?${authParams}`
      : `/api/droplets/${encodeURIComponent(id)}/events`;

    let retryCount = 0;
    let lastUpdatedAt: string | null = null;
    const connect = () => {
      if (esRef.current) esRef.current.close();
      const es = new EventSource(eventUrl);
      es.onmessage = (e) => {
        if (!mountedRef.current) return;
        try {
          const parsed = JSON.parse(e.data);
          if (parsed.id && parsed.updated_at !== lastUpdatedAt) {
            lastUpdatedAt = parsed.updated_at;
            callbackRef.current?.(parsed as Droplet);
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

  return { notes, loading, error, refetch };
}

export function useDropletIssues(id: string | null, filters?: { open?: boolean; flagged_by?: string }) {
  const [issues, setIssues] = useState<DropletIssue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const fetchCounter = useRef(0);

  const refetch = useCallback(() => {
    fetchCounter.current += 1;
    const current = fetchCounter.current;
    if (!id) return;
    const params = new URLSearchParams();
    if (filters?.open !== undefined) params.set('open', String(filters.open));
    if (filters?.flagged_by) params.set('flagged_by', filters.flagged_by);
    const qs = params.toString();
    const url = qs ? `/api/droplets/${encodeURIComponent(id)}/issues?${qs}` : `/api/droplets/${encodeURIComponent(id)}/issues`;
    let cancelled = false;
    setLoading(true);
    apiFetch<DropletIssue[]>(url)
      .then((res) => { if (!cancelled && fetchCounter.current === current) { setIssues(res); setError(null); } })
      .catch((err) => { if (!cancelled && fetchCounter.current === current) { setError(err); } })
      .finally(() => { if (!cancelled && fetchCounter.current === current) setLoading(false); });
  }, [id, filters?.open, filters?.flagged_by]);

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

  return { issues, loading, error, refetch };
}

export function useDropletDependencies(id: string | null) {
  const [dependencies, setDependencies] = useState<DropletDependency[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const fetchCounter = useRef(0);

  const refetch = useCallback(() => {
    fetchCounter.current += 1;
    const current = fetchCounter.current;
    if (!id) return;
    let cancelled = false;
    setLoading(true);
    apiFetch<DropletDependency[]>(`/api/droplets/${encodeURIComponent(id)}/dependencies`)
      .then((res) => { if (!cancelled && fetchCounter.current === current) { setDependencies(res); setError(null); } })
      .catch((err) => { if (!cancelled && fetchCounter.current === current) { setError(err); } })
      .finally(() => { if (!cancelled && fetchCounter.current === current) setLoading(false); });
  }, [id]);

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

  return { dependencies, loading, error, refetch };
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

export interface RepoInfo {
  name: string;
  url: string;
}

export function useRepos() {
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    apiFetch<RepoInfo[]>('/api/repos')
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

export async function renameDroplet(id: string, title: string): Promise<Droplet> {
  return apiFetch<Droplet>(`/api/droplets/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    body: JSON.stringify({ title }),
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

export function useRepoSteps(repoName: string | null) {
  const [steps, setSteps] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!repoName) { setLoading(false); return; }
    let cancelled = false;
    setLoading(true);
    apiFetch<string[]>(`/api/repos/${encodeURIComponent(repoName)}/steps`)
      .then((res) => { if (!cancelled) { setSteps(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [repoName]);

  return { steps, loading, error };
}

export function useAqueductDetail(name: string | null) {
  const [detail, setDetail] = useState<AqueductDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const fetchCounter = useRef(0);

  const refetch = useCallback(() => {
    fetchCounter.current += 1;
    const current = fetchCounter.current;
    if (!name) return;
    let cancelled = false;
    setLoading(true);
    apiFetch<AqueductDetail>(`/api/aqueducts/${encodeURIComponent(name)}`)
      .then((res) => { if (!cancelled && fetchCounter.current === current) { setDetail(res); setError(null); } })
      .catch((err) => { if (!cancelled && fetchCounter.current === current) { setError(err); } })
      .finally(() => { if (!cancelled && fetchCounter.current === current) setLoading(false); });
  }, [name]);

  useEffect(() => {
    if (!name) { setLoading(false); return; }
    let cancelled = false;
    setLoading(true);
    apiFetch<AqueductDetail>(`/api/aqueducts/${encodeURIComponent(name)}`)
      .then((res) => { if (!cancelled) { setDetail(res); setError(null); } })
      .catch((err) => { if (!cancelled) { setError(err); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [name]);

  return { detail, loading, error, refetch };
}