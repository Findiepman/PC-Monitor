import type { Unit } from './types';

export type Tone = 'ok' | 'warn' | 'fail' | 'idle';

const transitional = new Set(['starting', 'stopping', 'restarting']);

export function isTransitional(u: Unit): boolean {
  return transitional.has(u.state) || (u.state === 'running' && u.health === 'starting');
}

/** LED color for a unit. The only place status maps to color. */
export function tone(u: Unit): Tone {
  if (u.meta?.error) return 'fail';
  switch (u.state) {
    case 'running':
      if (u.health === 'unhealthy') return 'fail';
      if (u.health === 'starting') return 'warn';
      return 'ok';
    case 'failed':
      return 'fail';
    case 'starting':
    case 'stopping':
    case 'restarting':
    case 'paused':
      return 'warn';
    default:
      return 'idle';
  }
}

/** Short state word shown next to a unit. */
export function stateLabel(u: Unit): string {
  if (u.state === 'running' && u.health !== 'none') return u.health;
  return u.state;
}
