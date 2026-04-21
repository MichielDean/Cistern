import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiFetch } from '../api/shared';

describe('apiFetch', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.stubEnv('cistern_auth', '');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    globalThis.fetch = originalFetch;
    localStorage.clear();
    document.head.innerHTML = '';
  });

  it('returns parsed JSON on success', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: 1 }),
    });

    const result = await apiFetch<{ id: number }>('/api/test');
    expect(result).toEqual({ id: 1 });
  });

  it('returns undefined for 204 responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const result = await apiFetch<void>('/api/test');
    expect(result).toBeUndefined();
  });

  it('throws on non-401 error responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: () => Promise.resolve('Internal Server Error'),
    });

    await expect(apiFetch('/api/test')).rejects.toThrow('API error 500: Internal Server Error');
  });

  it('truncates long error response bodies', async () => {
    const longBody = 'x'.repeat(300);
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: () => Promise.resolve(longBody),
    });

    await expect(apiFetch('/api/test')).rejects.toThrow(`API error 500: ${'x'.repeat(200)}…`);
  });

  it('dispatches cistern:auth-expired event and clears stored key on 401 when auth is required', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    localStorage.setItem('cistern_api_key', 'old-key');

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
    });

    const handler = vi.fn();
    window.addEventListener('cistern:auth-expired', handler);

    await expect(apiFetch('/api/test')).rejects.toThrow('API error 401');

    expect(handler).toHaveBeenCalled();
    expect(localStorage.getItem('cistern_api_key')).toBeNull();

    window.removeEventListener('cistern:auth-expired', handler);
  });

  it('does not clear stored key on 401 when auth is not required', async () => {
    localStorage.setItem('cistern_api_key', 'old-key');

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
    });

    const handler = vi.fn();
    window.addEventListener('cistern:auth-expired', handler);

    await expect(apiFetch('/api/test')).rejects.toThrow('API error 401');

    expect(handler).not.toHaveBeenCalled();
    expect(localStorage.getItem('cistern_api_key')).toBe('old-key');

    window.removeEventListener('cistern:auth-expired', handler);
  });

  it('dispatches cistern:auth-expired as a CustomEvent', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
    });

    const handler = vi.fn();
    window.addEventListener('cistern:auth-expired', handler);

    await expect(apiFetch('/api/test')).rejects.toThrow();

    expect(handler).toHaveBeenCalledTimes(1);
    const event = handler.mock.calls[0][0];
    expect(event).toBeInstanceOf(CustomEvent);
    expect(event.type).toBe('cistern:auth-expired');

    window.removeEventListener('cistern:auth-expired', handler);
  });
});