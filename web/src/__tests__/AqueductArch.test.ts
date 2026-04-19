import { describe, it, expect } from 'vitest';
import { formatElapsed } from '../utils/formatElapsed';

describe('formatElapsed', () => {
  it('formats seconds only', () => {
    expect(formatElapsed(45_000_000_000)).toBe('0:45');
  });

  it('formats minutes and seconds', () => {
    expect(formatElapsed(125_000_000_000)).toBe('2:05');
  });

  it('formats hours and minutes', () => {
    expect(formatElapsed(3_780_000_000_000)).toBe('1h03m');
  });

  it('formats zero elapsed', () => {
    expect(formatElapsed(0)).toBe('0:00');
  });

  it('formats large durations', () => {
    expect(formatElapsed(90_000_000_000_000)).toBe('25h00m');
  });

  it('returns placeholder for NaN', () => {
    expect(formatElapsed(NaN)).toBe('--');
  });

  it('returns placeholder for undefined coerced to number', () => {
    expect(formatElapsed(Number(undefined))).toBe('--');
  });
});