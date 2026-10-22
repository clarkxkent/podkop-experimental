import { ValidationResult } from './types';
import { invalid, wrongStart, readServer, valid } from './linkAddress';

// trojan://password@host:port?parameters#name
export function validateTrojanUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['trojan']);
  if (start) return invalid('Trojan', start);
  const server = readServer(url, true);
  if (typeof server === 'string') return invalid('Trojan', server);
  if (!server.user) return invalid('Trojan', _('missing password'));
  return valid();
}
