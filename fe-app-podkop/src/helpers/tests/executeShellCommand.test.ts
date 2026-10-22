import { beforeEach, describe, expect, it, vi } from 'vitest';
import { executeShellCommand } from '../executeShellCommand';

// withTimeout logs through the whole page (its module imports every tab)
vi.mock('../withTimeout', () => ({
  withTimeout: <T>(promise: Promise<T>) => promise,
}));

// rpc.declare of LuCI, standing in: what the object podkop and file.exec answer, and the
// calls each got
let backend: () => Promise<unknown>;
const calls: string[] = [];

(globalThis as unknown as { rpc: unknown }).rpc = {
  declare:
    ({ object, method }: { object: string; method: string }) =>
    async (...args: unknown[]) => {
      calls.push(`${object}.${method} ${JSON.stringify(args)}`);
      if (object === 'podkop') return backend();
      return { code: 0, stdout: 'from the program\n', stderr: '' };
    },
};

describe('executeShellCommand', () => {
  beforeEach(() => {
    calls.length = 0;
  });

  it('takes the answer of the running backend', async () => {
    backend = async () => ({
      code: 0,
      stdout: 'from the backend\n',
      stderr: '',
    });
    const reply = await executeShellCommand({
      command: '/usr/bin/podkop',
      args: ['check_nft_rules', 'label'],
    });
    expect(reply.stdout).toBe('from the backend\n');
    expect(calls).toEqual(['podkop.run ["check_nft_rules",["label"]]']);
  });

  it('runs the program when the backend does not run', async () => {
    backend = async () => {
      throw new Error('Object not found');
    };
    const reply = await executeShellCommand({
      command: '/usr/bin/podkop',
      args: ['get_status'],
    });
    expect(reply.stdout).toBe('from the program\n');
    expect(calls).toEqual([
      'podkop.run ["get_status",[]]',
      'file.exec ["/usr/bin/podkop",["get_status"]]',
    ]);
  });

  it('runs the program for a command the backend does not answer', async () => {
    backend = async () => 8;
    const reply = await executeShellCommand({
      command: '/usr/bin/podkop',
      args: ['global_check'],
    });
    expect(reply.stdout).toBe('from the program\n');
    expect(calls).toHaveLength(2);
  });

  it('runs other programs as they are', async () => {
    await executeShellCommand({
      command: '/etc/init.d/podkop',
      args: ['restart'],
    });
    expect(calls).toEqual(['file.exec ["/etc/init.d/podkop",["restart"]]']);
  });
});
