import { describe, it, expect } from 'vitest';
import { truncateBuffer, MAX_BUFFER_SIZE, isAuthCloseCode } from '../utils/buffer';

describe('truncateBuffer', () => {
  it('appends small chunks without truncation', () => {
    const result = truncateBuffer('abc', 'def');
    expect(result).toBe('abcdef');
  });

  it('truncates from the beginning when buffer exceeds max size', () => {
    const largeChunk = 'x'.repeat(MAX_BUFFER_SIZE);
    const result = truncateBuffer(largeChunk, 'new');
    expect(result).toHaveLength(MAX_BUFFER_SIZE);
    expect(result.startsWith('new')).toBe(false);
    expect(result.endsWith('new')).toBe(true);
  });

  it('preserves most recent data when buffer exceeds limit', () => {
    const existing = 'a'.repeat(MAX_BUFFER_SIZE - 10);
    const newChunk = 'b'.repeat(20);
    const result = truncateBuffer(existing, newChunk);
    expect(result.length).toBeLessThanOrEqual(MAX_BUFFER_SIZE);
    expect(result.endsWith('b'.repeat(20))).toBe(true);
  });
});

describe('isAuthCloseCode', () => {
  it('classifies close code 1008 as auth failure', () => {
    expect(isAuthCloseCode(1008)).toBe(true);
  });

  it('classifies close code 4001 as auth failure', () => {
    expect(isAuthCloseCode(4001)).toBe(true);
  });

  it('does not classify normal close as auth failure', () => {
    expect(isAuthCloseCode(1000)).toBe(false);
  });

  it('does not classify other close codes as auth failure', () => {
    expect(isAuthCloseCode(1006)).toBe(false);
    expect(isAuthCloseCode(1011)).toBe(false);
  });
});