const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

export function bytes(n: number | null | undefined, digits = 1): string {
  if (n == null) return '–';
  let i = 0;
  while (n >= 1024 && i < UNITS.length - 1) {
    n /= 1024;
    i++;
  }
  const d = i === 0 || n >= 100 ? 0 : digits;
  return `${n.toFixed(d)} ${UNITS[i]}`;
}

/** "11.2 / 32 GB", sharing the larger value's unit. */
export function bytesOf(used: number, total: number): string {
  let i = 0;
  let t = total;
  while (t >= 1024 && i < UNITS.length - 1) {
    t /= 1024;
    i++;
  }
  const scale = 1024 ** i;
  const u = used / scale;
  const fmt = (v: number) => (v >= 100 || Number.isInteger(v) ? v.toFixed(0) : v.toFixed(1));
  return `${fmt(u)} / ${fmt(t)} ${UNITS[i]}`;
}

export function rate(bps: number): string {
  return `${bytes(bps)}/s`;
}

export function pct(n: number | null | undefined): string {
  if (n == null) return '–';
  if (n < 10) return `${n.toFixed(1)}%`;
  return `${Math.round(n)}%`;
}

export function duration(sec: number): string {
  if (!sec || sec < 0) return '–';
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${Math.floor(sec)}s`;
}

const pad = (n: number, w = 2) => String(n).padStart(w, '0');

export function clock(ms: number): string {
  if (!ms) return '--:--:--';
  const d = new Date(ms);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** Local timestamp used when copying log lines. */
export function stamp(ms: number): string {
  if (!ms) return '';
  const d = new Date(ms);
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
  );
}

export const pastTense: Record<string, string> = {
  start: 'Started',
  stop: 'Stopped',
  restart: 'Restarted',
  kill: 'Killed',
};

export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
