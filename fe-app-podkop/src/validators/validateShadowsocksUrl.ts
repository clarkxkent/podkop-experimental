import { ValidationResult } from './types';
import {
  wrongStart,
  decodeBase64,
  invalid,
  linkParts,
  readAddress,
  valid,
  validPort,
} from './linkAddress';

// ss://userinfo@host:port/?plugin=…#name (SIP002): the method and the password as they are
// ("method:password", URL-escaped) or in base64; or the older form, everything before the
// name in base64 ("method:password@host:port")
export function validateShadowsocksUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['ss']);
  if (start) return invalid('Shadowsocks', start);
  let address = linkParts(url).address;
  if (!address.includes('@')) {
    const decoded = decodeBase64(address.replace(/\/$/, ''));
    if (!decoded || !decoded.includes('@')) {
      return invalid(
        'Shadowsocks',
        _('neither method:password@server nor base64 of it'),
      );
    }
    // the user of this form is not escaped: a password may have a "%" of its own
    const at = decoded.lastIndexOf('@');
    address = decoded.slice(0, at).replace(/%/g, '%25') + decoded.slice(at);
  }
  const server = readAddress(address);
  if (typeof server === 'string') return invalid('Shadowsocks', server);
  if (server.port === '')
    return invalid('Shadowsocks', _('missing server port'));
  if (!validPort(server.port))
    return invalid('Shadowsocks', _('invalid server port'));
  let credentials = server.user ?? '';
  if (!credentials.includes(':')) credentials = decodeBase64(credentials) ?? '';
  const colon = credentials.indexOf(':');
  if (colon <= 0)
    return invalid('Shadowsocks', _('missing method and password'));
  return valid();
}
