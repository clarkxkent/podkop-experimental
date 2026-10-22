import { ValidationResult } from './types';
import { validateShadowsocksUrl } from './validateShadowsocksUrl';
import { validateVlessUrl } from './validateVlessUrl';
import { validateTrojanUrl } from './validateTrojanUrl';
import { validateSocksUrl } from './validateSocksUrl';
import {
  validateHysteria2Url,
  validateHysteriaUrl,
} from './validateHysteriaUrl';
import { validateVmessUrl } from './validateVmessUrl';
import { validateTuicUrl } from './validateTuicUrl';
import { validateHttpProxyUrl } from './validateHttpProxyUrl';
import { validateAnytlsUrl, validateNaiveUrl } from './validateTlsProxyUrl';

// The kinds of links podkop-engine reads (the full build; the slim one has no VMess, AnyTLS,
// Hysteria, NaiveProxy, which the router tells at the save), by scheme.
const VALIDATORS: Record<string, (url: string) => ValidationResult> = {
  vless: validateVlessUrl,
  vmess: validateVmessUrl,
  trojan: validateTrojanUrl,
  ss: validateShadowsocksUrl,
  socks: validateSocksUrl,
  socks4: validateSocksUrl,
  socks4a: validateSocksUrl,
  socks5: validateSocksUrl,
  socks5h: validateSocksUrl,
  http: validateHttpProxyUrl,
  https: validateHttpProxyUrl,
  hysteria2: validateHysteria2Url,
  hy2: validateHysteria2Url,
  hysteria: validateHysteriaUrl,
  tuic: validateTuicUrl,
  anytls: validateAnytlsUrl,
  'naive+https': validateNaiveUrl,
  'naive+quic': validateNaiveUrl,
};

export const PROXY_LINK_SCHEMES = Object.keys(VALIDATORS);

// validateProxyUrl checks one share link; empty lines around it are allowed, as the engine
// allows them
export function validateProxyUrl(url: string): ValidationResult {
  const lines = url
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean);
  if (lines.length > 1) {
    return { valid: false, message: _('One link per field') };
  }
  // trim takes a byte order mark too
  const link = lines[0] ?? '';
  const scheme = link.includes('://')
    ? link.slice(0, link.indexOf('://')).toLowerCase()
    : '';
  const validator = VALIDATORS[scheme];
  if (!validator) {
    return {
      valid: false,
      message: `${_('Unsupported link type')}: ${scheme ? scheme + '://' : link.slice(0, 12)}. ${_('Supported')}: vless, vmess, trojan, ss, socks, http(s), hysteria2/hy2, hysteria, tuic, anytls, naive+https/naive+quic`,
    };
  }
  return validator(link);
}
