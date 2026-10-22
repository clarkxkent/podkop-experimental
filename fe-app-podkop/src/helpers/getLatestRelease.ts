// The tag of the latest release of podkop-exp, asked of GitHub by the browser (its API allows
// any origin) and kept for an hour: the router asks nothing outside, and the limit of the API
// (60 requests an hour from one address) is not spent at every opening of a tab. A failed
// request is kept too, for 10 minutes, so that a browser that cannot reach GitHub does not
// wait for it at every opening. Empty when it is not known.
const RELEASE_URL =
  'https://api.github.com/repos/podkop-net/podkop-experimental/releases/latest';
const CACHE_KEY = 'podkop-latest-release';
const CACHE_MS = 60 * 60 * 1000;
const FAILURE_MS = 10 * 60 * 1000;

interface Cached {
  tag: string;
  at: number;
}

// readCachedRelease gives the kept tag ('' for a kept failure), or undefined when nothing
// usable is kept.
export function readCachedRelease(
  now: number,
  raw: string | null,
): string | undefined {
  if (!raw) return undefined;
  try {
    const cached = JSON.parse(raw) as Cached;
    if (typeof cached.tag !== 'string' || typeof cached.at !== 'number') {
      return undefined;
    }
    if (now - cached.at < (cached.tag ? CACHE_MS : FAILURE_MS)) {
      return cached.tag;
    }
  } catch {
    // a broken value is asked again
  }
  return undefined;
}

function keep(tag: string) {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify({ tag, at: Date.now() }));
  } catch {
    // no storage
  }
}

async function ask(): Promise<string> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(RELEASE_URL, { signal: controller.signal });
    if (!response.ok) return '';
    const tag = ((await response.json()) as { tag_name?: string }).tag_name;
    return typeof tag === 'string' ? tag : '';
  } catch {
    return '';
  } finally {
    clearTimeout(timer);
  }
}

export async function getLatestRelease(): Promise<string> {
  let stored: string | null = null;
  try {
    stored = localStorage.getItem(CACHE_KEY);
  } catch {
    // no storage: asked every time
  }
  const cached = readCachedRelease(Date.now(), stored);
  if (cached !== undefined) return cached;
  const tag = await ask();
  keep(tag);
  return tag;
}
