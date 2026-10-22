import { describe, expect, it } from 'vitest';
import {
  catchUp,
  isPodkopError,
  missedSome,
  type LogLine,
} from '../logWatch.service';

function line(id: number, time: number, msg = 'podkop: [info] x'): LogLine {
  return { id, time, msg };
}

describe('catchUp', () => {
  const minute = 60 * 1000;
  const log = [
    line(10, 0),
    line(11, 20 * minute),
    line(12, 25 * minute),
    line(13, 30 * minute),
  ];

  it('gives the lines after the cursor and the newest id', () => {
    expect(catchUp(log, 11)).toEqual({ lines: [log[2], log[3]], cursor: 13 });
    expect(catchUp(log, 13)).toEqual({ lines: [], cursor: 13 });
  });

  it('takes only the last minutes when the tab has seen no line', () => {
    expect(catchUp(log, undefined)).toEqual({
      lines: [log[2], log[3]],
      cursor: 13,
    });
  });

  it('takes a cursor beyond the newest line as one of the last boot', () => {
    expect(catchUp(log, 5000)).toEqual({
      lines: [log[2], log[3]],
      cursor: 13,
    });
  });

  it('keeps the cursor of an empty log', () => {
    expect(catchUp([], 7)).toEqual({ lines: [], cursor: 7 });
  });
});

describe('missedSome', () => {
  const few = [line(50, 0), line(51, 0), line(52, 0)];

  it('asks again when the few lines start after a gap', () => {
    expect(missedSome(few, 40)).toBe(true);
  });

  it('does not when they follow the cursor or nothing came', () => {
    expect(missedSome(few, 49)).toBe(false);
    expect(missedSome(few, 52)).toBe(false);
    expect(missedSome([], 52)).toBe(false);
  });

  it('does not for the first turn or after a reboot', () => {
    expect(missedSome(few, undefined)).toBe(false);
    expect(missedSome(few, 900)).toBe(false);
  });
});

describe('isPodkopError', () => {
  it('tells the errors of the backend', () => {
    expect(isPodkopError(line(1, 0, 'podkop[42]: [error] x'))).toBe(true);
    expect(isPodkopError(line(1, 0, 'podkop: [fatal] x'))).toBe(true);
    expect(isPodkopError(line(1, 0, 'podkop[42]: [ERROR] x'))).toBe(true);
  });

  it('leaves other lines and other programs', () => {
    expect(isPodkopError(line(1, 0, 'podkop[42]: [warn] x'))).toBe(false);
    expect(isPodkopError(line(1, 0, 'uhttpd[7]: [error] podkop'))).toBe(false);
    expect(isPodkopError(line(1, 0, 'podkop-guard: [error] x'))).toBe(false);
  });
});
