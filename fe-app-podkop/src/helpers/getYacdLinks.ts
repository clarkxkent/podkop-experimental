import { CLASH_API_PORT } from '../constants';

// getYacdLinks makes the links that open YACD with its key: one for each IPv4 address of the
// devices YACD is opened from (addresses as LuCI gives them, with or without the length of
// the prefix). YACD takes the address of the API and the key from the query and logs in.
export function getYacdLinks(addresses: string[], secret: string): string[] {
  const seen = new Set<string>();
  const links: string[] = [];
  for (const address of addresses) {
    const ip = address.split('/')[0];
    if (!/^\d{1,3}(\.\d{1,3}){3}$/.test(ip) || seen.has(ip)) continue;
    seen.add(ip);
    const query = new URLSearchParams({
      hostname: ip,
      port: String(CLASH_API_PORT),
      secret,
    });
    links.push(`http://${ip}:${CLASH_API_PORT}/ui/?${query.toString()}`);
  }
  return links;
}
