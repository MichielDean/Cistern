import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('fetchCastellariusStatus', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('transforms stub response into CastellariusStatus', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ status: 'ok' }),
    }));
    const { fetchCastellariusStatus } = await import('../api/castellarius');
    const result = await fetchCastellariusStatus();
    expect(result.running).toBe(true);
    expect(result.aqueducts).toEqual([]);
    expect(result.castellarius_running).toBe(false);
  });

  it('passes through full response', async () => {
    const fullStatus = {
      running: true,
      pid: 123,
      uptime_seconds: 3600,
      aqueducts: [{ name: 'default', status: 'flowing', droplet_id: 'ci-abc', droplet_title: 'Test', current_step: 'implement', elapsed: 300000000000 }],
      castellarius_running: true,
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(fullStatus),
    }));
    const { fetchCastellariusStatus } = await import('../api/castellarius');
    const result = await fetchCastellariusStatus();
    expect(result.running).toBe(true);
    expect(result.pid).toBe(123);
    expect(result.aqueducts).toHaveLength(1);
  });

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    }));
    const { fetchCastellariusStatus } = await import('../api/castellarius');
    await expect(fetchCastellariusStatus()).rejects.toThrow('castellarius status: 500');
  });
});

describe('castellariusAction', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('calls POST /api/castellarius/{action}', async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal('fetch', mockFetch);
    const { castellariusAction } = await import('../api/castellarius');
    await castellariusAction('start');
    expect(mockFetch).toHaveBeenCalledWith('/api/castellarius/start', {
      method: 'POST',
      headers: {},
    });
  });

  it('throws with error message on failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 501,
      json: () => Promise.resolve({ message: 'not yet supported' }),
    }));
    const { castellariusAction } = await import('../api/castellarius');
    await expect(castellariusAction('stop')).rejects.toThrow('not yet supported');
  });
});