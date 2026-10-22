import { describe, expect, it } from 'vitest';
import { getYacdLinks } from '../getYacdLinks';

describe('getYacdLinks', () => {
  it('makes a link with the key for each IPv4 address', () => {
    expect(
      getYacdLinks(['192.168.1.1/24', '10.0.0.1', 'fd00::1/64'], 'k&e y'),
    ).toEqual([
      'http://192.168.1.1:9090/ui/?hostname=192.168.1.1&port=9090&secret=k%26e+y',
      'http://10.0.0.1:9090/ui/?hostname=10.0.0.1&port=9090&secret=k%26e+y',
    ]);
  });

  it('gives an address once and nothing for no address', () => {
    expect(getYacdLinks(['10.0.0.1/8', '10.0.0.1/24'], 'k')).toHaveLength(1);
    expect(getYacdLinks([], 'k')).toEqual([]);
  });
});
