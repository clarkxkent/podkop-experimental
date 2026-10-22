import { readFileSync } from 'fs';
import { describe, expect, it } from 'vitest';
import { validateProxyUrl } from '../validateProxyUrl';

// links of every kind podkop-engine reads, with only what each needs
const valid = [
  ['vless', 'vless://b831381d-6324-4d53-ad4f-8cda48b30811@[2001:db8::1]:443'],
  [
    'vmess, base64 of the JSON of v2rayN (a name in Cyrillic)',
    'vmess://eyJ2IjogIjIiLCAicHMiOiAi0J/RgNC40LLQtdGCIiwgImFkZCI6ICJ2bS5leGFtcGxlLm9yZyIsICJwb3J0IjogIjQ0MyIsICJpZCI6ICJiODMxMzgxZC02MzI0LTRkNTMtYWQ0Zi04Y2RhNDhiMzA4MTEiLCAiYWlkIjogIjAiLCAibmV0IjogIndzIiwgInBhdGgiOiAiL3dzIiwgInRscyI6ICJ0bHMifQ==',
  ],
  [
    'vmess, the URL form',
    'vmess://b831381d-6324-4d53-ad4f-8cda48b30811@vm.example.org:443?type=ws&security=tls#vm',
  ],
  ['trojan', 'trojan://secret@trojan.example.org:443#t'],
  ['ss', 'ss://YWVzLTI1Ni1nY206cGFzcw@192.0.2.1:8388#s'],
  ['socks5 without a port', 'socks5://192.0.2.1'],
  ['http with a user', 'http://user:pass@192.0.2.1:3128'],
  ['https with a slash', 'https://proxy.example.org:443/'],
  [
    'hysteria2 with ports',
    'hysteria2://auth@hy.example.org:443,20000-30000/?obfs=salamander&obfs-password=x',
  ],
  ['hy2 without auth and port', 'hy2://hy.example.org'],
  [
    'hysteria (1)',
    'hysteria://hy.example.org:443?protocol=udp&auth=x&upmbps=50&downmbps=100#h1',
  ],
  [
    'tuic without a port',
    'tuic://b831381d-6324-4d53-ad4f-8cda48b30811:pass@tuic.example.org?alpn=h3',
  ],
  [
    'tuic, a UUID without dashes',
    'tuic://b831381d63244d53ad4f8cda48b30811:pass@tuic.example.org:8443',
  ],
  ['anytls', 'anytls://pass@anytls.example.org:443?sni=anytls.example.org'],
  ['naive+https', 'naive+https://user:pass@naive.example.org#n'],
  ['naive+quic', 'naive+quic://user:pass@naive.example.org:443'],
  [
    'upper-case scheme',
    'VLESS://b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443',
  ],
  [
    'empty lines around',
    '\n  vless://b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443  \n',
  ],
];

const invalid = [
  ['nothing', ''],
  ['no scheme', 'b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443'],
  ['an unknown scheme', 'wireguard://key@192.0.2.1:51820'],
  ['two links', 'trojan://a@192.0.2.1:443\ntrojan://b@192.0.2.2:443'],
  [
    'vmess JSON without a server',
    'vmess://eyJwb3J0IjogIjQ0MyIsICJpZCI6ICJiODMxMzgxZC02MzI0LTRkNTMtYWQ0Zi04Y2RhNDhiMzA4MTEifQ==',
  ],
  [
    'vmess JSON with port 0',
    'vmess://eyJhZGQiOiAidm0uZXhhbXBsZS5vcmciLCAicG9ydCI6ICIwIiwgImlkIjogImI4MzEzODFkLTYzMjQtNGQ1My1hZDRmLThjZGE0OGIzMDgxMSJ9',
  ],
  ['http without a port: a page', 'http://example.org'],
  ['http with a path: a page', 'https://example.org:443/sub/list.txt'],
  ['hysteria without a port', 'hysteria://hy.example.org?auth=x'],
  [
    'tuic without a password',
    'tuic://b831381d-6324-4d53-ad4f-8cda48b30811@tuic.example.org',
  ],
  ['tuic with a user that is no UUID', 'tuic://user:pass@tuic.example.org'],
  ['trojan without a password', 'trojan://trojan.example.org:443'],
  ['anytls with a broken port', 'anytls://pass@anytls.example.org:0'],
];

describe('validateProxyUrl', () => {
  it.each(valid)('takes %s', (_name, url) => {
    const result = validateProxyUrl(url);
    expect(result.message).toBe('Valid');
    expect(result.valid).toBe(true);
  });

  it.each(invalid)('refuses %s', (_name, url) => {
    expect(validateProxyUrl(url).valid).toBe(false);
  });

  it('names the kind and what is missing', () => {
    expect(validateProxyUrl('vless://192.0.2.1:443').message).toBe(
      'VLESS: missing UUID',
    );
    expect(validateProxyUrl('trojan://x@192.0.2.1').message).toBe(
      'Trojan: missing server port',
    );
  });
});

// The corpus of the lab (LINK_CORPUS: [{link, ok, error}] decoded by podkop-engine,
// rs-lab/pk08/fe-corpus.py): every link the engine reads passes the page.
const corpus = process.env.LINK_CORPUS;
describe.skipIf(!corpus)('the links of the corpus', () => {
  it('every link the engine reads passes', () => {
    const rows: { link: string; ok: boolean; error: string }[] = JSON.parse(
      readFileSync(corpus as string, 'utf-8'),
    );
    const refused = rows.filter(
      (row) => row.ok && !validateProxyUrl(row.link).valid,
    );
    const passedRefusals = rows.filter(
      (row) => !row.ok && validateProxyUrl(row.link).valid,
    );
    console.log(
      `corpus: ${rows.length} links, ${rows.filter((row) => row.ok).length} read by the engine; ` +
        `the page refuses ${refused.length} of them; it passes ${passedRefusals.length} of the ${rows.filter((row) => !row.ok).length} the engine refuses`,
    );
    expect(
      refused.map(
        (row) =>
          `${validateProxyUrl(row.link).message}: ${row.link.slice(0, 90)}`,
      ),
    ).toEqual([]);
  });
});
