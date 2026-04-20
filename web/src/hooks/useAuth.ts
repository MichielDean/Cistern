import { useState, useEffect, useCallback } from 'react';

const STORAGE_KEY = 'cistern_api_key';

function isAuthRequired(): boolean {
  const meta = document.querySelector('meta[name="cistern-auth"]');
  return meta?.getAttribute('content') === 'required';
}

function getStoredKey(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function storeKey(key: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, key);
  } catch {
    // localStorage unavailable; key kept in memory only
  }
}

function clearStoredKey(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // localStorage unavailable
  }
}

export function useAuth() {
  const [required] = useState(() => isAuthRequired());
  const [key, setKey] = useState<string | null>(() => {
    if (!isAuthRequired()) return null;
    return getStoredKey();
  });
  const [authenticated, setAuthenticated] = useState(false);
  const [authError, setAuthError] = useState(false);

  useEffect(() => {
    if (!required) {
      setAuthenticated(true);
      return;
    }
    if (!key) return;

    const controller = new AbortController();
    verifyKey(key, controller.signal).then((ok) => {
      if (controller.signal.aborted) return;
      if (ok) {
        setAuthenticated(true);
        setAuthError(false);
      } else {
        setKey(null);
        clearStoredKey();
        setAuthenticated(false);
        setAuthError(true);
      }
    });

    return () => { controller.abort(); };
  }, [required, key]);

  const login = useCallback((apiKey: string) => {
    storeKey(apiKey);
    setKey(apiKey);
    setAuthError(false);
  }, []);

  const logout = useCallback(() => {
    clearStoredKey();
    setKey(null);
    setAuthenticated(false);
  }, []);

  return { required, key, authenticated, authError, login, logout };
}

async function verifyKey(apiKey: string, signal?: AbortSignal): Promise<boolean> {
  try {
    const resp = await fetch('/api/dashboard', {
      headers: { Authorization: `Bearer ${apiKey}` },
      signal,
    });
    return resp.ok;
  } catch {
    return false;
  }
}

export function getAuthHeaders(): Record<string, string> {
  const apiKey = getStoredKey();
  if (!apiKey) return {};
  return { Authorization: `Bearer ${apiKey}` };
}

export function getAuthParams(): string {
  const apiKey = getStoredKey();
  if (!apiKey) return '';
  return `token=${encodeURIComponent(apiKey)}`;
}