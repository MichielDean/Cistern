import { getAuthHeaders } from '../hooks/useAuth';
import type { RepoInfo, SkillInfo } from './types';

export async function fetchRepos(): Promise<RepoInfo[]> {
  const resp = await fetch('/api/repos', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`repos: ${resp.status}`);
  const data = await resp.json();
  if (Array.isArray(data) && data.length > 0 && data[0].aqueducts === undefined) {
    return data.map((r: { name: string; url: string }) => ({
      name: r.name,
      prefix: '',
      url: r.url,
      aqueduct_config: null,
      aqueducts: [],
    }));
  }
  return data as RepoInfo[];
}

export async function fetchSkills(): Promise<SkillInfo[]> {
  const resp = await fetch('/api/skills', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`skills: ${resp.status}`);
  return resp.json();
}