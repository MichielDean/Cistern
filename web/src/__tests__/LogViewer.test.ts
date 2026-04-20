import { describe, it, expect } from 'vitest';
import { parseLogLines } from '../components/LogViewer';

describe('parseLogLines', () => {
  it('parses lines with level extraction', () => {
    const lines = [
      '2026-04-19 12:00:01 INFO server started',
      '2026-04-19 12:00:02 WARN low disk',
      '2026-04-19 12:00:03 ERROR connection lost',
      'stack trace line',
    ];
    const entries = parseLogLines(lines);
    expect(entries).toHaveLength(4);
    expect(entries[0].line).toBe(1);
    expect(entries[0].level).toBe('INFO');
    expect(entries[1].level).toBe('WARN');
    expect(entries[2].level).toBe('ERROR');
    expect(entries[3].level).toBe('');
  });

  it('assigns sequential line numbers', () => {
    const lines = ['line1', 'line2', 'line3'];
    const entries = parseLogLines(lines);
    expect(entries[0].line).toBe(1);
    expect(entries[1].line).toBe(2);
    expect(entries[2].line).toBe(3);
  });

  it('preserves raw text', () => {
    const lines = ['2026-04-19 12:00:01 DEBUG something'];
    const entries = parseLogLines(lines);
    expect(entries[0].raw).toBe('2026-04-19 12:00:01 DEBUG something');
    expect(entries[0].text).toBe('2026-04-19 12:00:01 DEBUG something');
  });

  it('handles empty array', () => {
    const entries = parseLogLines([]);
    expect(entries).toHaveLength(0);
  });

  it('extracts DEBUG level', () => {
    const entries = parseLogLines(['DEBUG processing']);
    expect(entries[0].level).toBe('DEBUG');
  });

  it('only extracts first log level match', () => {
    const entries = parseLogLines(['INFO something ERROR nested']);
    expect(entries[0].level).toBe('INFO');
  });
});