import { describe, it, expect } from 'vitest';
import { formatAge } from '../utils/formatAge';

describe('formatAge', () => {
  it('formats age in minutes for recent items', () => {
    const fiveMinAgo = new Date(Date.now() - 5 * 60000).toISOString();
    const age = formatAge(fiveMinAgo);
    expect(age).toBe('5m');
  });

  it('formats age in hours for older items', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 3600000).toISOString();
    const age = formatAge(twoHoursAgo);
    expect(age).toBe('2h');
  });

  it('formats age in days for very old items', () => {
    const threeDaysAgo = new Date(Date.now() - 3 * 86400000).toISOString();
    const age = formatAge(threeDaysAgo);
    expect(age).toBe('3d');
  });

  it('returns "--" for invalid created_at', () => {
    expect(formatAge('')).toBe('--');
    expect(formatAge('not-a-date')).toBe('--');
  });

  it('returns "0m" for future timestamps (clock skew)', () => {
    const future = new Date(Date.now() + 60000).toISOString();
    expect(formatAge(future)).toBe('0m');
  });

  it('returns "0m" for current timestamp', () => {
    const now = new Date().toISOString();
    expect(formatAge(now)).toBe('0m');
  });
});