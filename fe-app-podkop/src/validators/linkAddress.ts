import { ValidationResult } from './types';

// The checks of share links look only at what every working link of its kind has: the
// server, the port where the kind has no default, the user (UUID, password) where the kind
// needs one. They take the link apart as podkop-engine reads it ("sing-box tools
// decode-link"), so that a link the engine reads passes. What the engine of the router can
// do with the parameters (transport, security, flow) and with the kind itself (the slim
// build has no VMess, AnyTLS, Hysteria, NaiveProxy), the router tells at the save.

export interface LinkAddress {
  // the user before the last "@", URL-decoded; undefined when there is no "@"
  user?: string;
  host: string;
  // the port as written, "" when there is none
  port: string;
  // the path after the first "/" of the address, with the "/"
  path: string;
}

export function invalid(label: string, message: string): ValidationResult {
  return { valid: false, message: `${label}: ${message}` };
}

export function valid(): ValidationResult {
  return { valid: true, message: _('Valid') };
}

// linkParts splits a link into its address (between "://" and "?"), its parameters and its
// name; the name is free text.
export function linkParts(url: string): { address: string; query: string } {
  let rest = url.slice(url.indexOf('://') + 3);
  const hash = rest.indexOf('#');
  if (hash >= 0) rest = rest.slice(0, hash);
  const question = rest.indexOf('?');
  if (question < 0) return { address: rest, query: '' };
  return { address: rest.slice(0, question), query: rest.slice(question + 1) };
}

// schemeOf returns the scheme of a link in lower case, "" when it has none.
export function schemeOf(url: string): string {
  const end = url.indexOf('://');
  return end > 0 ? url.slice(0, end).toLowerCase() : '';
}

// wrongStart checks the scheme. It returns the message of what is wrong, or "". White space
// is looked at in the address only (readAddress), as the engine does: links of panels
// carry JSON with spaces in their parameters ("extra", "fm"), and the name after "#" is
// free text.
export function wrongStart(url: string, schemes: string[]): string {
  if (!schemes.includes(schemeOf(url))) {
    return `${_('must start with')} ${schemes.map((scheme) => scheme + '://').join(', ')}`;
  }
  return '';
}

// validPort tells a port number, 1–65535.
export function validPort(port: string): boolean {
  return /^\d{1,5}$/.test(port) && Number(port) >= 1 && Number(port) <= 65535;
}

// validPortList tells a list of ports and ranges of Hysteria 2: "443", "443,8443-8450",
// "20000:30000".
export function validPortList(list: string): boolean {
  const items = list
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
  return (
    items.length > 0 &&
    items.every((item) => item.split(/[-:]/).every((port) => validPort(port)))
  );
}

// validHost tells a server: an IPv4 address or a name (a domain, or a name of the LAN), or
// an IPv6 address, which a link writes in brackets.
export function validHost(host: string, bracketed: boolean): boolean {
  if (bracketed)
    return /^[0-9a-fA-F:.]+(%[\w.-]+)?$/.test(host) && host.includes(':');
  return /^[\p{L}\p{N}._-]+$/u.test(host);
}

// readAddress reads "[user@]host[:port][/path]". It returns the message of what is wrong,
// or the parts.
export function readAddress(address: string): LinkAddress | string {
  if (/\s/.test(address)) return _('must not contain spaces');
  let user: string | undefined;
  let hostPort = address;
  const at = address.lastIndexOf('@');
  if (at >= 0) {
    try {
      user = decodeURIComponent(address.slice(0, at));
    } catch (_e) {
      return _('invalid escape in the user');
    }
    hostPort = address.slice(at + 1);
  }
  let path = '';
  const slash = hostPort.indexOf('/');
  if (slash >= 0) {
    path = hostPort.slice(slash);
    hostPort = hostPort.slice(0, slash);
  }
  let host: string;
  let port = '';
  let bracketed = false;
  if (hostPort.startsWith('[')) {
    const end = hostPort.indexOf(']');
    if (end < 0) return _('invalid server address');
    host = hostPort.slice(1, end);
    bracketed = true;
    const after = hostPort.slice(end + 1);
    if (after && !after.startsWith(':')) return _('invalid server address');
    port = after.slice(1);
  } else {
    const colon = hostPort.indexOf(':');
    host = colon >= 0 ? hostPort.slice(0, colon) : hostPort;
    port = colon >= 0 ? hostPort.slice(colon + 1) : '';
  }
  if (!host) return _('missing server address');
  if (!validHost(host, bracketed)) return _('invalid server address');
  return { user, host, port, path };
}

// readServer reads the address of a link of a kind with or without a default port
// ("required": none) and checks the port.
export function readServer(
  url: string,
  portRequired: boolean,
): LinkAddress | string {
  const address = readAddress(linkParts(url).address);
  if (typeof address === 'string') return address;
  if (address.port === '') {
    return portRequired ? _('missing server port') : address;
  }
  return validPort(address.port) ? address : _('invalid server port');
}

// validUserID tells the user of VLESS and VMess: a UUID, or a text of up to 30 characters,
// which Xray and the engine turn into a UUID.
export function validUserID(user: string): boolean {
  if (user.length <= 30) return true;
  return /^[0-9a-fA-F]{8}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{4}-?[0-9a-fA-F]{12}$/.test(
    user,
  );
}

// strictUUID tells a UUID as TUIC needs it: 32 hex digits, or 36 characters with the dashes.
export function strictUUID(text: string): boolean {
  return (
    /^[0-9a-fA-F]{32}$/.test(text) ||
    /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(
      text,
    )
  );
}

// decodeBase64 decodes standard or URL-safe base64, padded or not; undefined when it is not
// base64.
export function decodeBase64(text: string): string | undefined {
  const cleaned = text.trim().replace(/-/g, '+').replace(/_/g, '/');
  if (!cleaned || !/^[A-Za-z0-9+/]+={0,2}$/.test(cleaned)) return undefined;
  try {
    return atob(cleaned);
  } catch (_e) {
    return undefined;
  }
}
