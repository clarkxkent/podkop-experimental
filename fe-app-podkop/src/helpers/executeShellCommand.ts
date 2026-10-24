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
let backendRun: ExecuteFile | undefined;

const PODKOP = '/usr/bin/podkop';

// The commands of /usr/bin/podkop are asked first of the running backend: the method run of
// its object "podkop" on ubus answers them without a program started for a call, in the form
// of file.exec. When the backend does not run (no such object), or does not answer that
// command itself (a status: the actions on the service, the global check), the program runs.
// A call that got no answer (LuCI gave up waiting) is not run again by the program: the
// backend may still be running it.
async function askBackend(
  args: string[],
): Promise<ExecuteShellCommandResponse | undefined> {
  backendRun ??= rpc.declare<ExecuteShellCommandResponse | number>({
    object: 'podkop',
    method: 'run',
    params: ['command', 'params'],
    nobatch: true,
  });
  try {
    const reply = await backendRun(args[0], args.slice(1));
    return typeof reply === 'object' && reply !== null ? reply : undefined;
  } catch (err) {
    // the errors of rpc.js for an answer that came: "RPC call to podkop/run failed with ..."
    if (/^RPC call to /.test((err as Error)?.message ?? '')) return undefined;
    throw err;
  }
}

// fs.exec leaves the call to LuCI, which gathers the calls of one frame into a single
// request; the router answers such a request call by call, so a quick command waits for
// the slow ones asked beside it. Here every command is a request of its own.
async function execute(
  command: string,
  args: string[],
): Promise<ExecuteShellCommandResponse> {
  if (command === PODKOP && args.length) {
    const answered = await askBackend(args);
    if (answered) return answered;
  }

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
