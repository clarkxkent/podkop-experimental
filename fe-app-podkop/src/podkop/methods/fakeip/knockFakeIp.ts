import { FAKEIP_CHECK_SUFFIX } from '../../../constants';

// Opens a connection to a name made up for a check and returns the label of the name. The
// name does not exist: the request fails either way, sooner or later, and nothing waits
// for that. Whether its DNS question and the connection reached sing-box is asked of the
// backend, with the label (check_fakeip_client, check_nft_rules).
export function knockFakeIp(): string {
  const label = newLabel();
  knock(`https://${label}.${FAKEIP_CHECK_SUFFIX}/`);
  return label;
}

// Opens connections to a check name with a port of its own, at once and then about every
// 400 ms until stop is called: global_check traces the packets of that port while it waits
// for one of them, so the ones opened after its start are traced. The ports are those no
// browser refuses to connect to.
export function knockFakeIpRepeatedly(): {
  label: string;
  port: number;
  stop: () => void;
} {
  const label = newLabel();
  const port = 40000 + Math.floor(Math.random() * 10000);
  const url = `https://${label}.${FAKEIP_CHECK_SUFFIX}:${port}/`;
  knock(url);
  const interval = setInterval(() => knock(url), 400);

  return { label, port, stop: () => clearInterval(interval) };
}

function newLabel(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(10)), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
}

function knock(url: string): void {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 3000);

  fetch(url, {
    mode: 'no-cors',
    cache: 'no-store',
    signal: controller.signal,
  })
    .catch(() => undefined)
    .finally(() => clearTimeout(timer));
}
