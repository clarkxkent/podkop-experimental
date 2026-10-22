import { describe, it, expect } from 'vitest';
import { validateShadowsocksUrl } from '../validateShadowsocksUrl';

const validUrls = [
  [
    'no-client',
    'ss://MjAyMi1ibGFrZTMtYWVzLTI1Ni1nY206ZG1DbHkvWmgxNVd3OStzK0dGWGlGVElrcHc3Yy9xQ0lTYUJyYWk3V2hoWT0@127.0.0.1:25144?type=tcp#shadowsocks-no-client',
  ],
  [
    'client',
    'ss://MjAyMi1ibGFrZTMtYWVzLTI1Ni1nY206S3FiWXZiNkhwb1RmTUt0N2VGcUZQSmJNNXBXaHlFU0ZKTXY2dEp1Ym1Fdz06dzRNMEx5RU9OTGQ5SWlkSGc0endTbzN2R3h4NS9aQ3hId0FpaWlxck5hcz0@127.0.0.1:26627?type=tcp#shadowsocks-client',
  ],
  [
    'plain-user',
    'ss://2022-blake3-aes-256-gcm:dmCly/Zh15Ww9+s+GFXiFTIkpw7c/qCISaBrai7WhhY=@127.0.0.1:27214?type=tcp#shadowsocks-plain-user',
  ],

  ['no parameters', 'ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:8388'],
  ['plain method and password', 'ss://aes-256-gcm:pass@[2001:db8::1]:8388#v6'],
  [
    'older form, all in base64',
    'ss://YWVzLTI1Ni1nY206cGFzc0AxMjcuMC4wLjE6ODM4OA#old',
  ],
  [
    'URL-safe base64 with a plugin',
    'ss://YWVzLTEyOC1nY206dGVzdA@192.0.2.1:8888/?plugin=obfs-local%3Bobfs%3Dhttp#plugin',
  ],
];

const invalidUrls = [
  ['No prefix', 'uuid@127.0.0.1:443?type=tcp'],
  ['No host', 'ss://YWVzLTI1Ni1nY206cGFzcw@:443?type=tcp'],
  ['No port', 'ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1?type=tcp'],
  ['Invalid port', 'ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:abc?type=tcp'],
  ['No method', 'ss://password@127.0.0.1:443'],
  ['Space in the server', 'ss://YWVzLTI1Ni1nY206cGFzcw@127.0. 0.1:443'],
  ['Older form without a server', 'ss://YWVzLTI1Ni1nY206cGFzcw'],
];

describe('validateShadowsocksUrl', () => {
  describe.each(validUrls)('Valid URL: %s', (_desc, url) => {
    it(`returns valid=true for "${url}"`, () => {
      const res = validateShadowsocksUrl(url);
      expect(res.valid).toBe(true);
    });
  });

  describe.each(invalidUrls)('Invalid URL: %s', (_desc, url) => {
    it(`returns valid=false for "${url}"`, () => {
      const res = validateShadowsocksUrl(url);
      expect(res.valid).toBe(false);
    });
  });

  it('detects invalid port range', () => {
    const res = validateShadowsocksUrl(
      'ss://password@127.0.0.1:99999?type=tcp',
    );
    expect(res.valid).toBe(false);
  });
});
