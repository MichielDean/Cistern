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

  it('parses JSON SSE events into LogEntry objects', async () => {
    const receivedEntries: Array<{ line: number; level: string; text: string }> = [];
    const mockEventSource = {
      onmessage: null as ((e: { data: string }) => void) | null,
      onerror: null as (() => void) | null,
      close: vi.fn(),
    };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation((_url: string) => {
      return mockEventSource;
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    createLogEventSource('castellarius', (entry) => {
      receivedEntries.push({ line: entry.line, level: entry.level, text: entry.text });
    }, () => {});

    mockEventSource.onmessage!({ data: '{"line":42,"text":"2026-04-19 12:00:01 INFO server started"}' });
    expect(receivedEntries).toHaveLength(1);
    expect(receivedEntries[0].line).toBe(42);
    expect(receivedEntries[0].level).toBe('INFO');
    expect(receivedEntries[0].text).toBe('2026-04-19 12:00:01 INFO server started');
  });

  it('gracefully handles unparseable SSE data', async () => {
    const receivedEntries: Array<{ line: number; text: string }> = [];
    const mockEventSource = {
      onmessage: null as ((e: { data: string }) => void) | null,
      onerror: null as (() => void) | null,
      close: vi.fn(),
    };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(() => mockEventSource));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    createLogEventSource('castellarius', (entry) => {
      receivedEntries.push({ line: entry.line, text: entry.text });
    }, () => {});

    mockEventSource.onmessage!({ data: 'not-json' });
    expect(receivedEntries).toHaveLength(1);
    expect(receivedEntries[0].text).toBe('not-json');
  });
});