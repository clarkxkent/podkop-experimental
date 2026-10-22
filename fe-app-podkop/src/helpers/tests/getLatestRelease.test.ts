import { describe, expect, it } from 'vitest';
import { readCachedRelease } from '../getLatestRelease';

describe('readCachedRelease', () => {
  const minute = 60 * 1000;

  it('gives a tag asked less than an hour ago', () => {
    const raw = JSON.stringify({ tag: 'v0.8.1', at: 1000 });
    expect(readCachedRelease(1000 + 60 * minute - 1, raw)).toBe('v0.8.1');
  });

  it('asks again after an hour', () => {
    const raw = JSON.stringify({ tag: 'v0.8.1', at: 1000 });
    expect(readCachedRelease(1000 + 60 * minute, raw)).toBeUndefined();
  });

  it('keeps a failure for 10 minutes', () => {
    const raw = JSON.stringify({ tag: '', at: 1000 });
    expect(readCachedRelease(1000 + 10 * minute - 1, raw)).toBe('');
    expect(readCachedRelease(1000 + 10 * minute, raw)).toBeUndefined();
  });

  it('asks again when nothing or something broken is kept', () => {
    expect(readCachedRelease(0, null)).toBeUndefined();
    expect(readCachedRelease(0, '{')).toBeUndefined();
    expect(readCachedRelease(0, '{"at":0}')).toBeUndefined();
    expect(readCachedRelease(0, '{"tag":"v1"}')).toBeUndefined();
  });
});
