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

<li class="unit" class:selected class:problem={t === 'fail'} aria-current={selected ? 'true' : undefined}>
  <button class="pick" onclick={onselect} aria-label="Show logs for {unit.name}, {stateLabel(unit)}">
    <Led tone={t} pulse={busy} />
    <span class="names">
      <span class="name">{unit.name}</span>
      <span class="detail mono">{detail}</span>
    </span>
  </button>

  <span class="state" class:bad={t === 'fail'}>
    {pending ? progressive[pending] : stateLabel(unit)}
  </span>
  <span class="num mono cpu" title="CPU, 100% is one core">{pct(unit.cpu)}</span>
  <span class="num mono mem" title="Memory">{memText}</span>
  <span class="num mono up" title="Uptime">{unit.state === 'running' ? duration(unit.uptime) : '–'}</span>

  <span class="actions">
    {#if armed}
      <span class="armed" role="status">Press {armed === 'restart' ? 'r' : 's'} again to {armed}</span>
    {:else}
      {#if can('start')}
        <button class="act" disabled={busy} title="Start {unit.name}" aria-label="Start {unit.name}" onclick={() => onaction('start')}>
          <Icon name="start" />
        </button>
      {/if}
      {#if can('restart')}
        <HoldButton label="Restart {unit.name}" disabled={busy} onconfirm={() => onaction('restart')}>
          <Icon name="restart" />
        </HoldButton>
      {/if}
      {#if can('stop')}
        <HoldButton label="Stop {unit.name}" danger disabled={busy} onconfirm={() => onaction('stop')}>
          <Icon name="stop" />
        </HoldButton>
      {/if}
    {/if}
  </span>

  {#if unit.meta?.error}
    <span class="error mono">{unit.meta.error}</span>
  {/if}
</li>

<style>
  .unit {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 84px 52px 116px 58px 100px;
    align-items: center;
    column-gap: 8px;
    min-height: 48px;
    padding: 0 8px 0 0;
    border-bottom: 1px solid var(--rule);
    background: var(--plate);
    box-shadow: inset 2px 0 0 transparent;
  }

  .unit:hover {
    background: var(--plate-hi);
  }

  .selected,
  .selected:hover {
    background: var(--plate-hi);
    box-shadow: inset 2px 0 0 var(--trace);
  }

  .pick {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
    height: 100%;
    min-height: 48px;
    padding: 6px 0 6px 16px;
    border: 0;
    background: none;
    text-align: left;
  }

  .pick:focus-visible {
    outline-offset: -2px;
  }

  .names {
    display: grid;
    min-width: 0;
  }

  .name {
    font-size: var(--fs-m);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .detail {
    font-size: 11px;
    color: var(--dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .state {
    font-size: var(--fs-s);
    color: var(--dim);
  }

  .state.bad {
    color: var(--fail);
  }

  .num {
    font-size: var(--fs-xs);
    text-align: right;
    white-space: nowrap;
  }

  .actions {
    position: relative;
    display: flex;
    justify-content: flex-end;
    gap: 2px;
    min-height: 30px;
  }

  .act {
    display: inline-grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: 5px;
    background: transparent;
    color: var(--dim);
  }

  .act:hover:not(:disabled) {
    color: var(--ink);
    border-color: var(--rule);
  }

  .act:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .armed {
    position: absolute;
    right: 0;
    top: 50%;
    transform: translateY(-50%);
    padding: 4px 8px;
    border: 1px solid var(--warn);
    border-radius: 5px;
    background: var(--plate-hi);
    font-size: var(--fs-xs);
    color: var(--warn);
    white-space: nowrap;
  }

  .error {
    grid-column: 1 / -1;
    padding: 0 16px 8px 36px;
    font-size: 11px;
    color: var(--fail);
  }

  @media (max-width: 1180px) {
    .unit {
      grid-template-columns: minmax(0, 1fr) 76px 48px 72px 96px;
    }

    .up {
      display: none;
    }

    .mem {
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  @media (max-width: 560px) {
    .unit {
      grid-template-columns: minmax(0, 1fr) auto;
      grid-template-rows: auto auto;
      row-gap: 0;
    }

    .pick {
      grid-row: 1 / 3;
    }

    .state {
      grid-column: 2;
      text-align: right;
      padding-top: 8px;
    }

    .cpu,
    .mem,
    .up {
      display: none;
    }

    .actions {
      grid-column: 2;
      padding-bottom: 6px;
    }
  }
</style>
