import { ValidationResult } from './types';
import {
  invalid,
  wrongStart,
  readServer,
  strictUUID,
  valid,
} from './linkAddress';

// tuic://uuid:password@host[:port]?parameters#name (TUIC v5): without a port, 443
export function validateTuicUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['tuic']);
  if (start) return invalid('TUIC', start);
  const server = readServer(url, false);
  if (typeof server === 'string') return invalid('TUIC', server);
  const user = server.user ?? '';
  const colon = user.indexOf(':');
  if (colon < 0 || !strictUUID(user.slice(0, colon))) {
    return invalid('TUIC', _('the user must be a UUID and a password'));
  }
  return valid();
}
