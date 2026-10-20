import { executeShellCommand } from '../../../helpers';
import { Podkop } from '../../types';

export async function callBaseMethod<T>(
  method: Podkop.AvailableMethods,
  args: string[] = [],
  command: string = '/usr/bin/podkop',
  timeout: number = 15000,
): Promise<Podkop.MethodResponse<T>> {
  const response = await executeShellCommand({
    command,
    args: [method as string, ...args],
    timeout,
  });

  if (response.stdout) {
    try {
      return {
        success: true,
        data: JSON.parse(response.stdout) as T,
      };
    } catch (_e) {
      return {
        success: true,
        data: response.stdout as T,
      };
    }
  }

  return {
    success: false,
    error: '',
  };
}
