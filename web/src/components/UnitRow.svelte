<script lang="ts">
  import type { Action, Unit } from '../lib/types';
  import { bytes, duration, pct } from '../lib/format';
  import { isTransitional, stateLabel, tone } from '../lib/status';
  import Led from './Led.svelte';
  import HoldButton from './HoldButton.svelte';
  import Icon from './Icon.svelte';

  let {
    unit,
    selected,
    pending,
    armed,
    onselect,
    onaction,
  }: {
    unit: Unit;
    selected: boolean;
    pending: Action | undefined;
    armed: Action | undefined;
    onselect: () => void;
    onaction: (a: Action) => void;
  } = $props();

  let t = $derived(tone(unit));
  let busy = $derived(!!pending || isTransitional(unit));

  let detail = $derived.by(() => {
    const m = unit.meta ?? {};
    const bits = [m.image ?? m.description ?? m.node ?? unit.kind];
    // A bot's Discord name says more than its pm2 name.
    if (m.discord) bits.unshift(m.discord);
    if (m.note) bits.push(m.note);
    if (m.exit && m.exit !== '0') bits.push(`exit ${m.exit}`);
    if (m.restarts) bits.push(`${m.restarts} restarts`);
    return bits.join(', ');
  });

  let memText = $derived(
    unit.mem == null ? '–' : unit.memLimit ? `${bytes(unit.mem)} / ${bytes(unit.memLimit, 0)}` : bytes(unit.mem),
  );

  const can = (a: Action) => unit.actions.includes(a);
  const progressive: Record<Action, string> = {
    start: 'starting…',
    stop: 'stopping…',
    restart: 'restarting…',
    kill: 'killing…',
  };
</script>

<li
  class="unit ucard"
  class:selected
  class:problem={t === 'fail'}
  class:stopped={unit.state === 'stopped'}
  aria-current={selected ? 'true' : undefined}
>
  <div class="uh">
    <button class="pick" onclick={onselect} aria-label="{unit.kind === 'bot' ? 'Select' : 'Show logs for'} {unit.name}, {stateLabel(unit)}">
      <span class="name">{unit.name}</span>
    </button>
    <span class="state pill {busy ? 'warn' : t === 'fail' ? 'fail' : t === 'ok' ? 'ok' : t === 'warn' ? 'warn' : 'idle'}">
      {pending ? progressive[pending] : stateLabel(unit)}
    </span>
  </div>

  {#if unit.about}
    <p class="about" title={unit.about}>{unit.about}</p>
  {/if}

  <div class="umeta">
    <span class="detail">{detail}</span>
    <span title="Uptime">up {unit.state === 'running' ? duration(unit.uptime) : '–'}</span>
  </div>

  <div class="umet">
    <span>CPU</span>
    <div class="bar" title="CPU, 100% is one core"><i style:width="{unit.cpu == null ? 0 : Math.min(100, Math.max(0, unit.cpu))}%"></i></div>
    <span>Mem</span>
    <div class="bar" title="Memory{unit.memLimit ? '' : ' (no limit set)'}">
      <i style:width="{unit.mem != null && unit.memLimit ? Math.min(100, (unit.mem / unit.memLimit) * 100) : 0}%"></i>
    </div>
  </div>
  <div class="umeta nums">
    <span class="mono">{pct(unit.cpu)} cpu</span>
    <span class="mono">{memText}</span>
  </div>

  {#if unit.meta?.error}
    <span class="uerr mono" role="alert">{unit.meta.error}</span>
  {/if}

  <div class="uacts">
    {#if armed}
      <span class="armed" role="status">Press {armed === 'restart' ? 'r' : 's'} again to {armed}</span>
    {:else}
      <!-- Every action needs a deliberate hold, so a stray tap never starts, restarts or stops anything. -->
      {#if can('start')}
        <HoldButton label="Start {unit.name}" tone="mint" disabled={busy} onconfirm={() => onaction('start')}>
          <Icon name="start" size={16} /><span class="lbl">Start</span>
        </HoldButton>
      {/if}
      {#if can('restart')}
        <HoldButton label="Restart {unit.name}" tone="lemon" disabled={busy} onconfirm={() => onaction('restart')}>
          <Icon name="restart" size={16} /><span class="lbl">Restart</span>
        </HoldButton>
      {/if}
      {#if can('stop')}
        <HoldButton label="Stop {unit.name}" tone="tomato" danger disabled={busy} onconfirm={() => onaction('stop')}>
          <Icon name="stop" size={16} /><span class="lbl">Stop</span>
        </HoldButton>
      {/if}
    {/if}
  </div>
</li>

<style>
  .ucard {
    position: relative;
    display: grid;
    gap: 8px;
    min-width: 0;
    padding: 12px;
    border: 3px solid var(--line);
    border-radius: 16px;
    background: var(--paper);
    color: var(--fg);
    box-shadow: 3px 3px 0 var(--line);
    transition:
      transform 0.15s cubic-bezier(0.3, 1.7, 0.5, 1),
      box-shadow 0.15s,
      background 0.2s;
  }

  .selected {
    background: var(--lemon);
    color: #111;
    --fg: #111;
    --line: #111;
    --sub: rgb(17 17 17 / 0.72);
    transform: translate(-2px, -2px);
    box-shadow: 6px 6px 0 var(--line);
  }

  .uh {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-width: 0;
  }

  .pick {
    min-width: 0;
    min-height: 44px;
    padding: 0;
    border: 0;
    border-radius: 6px;
    background: none;
    color: inherit;
    text-align: left;
  }

  .pick:focus-visible {
    outline-offset: 2px;
  }

  .name {
    display: block;
    font-size: 20px;
    font-weight: 800;
    letter-spacing: -0.02em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stopped .name {
    opacity: 0.75;
  }

  .state {
    flex: none;
    max-width: 55%;
    min-height: 26px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .about {
    margin: -4px 0 0;
    min-width: 0;
    font-size: 13px;
    font-weight: 500;
    line-height: 1.3;
    color: var(--sub);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .umeta {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 10px;
    min-width: 0;
    font-size: 13px;
    font-weight: 600;
    color: var(--sub);
  }

  .detail {
    min-width: 0;
    font-family: var(--mono);
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .nums span {
    font-size: 12px;
  }

  .umet {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 3px 8px;
    align-items: center;
    font-size: 12.5px;
    font-weight: 700;
  }

  .bar {
    height: 12px;
    border: 2.5px solid var(--line);
    border-radius: 999px;
    background: rgb(128 128 128 / 0.25);
    overflow: hidden;
  }

  .bar i {
    display: block;
    height: 100%;
    background: var(--fg);
    border-radius: 999px 0 0 999px;
    transition: width 0.6s cubic-bezier(0.3, 1.4, 0.5, 1);
  }

  .uerr {
    padding: 4px 8px;
    border: 2.5px solid #111;
    border-radius: 10px;
    background: var(--tomato);
    color: #111;
    font-size: 12px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .uacts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    min-height: 40px;
  }

  .armed {
    padding: 6px 10px;
    border: 3px solid #111;
    border-radius: 12px;
    background: var(--lemon);
    box-shadow: 3px 3px 0 #111;
    color: #111;
    font-size: var(--fs-s);
    font-weight: 700;
  }

  @media (prefers-reduced-motion: reduce) {
    .ucard,
    .bar i {
      transition: none;
    }
  }
</style>
