import { ValidationResult } from './types';
import {
  wrongStart,
  decodeBase64,
  invalid,
  readServer,
  valid,
  validPort,
  validUserID,
} from './linkAddress';

// vmess://base64 of the JSON of v2rayN ("add", "port", "id", …), or the URL form
// vmess://uuid@host:port?parameters#name
export function validateVmessUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['vmess']);
  if (start) return invalid('VMess', start);
  const body = url.slice('vmess://'.length).split('#')[0];
  const decoded = decodeBase64(body);
  if (decoded === undefined || !decoded.trim().startsWith('{')) {
    const server = readServer(url, true);
    if (typeof server === 'string') return invalid('VMess', server);
    if (!server.user) return invalid('VMess', _('missing UUID'));
    if (!validUserID(server.user)) return invalid('VMess', _('invalid UUID'));
    return valid();
  }
  let link: Record<string, unknown>;
  try {
    link = JSON.parse(decoded);
  } catch (_e) {
    return invalid('VMess', _('the base64 of the link is no JSON'));
  }
  const text = (value: unknown) =>
    typeof value === 'string' || typeof value === 'number' ? String(value) : '';
  if (!text(link.add)) return invalid('VMess', _('missing server address'));
  if (!validPort(text(link.port)))
    return invalid('VMess', _('invalid server port'));
  if (!text(link.id)) return invalid('VMess', _('missing UUID'));
  if (!validUserID(text(link.id))) return invalid('VMess', _('invalid UUID'));
  return valid();
}
