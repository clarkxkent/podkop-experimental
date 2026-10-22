import { ValidationResult } from './types';
import { invalid, wrongStart, readServer, valid } from './linkAddress';

// anytls://password@host[:port]?parameters#name: without a port, 443
export function validateAnytlsUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['anytls']);
  if (start) return invalid('AnyTLS', start);
  const server = readServer(url, false);
  if (typeof server === 'string') return invalid('AnyTLS', server);
  return valid();
}

// naive+https://user:password@host[:port]#name, naive+quic:// over QUIC: without a port, 443
export function validateNaiveUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['naive+https', 'naive+quic']);
  if (start) return invalid('NaiveProxy', start);
  const server = readServer(url, false);
  if (typeof server === 'string') return invalid('NaiveProxy', server);
  return valid();
}
