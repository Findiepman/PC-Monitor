import { SIGNED_OUT } from './api';
import type { AuditEntry, HostStats, LogLine, Unit, UnitsSnapshot } from './types';

const HISTORY = 150;

type LogHandler = {
  tail: number;
  onLines: (lines: LogLine[]) => void;
  onEnd: (error?: string) => void;
};

type Status = 'connecting' | 'live' | 'reconnecting';

/** One websocket to dashd carrying host stats, unit state, audit and logs. */
class Live {
  status = $state<Status>('connecting');
  history = $state.raw<HostStats[]>([]);
  units = $state.raw<Unit[]>([]);
  errors = $state.raw<Record<string, string>>({});
  audit = $state.raw<AuditEntry[]>([]);
  /** performance.now() of the newest host sample, for smooth trace scrolling. */
  hostArrivedAt = 0;

  #ws: WebSocket | null = null;
  #attempt = 0;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #stopped = true;
  #logs = new Map<string, LogHandler>();

  connect() {
    this.#stopped = false;
    this.#open();
  }

  close() {
    this.#stopped = true;
    clearTimeout(this.#timer);
    this.#ws?.close();
    this.#ws = null;
    this.#logs.clear();
  }

  setAudit(entries: AuditEntry[]) {
    this.audit = entries;
  }

  /** Streams a unit's logs; returns a function that stops the stream. */
  subscribeLogs(unit: string, tail: number, onLines: LogHandler['onLines'], onEnd: LogHandler['onEnd']) {
    const handler = { tail, onLines, onEnd };
    this.#logs.set(unit, handler);
    this.#send({ op: 'logs', unit, tail });
    return () => {
      if (this.#logs.get(unit) === handler) {
        this.#logs.delete(unit);
        this.#send({ op: 'unlogs', unit });
      }
    };
  }

  #send(msg: object) {
    if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(JSON.stringify(msg));
  }

  #open() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${location.host}/ws`);
    this.#ws = ws;

    ws.onopen = () => {
      this.#attempt = 0;
      this.status = 'live';
      // Resume log streams after a reconnect. Lines already shown are
      // replaced by the fresh tail.
      for (const [unit, h] of this.#logs) this.#send({ op: 'logs', unit, tail: h.tail });
    };

    ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data);
      switch (msg.type) {
        case 'hello':
          this.history = msg.data.history ?? [];
          this.hostArrivedAt = performance.now();
          this.#applyUnits(msg.data.units);
          break;
        case 'host':
          this.history = [...this.history.slice(-(HISTORY - 1)), msg.data];
          this.hostArrivedAt = performance.now();
          break;
        case 'units':
          this.#applyUnits(msg.data);
          break;
        case 'audit':
          this.audit = [msg.data, ...this.audit].slice(0, 100);
          break;
        case 'logs':
          this.#logs.get(msg.data.unit)?.onLines(msg.data.lines);
          break;
        case 'logs_end':
          this.#logs.get(msg.data.unit)?.onEnd(msg.data.error);
          break;
      }
    };

    ws.onclose = (ev) => {
      if (this.#ws !== ws) return;
      this.#ws = null;
      if (this.#stopped) return;
      if (ev.code === 4001) {
        window.dispatchEvent(new Event(SIGNED_OUT));
        return;
      }
      this.status = 'reconnecting';
      // 1s, 2s, 4s ... capped at 15s, with jitter so tabs don't stampede.
      const delay = Math.min(15000, 1000 * 2 ** this.#attempt++) * (0.8 + Math.random() * 0.4);
      this.#timer = setTimeout(() => this.#open(), delay);
      // A failed upgrade looks like a close; check whether the session died.
      if (this.#attempt === 2) {
        fetch('/api/session', { credentials: 'same-origin' }).then((r) => {
          if (r.status === 401) window.dispatchEvent(new Event(SIGNED_OUT));
        }, () => {});
      }
    };
  }

  #applyUnits(snap: UnitsSnapshot) {
    this.units = snap.units ?? [];
    this.errors = snap.errors ?? {};
  }
}

export const live = new Live();
