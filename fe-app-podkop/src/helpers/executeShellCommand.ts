import { COMMAND_TIMEOUT } from '../constants';
import { withTimeout } from './withTimeout';

interface ExecuteShellCommandParams {
  command: string;
  args: string[];
  timeout?: number;
}

interface ExecuteShellCommandResponse {
  stdout: string;
  stderr: string;
  code?: number;
}

type ExecuteFile = (
  command: string,
  args: string[],
) => Promise<ExecuteShellCommandResponse | number>;

let executeFile: ExecuteFile | undefined;

// fs.exec leaves the call to LuCI, which gathers the calls of one frame into a single
// request; the router answers such a request call by call, so a quick command waits for
// the slow ones asked beside it. Here every command is a request of its own.
async function execute(
  command: string,
  args: string[],
): Promise<ExecuteShellCommandResponse> {
  executeFile ??= rpc.declare<ExecuteShellCommandResponse | number>({
    object: 'file',
    method: 'exec',
    params: ['command', 'params', 'env'],
    nobatch: true,
  });

  const reply = await executeFile(command, args);

  // a number is the error code of the call itself, as fs.exec reports it by throwing
  if (typeof reply !== 'object' || reply === null) {
    throw new Error(`${command}: rpc error ${reply}`);
  }

  return reply;
}

export async function executeShellCommand({
  command,
  args,
  timeout = COMMAND_TIMEOUT,
}: ExecuteShellCommandParams): Promise<ExecuteShellCommandResponse> {
  try {
    // awaited here: a rejection must reach the catch below
    return await withTimeout(
      execute(command, args),
      timeout,
      [command, ...args].join(' '),
    );
  } catch (err) {
    const error = err as Error;
    return { stdout: '', stderr: error?.message, code: 0 };
  }
}
