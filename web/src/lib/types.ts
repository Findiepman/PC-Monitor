export type UnitState =
  | 'running'
  | 'starting'
  | 'stopping'
  | 'restarting'
  | 'stopped'
  | 'failed'
  | 'paused'
  | 'unknown';

export type Action = 'start' | 'stop' | 'restart' | 'kill';

export interface Unit {
  id: string;
  provider: string;
  name: string;
  kind: string;
  state: UnitState;
  health: 'healthy' | 'unhealthy' | 'starting' | 'none' | string;
  uptime: number;
  cpu: number | null;
  mem: number | null;
  memLimit: number | null;
  actions: Action[];
  meta?: Record<string, string>;
}

export interface UnitsSnapshot {
  units: Unit[];
  errors: Record<string, string>;
  t: number;
}

export interface Disk {
  mount: string;
  used: number;
  total: number;
}

export interface HostStats {
  t: number;
  uptime: number;
  load: number[];
  cpu: number;
  cores: number[] | null;
  memUsed: number;
  memTotal: number;
  swapUsed: number;
  swapTotal: number;
  disks: Disk[] | null;
  netRx: number;
  netTx: number;
  temp: number | null;
}

export interface LogLine {
  t: number;
  err?: boolean;
  text: string;
}

export interface AuditEntry {
  time: string;
  user: string;
  ip: string;
  unit: string;
  action: Action;
  ok: boolean;
  error?: string;
}

export interface Provider {
  name: string;
  label: string;
}

export interface Meta {
  hostname: string;
  providers: Provider[];
}
