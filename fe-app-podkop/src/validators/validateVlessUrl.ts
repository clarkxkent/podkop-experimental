import { ValidationResult } from './types';
import {
  invalid,
  wrongStart,
  readServer,
  valid,
  validUserID,
} from './linkAddress';

// vless://uuid@host:port?parameters#name: the UUID and the port are needed, the parameters
// are the engine's business
export function validateVlessUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['vless']);
  if (start) return invalid('VLESS', start);
  const server = readServer(url, true);
  if (typeof server === 'string') return invalid('VLESS', server);
  if (!server.user) return invalid('VLESS', _('missing UUID'));
  if (!validUserID(server.user)) return invalid('VLESS', _('invalid UUID'));
  return valid();
}
