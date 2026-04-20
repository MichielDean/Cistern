import { getAuthHeaders } from '../hooks/useAuth';
import type { FilterSession, FilterNewResponse, FilterResumeResponse, FilterMessage } from './types';

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

export async function createFilterSession(title: string, description?: string): Promise<FilterNewResponse> {
  return apiFetch<FilterNewResponse>('/api/filter/new', {
    method: 'POST',
    body: JSON.stringify({ title, description: description || '' }),
  });
}

export async function resumeFilterSession(sessionId: string, message: string): Promise<FilterResumeResponse> {
  return apiFetch<FilterResumeResponse>(`/api/filter/${encodeURIComponent(sessionId)}/resume`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  });
}

export async function listFilterSessions(): Promise<FilterSession[]> {
  return apiFetch<FilterSession[]>('/api/filter/sessions');
}

export async function getFilterSession(sessionId: string): Promise<FilterSession> {
  return apiFetch<FilterSession>(`/api/filter/${encodeURIComponent(sessionId)}`);
}

export function parseFilterMessages(messagesJson: string): FilterMessage[] {
  if (!messagesJson || messagesJson === '[]') return [];
  try {
    return JSON.parse(messagesJson) as FilterMessage[];
  } catch {
    return [];
  }
}