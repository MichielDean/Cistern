import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createFilterSession, resumeFilterSession, listFilterSessions, parseFilterMessages } from '../api/filter';
import type { FilterNewResponse, FilterResumeResponse, FilterSession } from '../api/types';

function mockFetch(response: unknown, ok = true, status = 200) {
  vi.spyOn(window, 'fetch').mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(response),
    text: () => Promise.resolve(JSON.stringify(response)),
  } as Response);
}

describe('filter API', () => {
  beforeEach(() => { localStorage.clear(); });
  afterEach(() => { vi.restoreAllMocks(); });

  describe('createFilterSession', () => {
    it('sends POST to /api/filter/new', async () => {
      const mockResponse: FilterNewResponse = {
        session_id: 'fs-abc1',
        llm_session_id: 'llm-123',
        session: {
          id: 'fs-abc1',
          title: 'Test idea',
          description: '',
          messages: '[]',
          spec_snapshot: '',
          llm_session_id: 'llm-123',
          created_at: '2026-04-20T00:00:00Z',
          updated_at: '2026-04-20T00:00:00Z',
        },
        assistant_message: 'Hello!',
      };
      mockFetch(mockResponse);

      const result = await createFilterSession('Test idea');
      expect(result.session_id).toBe('fs-abc1');
      expect(result.assistant_message).toBe('Hello!');
    });
  });

  describe('listFilterSessions', () => {
    it('fetches sessions list', async () => {
      const sessions: FilterSession[] = [
        { id: 'fs-1', title: 'Session 1', description: '', messages: '[]', spec_snapshot: '', llm_session_id: 'llm-abc', created_at: '2026-04-20T00:00:00Z', updated_at: '2026-04-20T00:00:00Z' },
      ];
      mockFetch(sessions);

      const result = await listFilterSessions();
      expect(result).toHaveLength(1);
      expect(result[0].title).toBe('Session 1');
      expect(result[0].llm_session_id).toBe('llm-abc');
    });
  });

  describe('resumeFilterSession', () => {
    it('sends POST to /api/filter/{id}/resume', async () => {
      const mockResponse: FilterResumeResponse = {
        session_id: 'fs-1',
        llm_session_id: 'llm-456',
        assistant_message: 'Good point!',
        spec_snapshot: 'Good point!',
      };
      mockFetch(mockResponse);

      const result = await resumeFilterSession('fs-1', 'my feedback');
      expect(result.assistant_message).toBe('Good point!');
    });
  });

  describe('parseFilterMessages', () => {
    it('parses valid JSON', () => {
      const msgs = parseFilterMessages('[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]');
      expect(msgs).toHaveLength(2);
      expect(msgs[0].role).toBe('user');
    });

    it('returns empty array for empty string', () => {
      expect(parseFilterMessages('')).toEqual([]);
      expect(parseFilterMessages('[]')).toEqual([]);
    });

    it('returns empty array for invalid JSON', () => {
      expect(parseFilterMessages('not json')).toEqual([]);
    });
  });
});