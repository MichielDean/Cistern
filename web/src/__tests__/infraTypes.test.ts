import { describe, it, expect } from 'vitest';
import { parseLogLines } from '../components/LogViewer';
import type {
  CastellariusStatus,
  DoctorResult,
  LogEntry,
  LogSourceInfo,
  RepoInfo,
  SkillInfo,
} from '../api/types';

describe('CastellariusStatus shape validation', () => {
  it('running status has all required fields for rendering', () => {
    const status: CastellariusStatus = {
      running: true,
      pid: 12345,
      uptime_seconds: 8100,
      aqueducts: [],
      castellarius_running: true,
    };
    expect(typeof status.running).toBe('boolean');
    expect(typeof status.pid === 'number' || status.pid === null).toBe(true);
    expect(typeof status.uptime_seconds === 'number' || status.uptime_seconds === null).toBe(true);
    expect(Array.isArray(status.aqueducts)).toBe(true);
    expect(typeof status.castellarius_running).toBe('boolean');
  });

  it('stopped status renders correctly with null optionals', () => {
    const status: CastellariusStatus = {
      running: false,
      pid: null,
      uptime_seconds: null,
      aqueducts: [],
      castellarius_running: false,
    };
    const label = status.running ? 'Running' : 'Stopped';
    expect(label).toBe('Stopped');
  });

  it('aqueduct with flowing status has droplet metadata', () => {
    const status: CastellariusStatus = {
      running: true,
      pid: 1,
      uptime_seconds: 0,
      aqueducts: [{
        name: 'default',
        status: 'flowing',
        droplet_id: 'ci-abc1',
        droplet_title: 'Test droplet',
        current_step: 'implement',
        elapsed: 300_000_000_000,
      }],
      castellarius_running: false,
    };
    const flowing = status.aqueducts.filter(a => a.status === 'flowing');
    expect(flowing).toHaveLength(1);
    expect(flowing[0].droplet_id).toBeTruthy();
  });
});

describe('DoctorResult rendering logic', () => {
  it('computes pass/fail/warn counts from checks', () => {
    const result: DoctorResult = {
      checks: [
        { name: 'Daemon running', status: 'pass', message: 'Running', category: 'Daemon' },
        { name: 'Config valid', status: 'fail', message: 'Missing field', category: 'Config' },
        { name: 'Disk space', status: 'warn', message: 'Low', category: 'System' },
      ],
      summary: { total: 3, passed: 1 },
      timestamp: '2026-04-19T00:00:00Z',
    };
    const passes = result.checks.filter(c => c.status === 'pass').length;
    const fails = result.checks.filter(c => c.status === 'fail').length;
    const warns = result.checks.filter(c => c.status === 'warn').length;
    expect(passes).toBe(1);
    expect(fails).toBe(1);
    expect(warns).toBe(1);
  });

  it('groups checks by category', () => {
    const result: DoctorResult = {
      checks: [
        { name: 'A', status: 'pass', message: '', category: 'Daemon' },
        { name: 'B', status: 'pass', message: '', category: 'Daemon' },
        { name: 'C', status: 'fail', message: '', category: 'Config' },
      ],
      summary: { total: 3, passed: 2 },
      timestamp: '',
    };
    const groups = new Map<string, typeof result.checks>();
    for (const c of result.checks) {
      const list = groups.get(c.category) ?? [];
      list.push(c);
      groups.set(c.category, list);
    }
    expect(groups.get('Daemon')).toHaveLength(2);
    expect(groups.get('Config')).toHaveLength(1);
  });
});

describe('LogEntry and parseLogLines integration', () => {
  it('parseLogLines produces valid LogEntry objects', () => {
    const entries: LogEntry[] = parseLogLines([
      '2026-04-19 12:00:01 INFO server started',
      'stack trace',
    ]);
    expect(entries).toHaveLength(2);
    expect(entries[0]).toMatchObject({ line: 1, level: 'INFO' });
    expect(entries[1]).toMatchObject({ line: 2, level: '' });
  });

  it('LogSourceInfo has required display fields', () => {
    const info: LogSourceInfo = {
      name: 'castellarius',
      size_bytes: 2048576,
      last_modified: '2026-04-19T12:00:00Z',
    };
    expect(info.name).toBeTruthy();
    expect(info.size_bytes).toBeGreaterThanOrEqual(0);
    expect(new Date(info.last_modified).toString()).not.toBe('Invalid Date');
  });
});

describe('RepoInfo aqueduct rendering', () => {
  it('renders step chain from aqueducts', () => {
    const repo: RepoInfo = {
      name: 'my-app',
      prefix: 'myapp',
      url: 'https://github.com/org/my-app',
      aqueduct_config: '/path/to/aqueduct.yaml',
      aqueducts: [
        { name: 'main', steps: ['architect', 'implement', 'review', 'qa'] },
        { name: 'docs', steps: ['docs-writer'] },
      ],
    };
    const stepCount = repo.aqueducts.reduce((sum, aq) => sum + aq.steps.length, 0);
    expect(stepCount).toBe(5);
  });

  it('handles repo with no aqueduct config', () => {
    const repo: RepoInfo = {
      name: 'simple',
      prefix: 'simple',
      url: 'https://github.com/org/simple',
      aqueduct_config: null,
      aqueducts: [],
    };
    expect(repo.aqueduct_config).toBeNull();
    expect(repo.aqueducts).toHaveLength(0);
  });
});

describe('SkillInfo display fields', () => {
  it('has name and URL for rendering', () => {
    const skill: SkillInfo = {
      name: 'cistern-git',
      source_url: '/skills/cistern-git/SKILL.md',
      installed_at: '2026-01-10T00:00:00Z',
    };
    expect(skill.name).toBeTruthy();
    expect(skill.source_url).toBeTruthy();
    expect(new Date(skill.installed_at).toString()).not.toBe('Invalid Date');
  });
});