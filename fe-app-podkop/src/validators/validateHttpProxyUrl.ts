import { ValidationResult } from './types';
import { invalid, wrongStart, readServer, valid } from './linkAddress';

// http://[user:password@]host:port, https:// over TLS: a proxy has a port and no path, an
// address without one or with a path is a page or a subscription
export function validateHttpProxyUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['http', 'https']);
  if (start) return invalid('HTTP', start);
  const server = readServer(url, true);
  if (typeof server === 'string') return invalid('HTTP', server);
  if (server.path !== '' && server.path !== '/') {
    return invalid('HTTP', _('an address with a path is a page, not a proxy'));
  }
  return valid();
}
