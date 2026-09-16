<script lang="ts">
  import type { Action, Provider, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { tone } from '../lib/status';
  import UnitRow from './UnitRow.svelte';

  let {
    providers,
    selectedId,
    pending,
    armed,
    onselect,
    onaction,
  }: {
    providers: Provider[];
    selectedId: string | undefined;
    pending: Record<string, Action>;
    armed: { id: string; action: Action } | null;
    onselect: (u: Unit) => void;
    onaction: (u: Unit, a: Action) => void;
  } = $props();

  let groups = $derived(
    providers.map((p) => ({ ...p, units: live.units.filter((u) => u.provider === p.name) })),
  );

  let summary = $derived.by(() => {
    const c = { running: 0, problem: 0, stopped: 0 };
    for (const u of live.units) {
      const t = tone(u);
      if (t === 'fail') c.problem++;
      else if (u.state === 'running') c.running++;
      else if (t === 'idle') c.stopped++;
    }
    const parts = [`${c.running} running`];
    if (c.problem) parts.push(`${c.problem} need attention`);
    if (c.stopped) parts.push(`${c.stopped} stopped`);
    return parts.join(', ');
  });
</script>

<section class="rack" aria-label="Services">
  <div class="head">
    <h2>Services</h2>
    <span class="summary">{live.units.length ? summary : ''}</span>
  </div>

  <div class="cols" aria-hidden="true">
    <span>Name</span><span>State</span><span class="r">CPU</span><span class="r">Memory</span><span class="r up">Up</span><span></span>
  </div>

  {#each groups as g (g.name)}
    <div class="group">
      <h3>{g.label} <span class="count mono">{g.units.length}</span></h3>
      {#if live.errors[g.name]}
        <p class="provider-error">
          <span class="mono">{live.errors[g.name]}</span>
        </p>
      {:else if g.units.length === 0 && live.status === 'live'}
        <p class="empty">Nothing to show here yet.</p>
      {/if}
      <ul>
        {#each g.units as u (u.id)}
          <UnitRow
            unit={u}
            selected={u.id === selectedId}
            pending={pending[u.id]}
            armed={armed?.id === u.id ? armed.action : undefined}
            onselect={() => onselect(u)}
            onaction={(a) => onaction(u, a)}
          />
        {/each}
      </ul>
    </div>
  {/each}

  <p class="keys">
    <span><kbd>j</kbd> <kbd>k</kbd> move</span>
    <span><kbd>r</kbd> restart</span>
    <span><kbd>s</kbd> stop</span>
    <span><kbd>/</kbd> filter logs</span>
  </p>
</section>

<style>
  .rack {
    min-width: 0;
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    padding: 16px 16px 10px;
  }

  h2 {
    margin: 0;
    font-size: var(--fs-l);
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .summary {
    font-size: var(--fs-s);
    color: var(--dim);
  }

  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 84px 52px 116px 58px 100px;
    column-gap: 8px;
    padding: 0 8px 6px 36px;
    font-size: var(--fs-xs);
    color: var(--dim);
    border-bottom: 1px solid var(--rule);
  }

  .r {
    text-align: right;
  }

  h3 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin: 0;
    padding: 14px 16px 6px;
    font-size: var(--fs-s);
    font-weight: 600;
    color: var(--dim);
    border-bottom: 1px solid var(--rule);
  }

  .count {
    font-size: 11px;
    font-weight: 400;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .provider-error {
    margin: 0;
    padding: 10px 16px;
    border-bottom: 1px solid var(--rule);
    border-left: 2px solid var(--fail);
    background: var(--fail-soft);
    font-size: var(--fs-xs);
  }

  .empty {
    margin: 0;
    padding: 12px 16px;
    font-size: var(--fs-s);
    color: var(--dim);
    border-bottom: 1px solid var(--rule);
  }

  .keys {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 18px;
    margin: 0;
    padding: 14px 16px 20px;
    font-size: var(--fs-xs);
    color: var(--dim);
  }

  @media (max-width: 1180px) {
    .cols {
      grid-template-columns: minmax(0, 1fr) 76px 48px 72px 96px;
    }

    .up {
      display: none;
    }
  }

  @media (max-width: 899px) {
    .keys {
      display: none;
    }
  }

  @media (max-width: 560px) {
    .cols {
      display: none;
    }
  }
</style>
