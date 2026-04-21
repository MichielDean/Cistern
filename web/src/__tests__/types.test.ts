import { describe, it, expect } from 'vitest';
import type { DashboardData, Droplet, CataractaeInfo, FlowActivity, CataractaeNote, DropletIssue } from '../api/types';

describe('DashboardData type', () => {
  it('has all required fields matching the Go struct', () => {
    const data: DashboardData = {
      cataractae_count: 3,
      flowing_count: 1,
      queued_count: 2,
      done_count: 5,
      cataractae: [],
      unassigned_items: [],
      cistern_items: [],
      pooled_items: [],
      recent_items: [],
      blocked_by_map: {},
      flow_activities: [],
      castellarius_running: true,
      fetched_at: '2026-04-19T00:00:00Z',
    };
    expect(data.cataractae_count).toBe(3);
    expect(data.flowing_count).toBe(1);
    expect(data.castellarius_running).toBe(true);
  });
});

describe('Droplet type', () => {
  it('has required fields matching the Go struct', () => {
    const droplet: Droplet = {
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
    expect(droplet.id).toBe('ct-abc123');
    expect(droplet.status).toBe('in_progress');
  });

  it('allows optional fields to be undefined', () => {
    const droplet: Droplet = {
      id: 'ct-abc123',
      repo: 'cistern',
      title: 'Test',
      description: '',
      priority: 1,
      complexity: 1,
      status: 'open',
      assignee: '',
      current_cataractae: '',
      created_at: '2026-04-19T00:00:00Z',
      updated_at: '2026-04-19T00:00:00Z',
    };
    expect(droplet.outcome).toBeUndefined();
    expect(droplet.assigned_aqueduct).toBeUndefined();
  });
});

describe('CataractaeInfo type', () => {
  it('has workflow steps and progress fields', () => {
    const info: CataractaeInfo = {
      name: 'default',
      repo_name: 'cistern',
      droplet_id: 'ct-abc123',
      title: 'Test droplet',
      step: 'implement',
      steps: ['flag', 'implement', 'review', 'qa'],
      elapsed: 300000000000,
      stage_elapsed: 120000000000,
      cataractae_index: 2,
      total_cataractae: 4,
    };
    expect(info.cataractae_index).toBe(2);
    expect(info.total_cataractae).toBe(4);
    expect(info.steps).toHaveLength(4);
  });
});

describe('FlowActivity type', () => {
  it('includes recent notes', () => {
    const note: CataractaeNote = {
      id: 1,
      droplet_id: 'ct-abc123',
      cataractae_name: 'implement',
      content: 'Implemented feature',
      created_at: '2026-04-19T00:00:00Z',
    };
    const activity: FlowActivity = {
      droplet_id: 'ct-abc123',
      title: 'Test droplet',
      step: 'implement',
      recent_notes: [note],
    };
    expect(activity.recent_notes).toHaveLength(1);
    expect(activity.recent_notes[0].cataractae_name).toBe('implement');
  });
});

describe('DropletIssue type', () => {
  it('supports open/resolved/unresolved status', () => {
    const issue: DropletIssue = {
      id: 'issue-1',
      droplet_id: 'ct-abc123',
      flagged_by: 'reviewer',
      flagged_at: '2026-04-19T00:00:00Z',
      description: 'Bug found',
      status: 'open',
    };
    expect(issue.status).toBe('open');

    const resolved: DropletIssue = {
      ...issue,
      status: 'resolved',
      resolved_at: '2026-04-19T01:00:00Z',
    };
    expect(resolved.status).toBe('resolved');
  });
});