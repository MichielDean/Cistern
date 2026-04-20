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

  it('maps server line-numbered response to LogEntry objects', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([
        { line: 996, text: '2026 INFO old' },
        { line: 997, text: '2026 WARN mid' },
        { line: 998, text: '2026 ERROR err' },
        { line: 999, text: '2026 DEBUG dbg' },
        { line: 1000, text: '2026 plain end' },
      ]),
    }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { fetchLogHistory } = await import('../api/logs');
    const entries = await fetchLogHistory(500, 'castellarius');
    expect(entries).toHaveLength(5);
    expect(entries[0]).toEqual({ line: 996, level: 'INFO', text: '2026 INFO old', raw: '2026 INFO old' });
    expect(entries[1]).toEqual({ line: 997, level: 'WARN', text: '2026 WARN mid', raw: '2026 WARN mid' });
    expect(entries[2]).toEqual({ line: 998, level: 'ERROR', text: '2026 ERROR err', raw: '2026 ERROR err' });
    expect(entries[3]).toEqual({ line: 999, level: 'DEBUG', text: '2026 DEBUG dbg', raw: '2026 DEBUG dbg' });
    expect(entries[4]).toEqual({ line: 1000, level: '', text: '2026 plain end', raw: '2026 plain end' });
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
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource, url: string) {
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
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource, url: string) {
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
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource, _url: string) {
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
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource) { return mockEventSource; }));

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

  it('SSE onEntry callback filters entries by lastHistoryLine', async () => {
    const receivedEntries: Array<{ line: number; text: string }> = [];
    const mockEventSource = {
      onmessage: null as ((e: { data: string }) => void) | null,
      onerror: null as (() => void) | null,
      close: vi.fn(),
    };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource) { return mockEventSource; }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    const lastHistoryLine = 10;
    createLogEventSource('castellarius', (entry) => {
      if (entry.line <= lastHistoryLine) return;
      receivedEntries.push({ line: entry.line, text: entry.text });
    }, () => {});

    mockEventSource.onmessage!({ data: '{"line":5,"text":"old line"}' });
    mockEventSource.onmessage!({ data: '{"line":10,"text":"boundary line"}' });
    mockEventSource.onmessage!({ data: '{"line":11,"text":"new line"}' });
    mockEventSource.onmessage!({ data: '{"line":42,"text":"future line"}' });
    expect(receivedEntries).toHaveLength(2);
    expect(receivedEntries[0].line).toBe(11);
    expect(receivedEntries[1].line).toBe(42);
  });

  it('SSE overlap dedup works with absolute line numbers from history', async () => {
    const receivedEntries: Array<{ line: number; text: string }> = [];
    const mockEventSource = {
      onmessage: null as ((e: { data: string }) => void) | null,
      onerror: null as (() => void) | null,
      close: vi.fn(),
    };
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(function(this: EventSource) { return mockEventSource; }));

    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    });

    const { createLogEventSource } = await import('../api/logs');
    const lastHistoryLine = 500;
    createLogEventSource('castellarius', (entry) => {
      if (entry.line <= lastHistoryLine) return;
      receivedEntries.push({ line: entry.line, text: entry.text });
    }, () => {});

    mockEventSource.onmessage!({ data: '{"line":498,"text":"overlap-1"}' });
    mockEventSource.onmessage!({ data: '{"line":499,"text":"overlap-2"}' });
    mockEventSource.onmessage!({ data: '{"line":500,"text":"overlap-3"}' });
    mockEventSource.onmessage!({ data: '{"line":501,"text":"new-1"}' });
    mockEventSource.onmessage!({ data: '{"line":502,"text":"new-2"}' });
    expect(receivedEntries).toHaveLength(2);
    expect(receivedEntries[0].line).toBe(501);
    expect(receivedEntries[1].line).toBe(502);
  });
});