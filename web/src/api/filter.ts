import { apiFetch } from './shared';
import type { FilterSession, FilterNewResponse, FilterResumeResponse, FilterMessage } from './types';

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