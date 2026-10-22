import { describe, expect, it, vi } from 'vitest';
import { Logger } from '../logger.service';

describe('Logger', () => {
  it('keeps the last 1000 lines', () => {
    const logger = new Logger();
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    for (let i = 0; i < 1100; i++) logger.warn('line', i);
    warn.mockRestore();
    const lines = logger.getLogs().split('\n');
    expect(lines).toHaveLength(1000);
    expect(lines[0]).toBe('[WARN] line 100');
    expect(lines[999]).toBe('[WARN] line 1099');
  });

  it('writes only warnings and errors to the console', () => {
    const logger = new Logger();
    const info = vi.spyOn(console, 'info').mockImplementation(() => {});
    const debug = vi.spyOn(console, 'log').mockImplementation(() => {});
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    logger.info('a');
    logger.debug('b');
    logger.error('c');
    expect(info).not.toHaveBeenCalled();
    expect(debug).not.toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith('[ERROR] c');
    vi.restoreAllMocks();
    expect(logger.getLogs()).toBe('[INFO] a\n[DEBUG] b\n[ERROR] c');
  });
});
