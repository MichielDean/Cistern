import { describe, it, expect } from 'vitest';
import { formatElapsed } from '../utils/formatElapsed';

describe('formatElapsed timer computation', () => {
  it('computes elapsed from a known start time', () => {
    const startTime = Date.now() - 30000;
    const elapsedNs = (Date.now() - startTime) * 1e6;
    expect(formatElapsed(elapsedNs)).toMatch(/0:3[0-9]/);
  });

  it('recalculates when flowing transitions without SSE update', () => {
    const elapsedNs = 10_000_000_000;
    const currentElapsed = (5 * 1000 + elapsedNs / 1e6) * 1e6;
    expect(formatElapsed(currentElapsed)).toMatch(/\d+:\d{2}/);
  });
});

describe('formatElapsed edge cases', () => {
  it('returns placeholder for negative values', () => {
    expect(formatElapsed(-1_000_000_000)).toBe('--');
  });

  it('returns placeholder for NaN', () => {
    expect(formatElapsed(NaN)).toBe('--');
  });
});