import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import {
  useDroplets,
  useDroplet,
  useDropletNotes,
  useDropletIssues,
  useDropletDependencies,
  useRepoSteps,
  useRepos,
  useSearchDroplets,
  useDropletMutation,
  addNote,
  editDroplet,
  createDroplet,
  addIssue,
  resolveIssue,
  rejectIssue,
  addDependency,
  removeDependency,
} from '../hooks/useApi';
import type { DropletListResponse, Droplet, DropletIssue } from '../api/types';

function mockFetch(response: unknown, ok = true) {
  vi.spyOn(window, 'fetch').mockResolvedValue({
    ok,
    status: ok ? 200 : 500,
    json: () => Promise.resolve(response),
    text: () => Promise.resolve(JSON.stringify(response)),
  } as Response);
}

function getLastFetchCall(): [string, RequestInit | undefined] {
  const mock = vi.mocked(window.fetch);
  const calls = mock.mock.calls as [string, RequestInit | undefined][];
  return calls[calls.length - 1];
}

describe('useDroplets', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('fetches droplets list with default params', async () => {
    const mockResponse: DropletListResponse = {
      droplets: [],
      total: 0,
      page: 1,
      per_page: 50,
    };
    mockFetch(mockResponse);

    const { result } = renderHook(() => useDroplets({}));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.data).toEqual(mockResponse);
    expect(result.current.error).toBeNull();
  });

  it('passes filter params to API', async () => {
    const mockResponse: DropletListResponse = {
      droplets: [],
      total: 0,
      page: 2,
      per_page: 10,
    };
    mockFetch(mockResponse);

    renderHook(() => useDroplets({
      status: 'in_progress',
      repo: 'cistern',
      page: 2,
      per_page: 10,
      sort: 'priority',
    }));

    await waitFor(() => {
      expect(window.fetch).toHaveBeenCalledTimes(1);
    });

    const url = (window.fetch as unknown as { mock: { calls: string[][] } }).mock.calls[0][0];
    expect(url).toContain('status=in_progress');
    expect(url).toContain('repo=cistern');
    expect(url).toContain('page=2');
    expect(url).toContain('per_page=10');
    expect(url).toContain('sort=priority');
  });

  it('handles API errors', async () => {
    vi.spyOn(window, 'fetch').mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve('Internal Server Error'),
    } as Response);

    const { result } = renderHook(() => useDroplets({}));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).not.toBeNull();
  });
});

describe('useDroplet', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('fetches a single droplet by ID', async () => {
    const mockDroplet: Droplet = {
      id: 'ct-abc123',
      repo: 'cistern',
      title: 'Test droplet',
      description: 'A test',
      priority: 1,
      complexity: 2,
      status: 'in_progress',
      assignee: 'implement',
      current_cataractae: 'implement',
      created_at: '2026-04-19T00:00:00Z',
      updated_at: '2026-04-19T00:00:00Z',
    };
    mockFetch(mockDroplet);

    const { result } = renderHook(() => useDroplet('ct-abc123'));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.droplet).toEqual(mockDroplet);
  });

  it('skips fetch when id is null', async () => {
    const fetchSpy = vi.spyOn(window, 'fetch');
    const { result } = renderHook(() => useDroplet(null));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(result.current.droplet).toBeNull();
    fetchSpy.mockRestore();
  });
});

describe('useDropletIssues', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('fetches issues with open filter', async () => {
    const mockIssues: DropletIssue[] = [
      { id: 'issue-1', droplet_id: 'ct-abc123', flagged_by: 'reviewer', flagged_at: '2026-04-19T00:00:00Z', description: 'Bug found', status: 'open' },
    ];
    mockFetch(mockIssues);

    const { result } = renderHook(() => useDropletIssues('ct-abc123', { open: true }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.issues).toEqual(mockIssues);
    const [url] = getLastFetchCall();
    expect(url).toContain('open=true');
  });

  it('skips fetch when id is null', async () => {
    const fetchSpy = vi.spyOn(window, 'fetch');
    const { result } = renderHook(() => useDropletIssues(null));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });
});

describe('useDropletDependencies', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('fetches dependencies for a droplet', async () => {
    const mockDeps = [
      { depends_on: 'ct-dep1', type: 'blocking' as const },
      { depends_on: 'ct-dep2', type: 'blocked_by' as const },
    ];
    mockFetch(mockDeps);

    const { result } = renderHook(() => useDropletDependencies('ct-abc123'));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.dependencies).toEqual(mockDeps);
  });
});

describe('useRepoSteps', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('fetches pipeline steps for a repo', async () => {
    const mockSteps = ['flag', 'implement', 'review', 'qa'];
    mockFetch(mockSteps);

    const { result } = renderHook(() => useRepoSteps('cistern'));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.steps).toEqual(mockSteps);
    expect(result.current.error).toBeNull();
  });

  it('skips fetch when repoName is null', async () => {
    const fetchSpy = vi.spyOn(window, 'fetch');
    const { result } = renderHook(() => useRepoSteps(null));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(result.current.steps).toEqual([]);
    fetchSpy.mockRestore();
  });

  it('handles errors gracefully', async () => {
    vi.spyOn(window, 'fetch').mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve('Internal Server Error'),
    } as Response);

    const { result } = renderHook(() => useRepoSteps('cistern'));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).not.toBeNull();
    expect(result.current.steps).toEqual([]);
  });
});

describe('useRepos', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => vi.restoreAllMocks());

  it('fetches repos list as RepoInfo objects', async () => {
    const mockRepos = [
      { name: 'cistern', url: 'https://github.com/example/cistern' },
      { name: 'other', url: 'https://github.com/example/other' },
    ];
    mockFetch(mockRepos);

    const { result } = renderHook(() => useRepos());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.repos).toEqual(mockRepos);
    expect(result.current.repos[0].name).toBe('cistern');
    expect(result.current.repos[0].url).toBe('https://github.com/example/cistern');
  });
});

describe('useSearchDroplets', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('searches droplets with query', async () => {
    const mockResults = { droplets: [], total: 0 };
    mockFetch(mockResults);

    const { result } = renderHook(() => useSearchDroplets());
    const searchResult = await result.current.mutate('test query');

    expect(searchResult).toEqual(mockResults);
  });
});

describe('useDropletMutation', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('posts action to droplet endpoint', async () => {
    mockFetch(undefined);

    const { result } = renderHook(() => useDropletMutation());
    await result.current.mutate('ct-abc123', 'pass', { notes: 'done' });

    const [url, options] = getLastFetchCall();
    expect(url).toContain('/ct-abc123/pass');
    expect(options?.method).toBe('POST');
  });
});

describe('Mutation functions', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it('addNote posts to notes endpoint', async () => {
    mockFetch(undefined);
    await addNote('ct-abc123', { cataractae: 'manual', content: 'test note' });

    const [url] = getLastFetchCall();
    expect(url).toContain('/ct-abc123/notes');
  });

  it('editDroplet patches droplet endpoint', async () => {
    const mockDroplet: Droplet = {
      id: 'ct-abc123', repo: 'cistern', title: 'Updated', description: '',
      priority: 1, complexity: 2, status: 'open', assignee: '', current_cataractae: '',
      created_at: '2026-04-19T00:00:00Z', updated_at: '2026-04-19T01:00:00Z',
    };
    mockFetch(mockDroplet);
    await editDroplet('ct-abc123', { title: 'Updated' });

    const [url, options] = getLastFetchCall();
    expect(url).toContain('/ct-abc123');
    expect(options?.method).toBe('PATCH');
  });

  it('createDroplet posts to droplets endpoint', async () => {
    const mockDroplet: Droplet = {
      id: 'ct-new1', repo: 'cistern', title: 'New', description: '',
      priority: 1, complexity: 2, status: 'open', assignee: '', current_cataractae: '',
      created_at: '2026-04-19T00:00:00Z', updated_at: '2026-04-19T00:00:00Z',
    };
    mockFetch(mockDroplet);
    await createDroplet({ repo: 'cistern', title: 'New' });

    const [url] = getLastFetchCall();
    expect(url).toContain('/droplets');
  });

  it('addIssue posts to issues endpoint', async () => {
    const mockIssue: DropletIssue = {
      id: 'issue-1', droplet_id: 'ct-abc123', flagged_by: 'manual',
      flagged_at: '2026-04-19T00:00:00Z', description: 'test issue', status: 'open',
    };
    mockFetch(mockIssue);
    await addIssue('ct-abc123', { description: 'test issue' });

    const [url] = getLastFetchCall();
    expect(url).toContain('/ct-abc123/issues');
  });

  it('resolveIssue posts to resolve endpoint', async () => {
    mockFetch(undefined);
    await resolveIssue('issue-1', { evidence: 'fixed' });

    const [url] = getLastFetchCall();
    expect(url).toContain('/issues/issue-1/resolve');
  });

  it('rejectIssue posts to reject endpoint', async () => {
    mockFetch(undefined);
    await rejectIssue('issue-1', { evidence: 'not a bug' });

    const [url] = getLastFetchCall();
    expect(url).toContain('/issues/issue-1/reject');
  });

  it('addDependency posts to dependencies endpoint', async () => {
    mockFetch(undefined);
    await addDependency('ct-abc123', 'ct-dep1');

    const [url, options] = getLastFetchCall();
    expect(url).toContain('/ct-abc123/dependencies');
    expect(options?.method).toBe('POST');
  });

  it('removeDependency deletes from dependencies endpoint', async () => {
    mockFetch(undefined);
    await removeDependency('ct-abc123', 'ct-dep1');

    const [url, options] = getLastFetchCall();
    expect(url).toContain('/ct-abc123/dependencies/ct-dep1');
    expect(options?.method).toBe('DELETE');
  });
});

describe('useDroplet refetch', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => vi.restoreAllMocks());

  it('exposes a refetch function that re-fetches the droplet', async () => {
    const mockDroplet: Droplet = {
      id: 'ct-abc123', repo: 'cistern', title: 'Test', description: '',
      priority: 1, complexity: 2, status: 'in_progress', assignee: 'implement',
      current_cataractae: 'implement', created_at: '2026-04-19T00:00:00Z', updated_at: '2026-04-19T00:00:00Z',
    };
    mockFetch(mockDroplet);

    const { result } = renderHook(() => useDroplet('ct-abc123'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.droplet).toEqual(mockDroplet);
    expect(typeof result.current.refetch).toBe('function');

    const updated: Droplet = { ...mockDroplet, status: 'done', updated_at: '2026-04-19T01:00:00Z' };
    mockFetch(updated);
    await result.current.refetch();
    await waitFor(() => expect(result.current.droplet?.status).toBe('done'));
  });
});

describe('useDropletNotes refetch', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => vi.restoreAllMocks());

  it('exposes a refetch function that re-fetches notes', async () => {
    const mockES = { onmessage: null, onerror: null, close: vi.fn() };
    vi.stubGlobal('EventSource', vi.fn(() => mockES));

    const mockNotes = [
      { id: 1, droplet_id: 'ct-abc123', cataractae_name: 'implement', content: 'note', created_at: '2026-04-19T00:00:00Z' },
    ];
    mockFetch(mockNotes);

    const { result } = renderHook(() => useDropletNotes('ct-abc123'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.notes).toEqual(mockNotes);
    expect(typeof result.current.refetch).toBe('function');

    vi.unstubAllGlobals();
  });
});

describe('useDropletIssues refetch', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => vi.restoreAllMocks());

  it('exposes a refetch function', async () => {
    const mockIssues: DropletIssue[] = [
      { id: 'issue-1', droplet_id: 'ct-abc123', flagged_by: 'reviewer', flagged_at: '2026-04-19T00:00:00Z', description: 'Bug', status: 'open' },
    ];
    mockFetch(mockIssues);

    const { result } = renderHook(() => useDropletIssues('ct-abc123', { open: true }));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(typeof result.current.refetch).toBe('function');
  });
});

describe('useDropletDependencies refetch', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => vi.restoreAllMocks());

  it('exposes a refetch function', async () => {
    const mockDeps = [
      { depends_on: 'ct-dep1', type: 'blocking' as const },
    ];
    mockFetch(mockDeps);

    const { result } = renderHook(() => useDropletDependencies('ct-abc123'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(typeof result.current.refetch).toBe('function');
  });
});