import { describe, it, expect } from 'vitest';
import type {
  CastellariusStatus,
  AqueductStatus,
  DoctorResult,
  LogEntry,
  LogSourceInfo,
  RepoInfo,
  AqueductBrief,
  SkillInfo,
} from '../api/types';

describe('CastellariusStatus type', () => {
  it('accepts full status', () => {
    const status: CastellariusStatus = {
      running: true,
      pid: 12345,
      uptime_seconds: 8100,
      aqueducts: [],
      farm_running: true,
    };
    expect(status.running).toBe(true);
    expect(status.pid).toBe(12345);
    expect(status.uptime_seconds).toBe(8100);
  });

  it('accepts status with null optional fields', () => {
    const status: CastellariusStatus = {
      running: false,
      pid: null,
      uptime_seconds: null,
      aqueducts: [],
      farm_running: false,
    };
    expect(status.running).toBe(false);
    expect(status.pid).toBeNull();
  });

  it('accepts aqueducts with flowing status', () => {
    const aq: AqueductStatus = {
      name: 'default',
      status: 'flowing',
      droplet_id: 'ci-abc1',
      droplet_title: 'Test droplet',
      current_step: 'implement',
      elapsed: 300000000000,
    };
    expect(aq.status).toBe('flowing');
    expect(aq.droplet_id).toBe('ci-abc1');
  });

  it('accepts aqueducts with idle status', () => {
    const aq: AqueductStatus = {
      name: 'docs',
      status: 'idle',
      droplet_id: null,
      droplet_title: null,
      current_step: null,
      elapsed: 0,
    };
    expect(aq.status).toBe('idle');
    expect(aq.droplet_id).toBeNull();
  });
});

describe('DoctorResult type', () => {
  it('accepts check result with categories', () => {
    const result: DoctorResult = {
      checks: [
        { name: 'Daemon running', status: 'pass' as const, message: 'Running (pid 123)', category: 'Daemon' },
        { name: 'Config valid', status: 'fail' as const, message: 'Missing field X', category: 'Config' },
        { name: 'Disk space', status: 'warn' as const, message: 'Low disk space', category: 'System' },
      ],
      summary: { total: 3, passed: 1 },
      timestamp: '2026-04-19T00:00:00Z',
    };
    expect(result.checks).toHaveLength(3);
    expect(result.summary.passed).toBe(1);
    expect(result.checks[1].status).toBe('fail');
  });

  it('accepts empty check result', () => {
    const result: DoctorResult = {
      checks: [],
      summary: { total: 0, passed: 0 },
      timestamp: '',
    };
    expect(result.checks).toHaveLength(0);
  });
});

describe('LogEntry type', () => {
  it('accepts log entry with level', () => {
    const entry: LogEntry = {
      line: 1,
      level: 'INFO',
      text: '2026-04-19 12:00:01 INFO server started',
      raw: '2026-04-19 12:00:01 INFO server started',
    };
    expect(entry.level).toBe('INFO');
    expect(entry.line).toBe(1);
  });

  it('accepts log entry with empty level', () => {
    const entry: LogEntry = {
      line: 5,
      level: '',
      text: 'stack trace line',
      raw: 'stack trace line',
    };
    expect(entry.level).toBe('');
  });
});

describe('LogSourceInfo type', () => {
  it('accepts source info', () => {
    const info: LogSourceInfo = {
      name: 'castellarius',
      path: '/home/user/.cistern/castellarius.log',
      size_bytes: 2048576,
      last_modified: '2026-04-19T12:00:00Z',
    };
    expect(info.name).toBe('castellarius');
    expect(info.size_bytes).toBeGreaterThan(0);
  });
});

describe('RepoInfo type', () => {
  it('accepts repo with aqueducts', () => {
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
    expect(repo.aqueducts).toHaveLength(2);
    expect(repo.aqueducts[0].steps).toHaveLength(4);
  });

  it('accepts repo with no aqueducts', () => {
    const repo: RepoInfo = {
      name: 'simple',
      prefix: 'simple',
      url: 'https://github.com/org/simple',
      aqueduct_config: null,
      aqueducts: [],
    };
    expect(repo.aqueduct_config).toBeNull();
  });
});

describe('AqueductBrief type', () => {
  it('accepts brief with steps', () => {
    const brief: AqueductBrief = {
      name: 'default',
      steps: ['flag', 'implement', 'review', 'qa'],
    };
    expect(brief.steps).toHaveLength(4);
  });
});

describe('SkillInfo type', () => {
  it('accepts skill info', () => {
    const skill: SkillInfo = {
      name: 'cistern-git',
      source_url: '/skills/cistern-git/SKILL.md',
      installed_at: '2026-01-10T00:00:00Z',
    };
    expect(skill.name).toBe('cistern-git');
  });
});