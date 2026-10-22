import { logger } from './logger.service';

// The page learns of the backend's errors from the system log: it asks logd for the last
// lines ("log read" through ubus) every 10 s while it is visible. logd answers in C: no program
// of podkop runs for it, as "podkop check_logs" did every 3 s. The cursor, the id of the last
// line seen, outlives a reload of the page (LuCI reloads it after Save & Apply, just as podkop
// restarts) for as long as the browser tab is open: a line is raised once.
//
// Not the subscription of uhttpd (GET /ubus/subscribe/log), which would push each line: in the
// uhttpd of OpenWrt 24.10 (2025.07.06) a subscription never lets its connection go, so uhttpd
// stops taking connections after max_connections of them, and it goes silent for good after
// network_timeout (30 s) without a line, its connection still open (fixed upstream in 2026).

export interface LogLine {
  id: number;
  time: number;
  msg: string;
}

const CURSOR_KEY = 'podkop-log-id';
const POLL_MS = 10000;
// the lines asked at each turn, and at the first one or when some were missed between turns
const FEW_LINES = 20;
const MANY_LINES = 200;
// a tab that has seen no line yet raises the errors of the last minutes only, not the old
// ones still in the log
const FIRST_WINDOW_MS = 10 * 60 * 1000;

function toLine(value: unknown): LogLine | undefined {
  const line = value as { id?: unknown; time?: unknown; msg?: unknown };
  if (typeof line?.msg !== 'string') return undefined;
  return {
    id: Number(line.id) || 0,
    time: Number(line.time) || 0,
    msg: line.msg,
  };
}

// catchUp picks out of the last lines of the log (oldest first) those after the cursor, the
// id of the last line seen, and gives the cursor after them. logd counts from 1 again after a
// reboot: a cursor beyond the newest line, like no cursor at all, takes the lines of the last
// minutes.
export function catchUp(
  lines: LogLine[],
  cursor: number | undefined,
): { lines: LogLine[]; cursor: number | undefined } {
  if (!lines.length) return { lines: [], cursor };
  const newest = lines[lines.length - 1];
  if (cursor === undefined || cursor > newest.id) {
    return {
      lines: lines.filter((line) => newest.time - line.time < FIRST_WINDOW_MS),
      cursor: newest.id,
    };
  }
  return {
    lines: lines.filter((line) => line.id > cursor),
    cursor: newest.id,
  };
}

// missedSome tells that the few last lines start after a gap: more lines came since the last
// turn than were asked, so the turn asks again for many.
export function missedSome(
  lines: LogLine[],
  cursor: number | undefined,
): boolean {
  return (
    cursor !== undefined &&
    lines.length > 0 &&
    lines[0].id > cursor + 1 &&
    lines[lines.length - 1].id >= cursor
  );
}

// isPodkopError tells a line of the backend ("podkop:" or "podkop[pid]:") that says [error] or
// [fatal]: the lines the page raises as notices.
export function isPodkopError(line: LogLine): boolean {
  if (!/^podkop(\[\d+\])?:/.test(line.msg)) return false;
  const text = line.msg.toLowerCase();
  return text.includes('[error]') || text.includes('[fatal]');
}

function readCursor(): number | undefined {
  try {
    const value = sessionStorage.getItem(CURSOR_KEY);
    return value ? Number(value) : undefined;
  } catch {
    return undefined;
  }
}

function writeCursor(id: number): void {
  try {
    sessionStorage.setItem(CURSOR_KEY, String(id));
  } catch {
    // no storage: a line is still not raised twice while the page is open
  }
}

let readLog:
  | ((lines: number, stream: boolean) => Promise<{ log?: unknown[] }>)
  | undefined;

async function lastLines(count: number): Promise<LogLine[]> {
  readLog ??= rpc.declare<{ log?: unknown[] }>({
    object: 'log',
    method: 'read',
    params: ['lines', 'stream'],
  });
  const reply = await readLog(count, false);
  return (Array.isArray(reply?.log) ? reply.log : [])
    .map(toLine)
    .filter((line): line is LogLine => line !== undefined);
}

// watchLog calls onLine for every new line of the system log while the page is open; a hidden
// page asks nothing and catches up when it is shown again.
export function watchLog(onLine: (line: LogLine) => void): void {
  let cursor = readCursor();
  let first = true;
  let busy = false;

  const turn = async () => {
    if (busy || document.hidden) return;
    busy = true;
    try {
      let lines = await lastLines(first ? MANY_LINES : FEW_LINES);
      if (missedSome(lines, cursor)) lines = await lastLines(MANY_LINES);
      first = false;
      const fresh = catchUp(lines, cursor);
      if (fresh.cursor !== undefined && fresh.cursor !== cursor) {
        cursor = fresh.cursor;
        writeCursor(fresh.cursor);
      }
      fresh.lines.forEach(onLine);
    } catch (err) {
      logger.warn('[LOG]', 'the system log is not read:', err);
    } finally {
      busy = false;
    }
  };

  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) turn();
  });
  setInterval(turn, POLL_MS);
  turn();
}
