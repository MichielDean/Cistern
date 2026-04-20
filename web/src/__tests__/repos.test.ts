import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('fetchRepos', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('transforms stub repos without aqueducts', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([{ name: 'cistern', url: 'https://github.com/org/cistern' }]),
    }));
    const { fetchRepos } = await import('../api/repos');
    const result = await fetchRepos();
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe('cistern');
    expect(result[0].url).toBe('https://github.com/org/cistern');
    expect(result[0].aqueducts).toEqual([]);
  });

  it('passes through full repos', async () => {
    const repos = [{
      name: 'cistern',
      prefix: 'ci',
      url: 'https://github.com/org/cistern',
      aqueduct_config: '/path/to/config',
      aqueducts: [{ name: 'default', steps: ['flag', 'implement'] }],
    }];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(repos),
    }));
    const { fetchRepos } = await import('../api/repos');
    const result = await fetchRepos();
    expect(result[0].aqueducts).toHaveLength(1);
    expect(result[0].prefix).toBe('ci');
  });
});

describe('fetchSkills', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('returns skills from API', async () => {
    const skills = [
      { name: 'cistern-git', source_url: '/skills/cistern-git/SKILL.md', installed_at: '2026-01-10T00:00:00Z' },
    ];
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(skills),
    }));
    const { fetchSkills } = await import('../api/repos');
    const result = await fetchSkills();
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe('cistern-git');
  });

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    }));
    const { fetchSkills } = await import('../api/repos');
    await expect(fetchSkills()).rejects.toThrow('skills: 500');
  });
});