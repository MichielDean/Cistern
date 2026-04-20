import { apiFetch } from './shared';
import type { Droplet, ImportRequest } from './types';

export interface ImportPreview {
  key: string;
  title: string;
  description: string;
  priority: number;
  labels: string[];
  source_url: string;
}

export async function fetchImportPreview(provider: string, key: string): Promise<ImportPreview> {
  const params = new URLSearchParams({ provider, key });
  return apiFetch<ImportPreview>(`/api/import/preview?${params}`);
}

export async function importIssue(req: ImportRequest): Promise<Droplet> {
  return apiFetch<Droplet>('/api/import', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}