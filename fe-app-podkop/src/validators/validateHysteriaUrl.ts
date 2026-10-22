import { ValidationResult } from './types';
import {
  wrongStart,
  invalid,
  linkParts,
  readAddress,
  readServer,
  valid,
  validPortList,
} from './linkAddress';

// hysteria2://[auth@]host[:ports]/?parameters#name, also hy2://: without a port, 443; the
// port may be a list of ports and ranges ("443,8443-8450")
export function validateHysteria2Url(url: string): ValidationResult {
  const start = wrongStart(url, ['hysteria2', 'hy2']);
  if (start) return invalid('Hysteria2', start);
  const server = readAddress(linkParts(url).address);
  if (typeof server === 'string') return invalid('Hysteria2', server);
  if (
    server.port !== '' &&
    !(/^[\d,-]+$/.test(server.port) && validPortList(server.port))
  ) {
    return invalid('Hysteria2', _('invalid server port'));
  }
  return valid();
}

// hysteria://host:port?auth=…&upmbps=…#name, Hysteria 1
export function validateHysteriaUrl(url: string): ValidationResult {
  const start = wrongStart(url, ['hysteria']);
  if (start) return invalid('Hysteria', start);
  const server = readServer(url, true);
  if (typeof server === 'string') return invalid('Hysteria', server);
  return valid();
}
