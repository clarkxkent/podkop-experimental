import { ValidationResult } from './types';
import { invalid, wrongStart, readServer, valid } from './linkAddress';

// socks5://[user:password@]host[:port]#name, also socks4, socks4a, socks5h and socks:// (the
// user in base64); without a port, 1080
export function validateSocksUrl(url: string): ValidationResult {
  const start = wrongStart(url, [
    'socks',
    'socks4',
    'socks4a',
    'socks5',
    'socks5h',
  ]);
  if (start) return invalid('SOCKS', start);
  const server = readServer(url, false);
  if (typeof server === 'string') return invalid('SOCKS', server);
  return valid();
}
