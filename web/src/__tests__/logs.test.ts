import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('fetchLogHistory', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('encodes source parameter in URL', async () => {
    const calls: string[] = [];
    vi.stubGlobal('fetch', vi.fn().mockImplementation((url: string) => {
      calls.push(url);
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve([]),
      });
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { fetchLogHistory } = await import('../api/logs');
    await fetchLogHistory(100, 'my-app');
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain('source=my-app');
    expect(calls[0]).toContain('lines=100');
  });

  it('encodes special characters in source', async () => {
    const calls: string[] = [];
    vi.stubGlobal('fetch', vi.fn().mockImplementation((url: string) => {
      calls.push(url);
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve([]),
      });
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { fetchLogHistory } = await import('../api/logs');
    await fetchLogHistory(50, 'app&evil=injection');
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain('source=app%26evil%3Dinjection');
    expect(calls[0]).not.toContain('source=app&evil');
  });

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { fetchLogHistory } = await import('../api/logs');
    await expect(fetchLogHistory()).rejects.toThrow('logs: 500');
  });
});

describe('createLogEventSource', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('encodes source parameter in SSE URL', async () => {
    const calls: string[] = [];
    const mockEventSource = { onmessage: null as (() => void) | null, onerror: null as (() => void) | null, close: vi.fn() };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation((url: string) => {
      calls.push(url);
      return mockEventSource;
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    createLogEventSource('my-app', () => {}, () => {});
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain('source=my-app');
  });

  it('encodes special characters in SSE source', async () => {
    const calls: string[] = [];
    const mockEventSource = { onmessage: null as (() => void) | null, onerror: null as (() => void) | null, close: vi.fn() };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation((url: string) => {
      calls.push(url);
      return mockEventSource;
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    createLogEventSource('app&evil=injection', () => {}, () => {});
    expect(calls).toHaveLength(1);
    expect(calls[0]).toContain('source=app%26evil%3Dinjection');
    expect(calls[0]).not.toContain('source=app&evil');
  });
});