import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useAuth, getAuthHeaders, getAuthParams } from '../hooks/useAuth';

describe('useAuth', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.head.innerHTML = '';
  });

  it('returns authenticated=true when no auth required', () => {
    const { result } = renderHook(() => useAuth());
    expect(result.current.required).toBe(false);
    expect(result.current.authenticated).toBe(true);
  });

  it('returns required=true when auth meta tag is present', () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    const { result } = renderHook(() => useAuth());
    expect(result.current.required).toBe(true);
  });

  it('stores and retrieves API key via login', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    const fetchSpy = vi.spyOn(window, 'fetch').mockResolvedValue({
      ok: true,
    } as Response);

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      result.current.login('test-api-key');
    });

    expect(localStorage.getItem('cistern_api_key')).toBe('test-api-key');
    expect(fetchSpy).toHaveBeenCalledWith('/api/dashboard', expect.objectContaining({
      headers: { Authorization: 'Bearer test-api-key' },
    }));
  });

  it('aborts in-flight verification on unmount', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    let resolveFetch!: () => void;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolveFetch = () => resolve({ ok: true } as Response);
    });

    vi.spyOn(window, 'fetch').mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
      const signal = init?.signal as AbortSignal | undefined;
      if (signal) {
        signal.addEventListener('abort', () => {
          resolveFetch();
        });
      }
      return fetchPromise;
    });

    localStorage.setItem('cistern_api_key', 'test-key');
    const { unmount } = renderHook(() => useAuth());

    unmount();

    await act(async () => {
      await Promise.resolve();
    });
  });

  it('clears key on logout', () => {
    localStorage.setItem('cistern_api_key', 'old-key');

    const { result } = renderHook(() => useAuth());

    act(() => {
      result.current.logout();
    });

    expect(localStorage.getItem('cistern_api_key')).toBeNull();
  });

  it('sets authError=true when verification fails', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    vi.spyOn(window, 'fetch').mockResolvedValue({
      ok: false,
      status: 401,
    } as Response);

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      result.current.login('bad-key');
    });

    expect(result.current.authError).toBe(true);
    expect(result.current.authenticated).toBe(false);
  });

  it('clears authError when login is retried', async () => {
    const meta = document.createElement('meta');
    meta.setAttribute('name', 'cistern-auth');
    meta.setAttribute('content', 'required');
    document.head.appendChild(meta);

    const fetchSpy = vi.spyOn(window, 'fetch');

    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 401,
    } as Response);

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      result.current.login('bad-key');
    });

    expect(result.current.authError).toBe(true);

    fetchSpy.mockResolvedValueOnce({
      ok: true,
    } as Response);

    await act(async () => {
      result.current.login('good-key');
    });

    expect(result.current.authError).toBe(false);
    expect(result.current.authenticated).toBe(true);
  });
});

describe('getAuthHeaders', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('returns empty object when no key stored', () => {
    expect(getAuthHeaders()).toEqual({});
  });

  it('returns Authorization header when key stored', () => {
    localStorage.setItem('cistern_api_key', 'my-key');
    expect(getAuthHeaders()).toEqual({ Authorization: 'Bearer my-key' });
  });
});

describe('getAuthParams', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('returns empty string when no key stored', () => {
    expect(getAuthParams()).toBe('');
  });

  it('returns token query param when key stored', () => {
    localStorage.setItem('cistern_api_key', 'my-key');
    expect(getAuthParams()).toBe('token=my-key');
  });

  it('encodes special characters in key', () => {
    localStorage.setItem('cistern_api_key', 'key with spaces&special=chars');
    expect(getAuthParams()).toBe('token=key%20with%20spaces%26special%3Dchars');
  });
});