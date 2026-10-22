import { beforeEach, describe, expect, it, vi } from 'vitest';

// the router, standing in: check_links answers by what a link holds
const asked: string[][] = [];
let answer: (links: string[]) => unknown = (links) => ({
  success: true,
  data: links.map((link) =>
    link.includes('type=kcp')
      ? { ok: false, error: 'the mKCP transport is not supported' }
      : { ok: true },
  ),
});
vi.mock('../methods', () => ({
  PodkopShellMethods: {
    checkLinks: async (links: string[]) => {
      asked.push(links);
      return answer(links);
    },
  },
}));
vi.mock('../services', () => ({ logger: { warn: () => {} } }));

const { linkPlaces, linkLabel, checkLinksBeforeSave, linkBatches } =
  await import('../linkCheck');

const GOOD = 'vless://b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443#DE-1';
const KCP =
  'vless://b831381d-6324-4d53-ad4f-8cda48b30811@[2001:db8::1]:443?type=kcp#NL%20kcp';

// a form of LuCI, standing in: the values of the fields by section, active or not
function form(
  values: Record<string, Record<string, unknown>>,
  inactive: string[] = [],
) {
  const options = ['proxy_string', 'urltest_proxy_links', 'domain_list'].map(
    (name) => ({
      option: name,
      title: name === 'urltest_proxy_links' ? 'URLTest Proxy Links' : name,
      isActive: (sectionId: string) =>
        !inactive.includes(`${sectionId}.${name}`) && name in values[sectionId],
      formvalue: (sectionId: string) => values[sectionId][name],
    }),
  );
  return {
    children: [
      { cfgsections: () => Object.keys(values), children: options },
      { children: [] },
    ],
  };
}

describe('linkPlaces', () => {
  it('lists the links of the active link fields, with their places', () => {
    const places = linkPlaces(
      form(
        {
          main: { proxy_string: `  ${GOOD}\n`, domain_list: 'example.org' },
          second: { urltest_proxy_links: [GOOD, '', KCP] },
          off: { proxy_string: KCP },
        },
        ['off.proxy_string'],
      ) as never,
    );
    expect(places).toEqual([
      {
        section: 'main',
        option: 'proxy_string',
        field: 'proxy_string',
        index: 1,
        count: 1,
        link: GOOD,
      },
      {
        section: 'second',
        option: 'urltest_proxy_links',
        field: 'URLTest Proxy Links',
        index: 1,
        count: 2,
        link: GOOD,
      },
      {
        section: 'second',
        option: 'urltest_proxy_links',
        field: 'URLTest Proxy Links',
        index: 2,
        count: 2,
        link: KCP,
      },
    ]);
  });
});

describe('linkLabel', () => {
  it('names a link by its name, kind and server, never its secrets', () => {
    expect(linkLabel(GOOD)).toBe('«DE-1» (vless, 192.0.2.1:443)');
    expect(linkLabel(KCP)).toBe('«NL kcp» (vless, [2001:db8::1]:443)');
    expect(linkLabel('trojan://secret@t.example.org:443')).toBe(
      'trojan, t.example.org:443',
    );
    expect(linkLabel('trojan://secret@t.example.org:443')).not.toContain(
      'secret',
    );
  });
});

describe('checkLinksBeforeSave', () => {
  beforeEach(() => {
    asked.length = 0;
  });

  it('asks once for the links of the form and returns the refused ones', async () => {
    const problems = await checkLinksBeforeSave(
      form({
        main: { proxy_string: KCP },
        second: { urltest_proxy_links: [GOOD, KCP] },
      }) as never,
    );
    expect(asked).toEqual([[KCP, GOOD]]);
    expect(
      problems.map((problem) => [
        problem.section,
        problem.index,
        problem.error,
      ]),
    ).toEqual([
      ['main', 1, 'the mKCP transport is not supported'],
      ['second', 2, 'the mKCP transport is not supported'],
    ]);
  });

  it('asks nothing when a link has a wrong form: the form refuses it itself', async () => {
    const problems = await checkLinksBeforeSave(
      form({
        main: { urltest_proxy_links: [KCP, 'vless://no-port@192.0.2.1'] },
      }) as never,
    );
    expect(problems).toEqual([]);
    expect(asked).toEqual([]);
  });

  it('asks for many long links in parts below the limit of uhttpd', async () => {
    const long = Array.from(
      { length: 150 },
      (_x, index) => `${GOOD}&extra=${'x'.repeat(500)}${index}`,
    );
    const problems = await checkLinksBeforeSave(
      form({ main: { urltest_proxy_links: [...long, KCP] } }) as never,
    );
    expect(asked.length).toBeGreaterThan(1);
    expect(asked.flat()).toEqual([...long, KCP]);
    for (const part of asked)
      expect(JSON.stringify(part).length).toBeLessThan(65536 - 1024);
    expect(problems.map((problem) => problem.index)).toEqual([151]);
  });

  it('keeps a link longer than a part alone', () => {
    expect(
      linkBatches(['a', 'x'.repeat(50000), 'b']).map((part) => part.length),
    ).toEqual([1, 1, 1]);
  });

  it('refuses nothing when the router does not answer', async () => {
    answer = () => ({ success: false, error: '' });
    expect(
      await checkLinksBeforeSave(
        form({ main: { proxy_string: KCP } }) as never,
      ),
    ).toEqual([]);
    answer = () => {
      throw new Error('timeout');
    };
    expect(
      await checkLinksBeforeSave(
        form({ main: { proxy_string: KCP } }) as never,
      ),
    ).toEqual([]);
  });
});
