<script lang="ts">
  import type { Action, Provider, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { tone } from '../lib/status';
  import UnitRow from './UnitRow.svelte';

  let {
    providers,
    metaState = 'ok',
    selectedId,
    pending,
    armed,
    filter = $bindable('all'),
    onselect,
    onaction,
  }: {
    providers: Provider[];
    metaState?: 'loading' | 'ok' | 'failed';
    selectedId: string | undefined;
    pending: Record<string, Action>;
    armed: { id: string; action: Action } | null;
    filter?: string;
    onselect: (u: Unit) => void;
    onaction: (u: Unit, a: Action) => void;
  } = $props();

  // Only the units of the providers the config lists.
  let units = $derived(live.units.filter((u) => providers.some((p) => p.name === u.provider)));
  let groups = $derived(providers.map((p) => ({ ...p, units: units.filter((u) => u.provider === p.name) })));
  let shown = $derived(filter === 'all' ? groups : groups.filter((g) => g.name === filter));

  // A provider that disappears from the config can't stay selected as a filter.
  $effect(() => {
    if (filter !== 'all' && providers.length && !providers.some((p) => p.name === filter)) filter = 'all';
  });

  let loading = $derived(
    metaState === 'loading' || (live.status !== 'live' && units.length === 0 && metaState !== 'failed'),
  );

  let summary = $derived.by(() => {
    const c = { running: 0, problem: 0, stopped: 0 };
    for (const u of units) {
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

<section class="tile t-units" aria-labelledby="h-units">
  <header class="th">
    <h2 id="h-units">Services</h2>
    {#if units.length}<span class="summary pill plain">{summary}</span>{/if}
  </header>

  {#if providers.length > 1}
    <div class="filters" role="group" aria-label="Filter by provider">
      <button class="chipbtn" aria-pressed={filter === 'all'} onclick={() => (filter = 'all')}>
        All<b>{units.length}</b>
      </button>
      {#each groups as g (g.name)}
        <button class="chipbtn" aria-pressed={filter === g.name} onclick={() => (filter = g.name)}>
          {g.label}<b>{g.units.length}</b>
        </button>
      {/each}
    </div>
  {/if}

  <p class="hint keys">
    <span>Hold Start, Restart or Stop until the fill completes.</span>
    <span class="kbds"><kbd>j</kbd> <kbd>k</kbd> move, <kbd>r</kbd> restart, <kbd>s</kbd> stop, <kbd>/</kbd> filter logs</span>
  </p>
  <p class="hint touch">Press and hold a button to start, restart or stop. A quick tap does nothing.</p>

  {#if loading}
    <div class="ugrid" aria-busy="true" aria-label="Loading services">
      <div class="skel"></div><div class="skel"></div><div class="skel"></div>
    </div>
  {:else if metaState === 'failed' && providers.length === 0}
    <div class="empty" role="alert"><b>Couldn't load the service list</b><p>Reload the page to try again.</p></div>
  {:else if providers.length === 0}
    <div class="empty"><b>No providers configured</b><p>Add a provider in the dashd config and its services will show up here.</p></div>
  {/if}

  {#each shown as g (g.name)}
    <div class="group g-{g.name}">
      <h3 class="ghead">
        <span class="gtag">{g.label}</span>
        <small>{g.units.length} {g.units.length === 1 ? 'service' : 'services'}</small>
      </h3>
      {#if live.errors[g.name]}
        <p class="banner" role="alert"><span class="mono">{live.errors[g.name]}</span></p>
      {:else if g.units.length === 0 && live.status === 'live'}
        <div class="empty"><b>Nothing to show here yet</b></div>
      {/if}
      {#if g.units.length}
        <ul class="ugrid">
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
      {/if}
    </div>
  {/each}
</section>

<style>
  .summary {
    min-height: 28px;
    white-space: normal;
    background: var(--mint);
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .chipbtn {
    min-height: 40px;
    padding: 2px 14px;
    border: 3px solid var(--line);
    border-radius: 999px;
    background: var(--card);
    color: var(--fg);
    font-size: var(--fs-s);
    font-weight: 800;
    box-shadow: 2px 2px 0 var(--line);
    transition:
      transform 0.08s,
      box-shadow 0.08s;
    touch-action: manipulation;
  }

  .chipbtn:active {
    transform: translate(2px, 2px);
    box-shadow: none;
  }

  .chipbtn[aria-pressed='true'] {
    background: var(--lemon);
    color: #111;
  }

  .chipbtn b {
    margin-left: 6px;
    opacity: 0.7;
    font-variant-numeric: tabular-nums;
  }

  .hint {
    font-size: 13px;
    font-weight: 600;
    color: var(--sub);
  }

  .keys {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 14px;
  }

  .touch {
    display: none;
  }

  @media (pointer: coarse) {
    .touch {
      display: block;
    }
    .keys {
      display: none;
    }
  }

  .group {
    display: grid;
    gap: 10px;
  }

  .group + .group,
  .ugrid + .group {
    margin-top: 8px;
  }

  .ghead {
    display: flex;
    align-items: center;
    gap: 10px;
    margin: 0;
    font-size: 18px;
    font-weight: 800;
  }

  .gtag {
    padding: 0 10px;
    border: 3px solid var(--line);
    border-radius: 10px;
    background: var(--lilac);
    color: #111;
    transform: rotate(-1.5deg);
  }

  .g-docker .gtag {
    background: var(--sky);
  }

  .g-pterodactyl .gtag {
    background: var(--mint);
  }

  .g-pm2 .gtag {
    background: var(--lemon);
  }

  .ghead small {
    color: var(--sub);
    font-size: var(--fs-s);
    font-weight: 600;
  }

  .ugrid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 270px), 1fr));
    gap: 14px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .banner {
    margin: 0;
    padding: 8px 12px;
    border: 3px solid #111;
    border-radius: 14px;
    background: var(--tomato);
    color: #111;
    font-size: var(--fs-s);
    font-weight: 700;
    overflow-wrap: anywhere;
  }

  .skel {
    height: 150px;
    border: 3px solid var(--line);
    border-radius: 16px;
    background: linear-gradient(90deg, rgb(128 128 128 / 0.12) 25%, rgb(128 128 128 / 0.28) 50%, rgb(128 128 128 / 0.12) 75%);
    background-size: 300% 100%;
    animation: shim 1.1s linear infinite;
  }

  @keyframes shim {
    to {
      background-position: -300% 0;
    }
  }

  .empty {
    padding: 18px;
    border: 3px dashed var(--line);
    border-radius: 16px;
    text-align: center;
    font-weight: 700;
  }

  .empty p {
    margin-top: 4px;
    font-weight: 500;
    color: var(--sub);
  }
</style>
