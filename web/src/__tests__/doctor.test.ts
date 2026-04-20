import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('fetchDoctor', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('transforms stub response with config_ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ config_ok: true, repos: [{ name: 'cistern', url: 'https://github.com/org/cistern' }] }),
    }));
    const { fetchDoctor } = await import('../api/doctor');
    const result = await fetchDoctor();
    expect(result.checks.length).toBeGreaterThanOrEqual(1);
    expect(result.checks[0].name).toBe('Config Valid');
    expect(result.checks[0].status).toBe('pass');
    expect(result.summary.total).toBe(result.checks.length);
    expect(result.summary.passed).toBe(result.checks.length);
    expect(result.timestamp).toBeTruthy();
  });

  it('transforms stub response with config_ok=false', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ config_ok: false, repos: [] }),
    }));
    const { fetchDoctor } = await import('../api/doctor');
    const result = await fetchDoctor();
    const configCheck = result.checks.find(c => c.name === 'Config Valid');
    expect(configCheck).toBeDefined();
    expect(configCheck!.status).toBe('fail');
  });

  it('passes through full DoctorResult response', async () => {
    const fullResult = {
      checks: [
        { name: 'Daemon running', status: 'pass', message: 'Running', category: 'Daemon' },
      ],
      summary: { total: 1, passed: 1 },
      timestamp: '2026-04-19T00:00:00Z',
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(fullResult),
    }));
    const { fetchDoctor } = await import('../api/doctor');
    const result = await fetchDoctor();
    expect(result.checks).toHaveLength(1);
    expect(result.timestamp).toBe('2026-04-19T00:00:00Z');
  });

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    }));
    const { fetchDoctor } = await import('../api/doctor');
    await expect(fetchDoctor()).rejects.toThrow('doctor: 500');
  });
});