<script lang="ts">
  import { onMount, tick } from 'svelte';
  import type { Action, Meta, Unit } from '../lib/types';
  import { api } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { notices } from '../lib/notices.svelte';
  import { pastTense } from '../lib/format';
  import TopBar from './TopBar.svelte';
  import HostTrace from './HostTrace.svelte';
  import Rack from './Rack.svelte';
  import LogPane from './LogPane.svelte';
  import Activity from './Activity.svelte';
  import Palette from './Palette.svelte';
  import Notices from './Notices.svelte';

  let { onsignout }: { onsignout: () => void } = $props();

  const SELECTED_KEY = 'dashd:selected';

  let meta = $state<Meta>({ hostname: '', providers: [] });
  let selectedId = $state<string | undefined>(readSelected());
  let tab = $state<'logs' | 'activity'>('logs');
  let sheetOpen = $state(false);
  let paletteOpen = $state(false);
  let pending = $state<Record<string, Action>>({});
  let armed = $state<{ id: string; action: Action } | null>(null);
  let filterInput = $state<HTMLInputElement>();
  let narrow = $state(false);

  // Units in the order they appear on screen, grouped by provider.
  let ordered = $derived(meta.providers.flatMap((p) => live.units.filter((u) => u.provider === p.name)));
  let selected = $derived(ordered.find((u) => u.id === selectedId));

  function readSelected() {
    try {
      return localStorage.getItem(SELECTED_KEY) ?? undefined;
    } catch {
      return undefined;
    }
  }

  $effect(() => {
    if (!selectedId) return;
    try {
      localStorage.setItem(SELECTED_KEY, selectedId);
    } catch {
      // Storage unavailable; selection just won't persist.
    }
  });

  // Fall back to the first unit when nothing (or a vanished unit) is selected.
  $effect(() => {
    if (ordered.length && !selected && !narrow) selectedId = ordered[0].id;
  });

  $effect(() => {
    document.title = meta.hostname ? `${meta.hostname} dashboard` : 'dashd';
  });

  onMount(() => {
    live.connect();
    api.meta().then((m) => (meta = m), () => {});
    api.audit().then((a) => live.setAudit(a), () => {});

    const mq = matchMedia('(max-width: 899px)');
    narrow = mq.matches;
    const onMq = () => (narrow = mq.matches);
    mq.addEventListener('change', onMq);
    return () => {
      live.close();
      mq.removeEventListener('change', onMq);
    };
  });

  function select(u: Unit, openSheet = true) {
    selectedId = u.id;
    tab = 'logs';
    armed = null;
    if (narrow && openSheet) sheetOpen = true;
  }

  async function run(u: Unit, action: Action) {
    armed = null;
    if (pending[u.id]) return;
    pending[u.id] = action;
    try {
      await api.action(u.id, action);
      notices.push('ok', `${pastTense[action]} ${u.name}`);
    } catch (err) {
      notices.push('fail', `Couldn't ${action} ${u.name}. ${(err as Error).message}`);
    } finally {
      delete pending[u.id];
    }
  }

  async function signOut() {
    try {
      await api.logout();
    } catch {
      // Session may already be gone; either way we're signed out.
    }
    live.close();
    onsignout();
  }

  let armTimer: ReturnType<typeof setTimeout> | undefined;

  function arm(action: Action) {
    const u = selected;
    if (!u || !u.actions.includes(action)) return;
    if (armed?.id === u.id && armed.action === action) {
      run(u, action);
      return;
    }
    armed = { id: u.id, action };
    clearTimeout(armTimer);
    armTimer = setTimeout(() => (armed = null), 4000);
  }

  function move(delta: number) {
    if (!ordered.length) return;
    const i = ordered.findIndex((u) => u.id === selectedId);
    const next = ordered[Math.max(0, Math.min(ordered.length - 1, i + delta))];
    select(next, false);
    tick().then(() =>
      document.querySelector('.unit[aria-current="true"]')?.scrollIntoView({ block: 'nearest' }),
    );
  }

  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      paletteOpen = !paletteOpen;
      return;
    }
    if (paletteOpen || e.metaKey || e.ctrlKey || e.altKey) return;
    const target = e.target as HTMLElement;
    if (target.closest('input, textarea, [contenteditable]')) return;

    switch (e.key) {
      case 'j':
        move(1);
        break;
      case 'k':
        move(-1);
        break;
      case 'r':
        arm('restart');
        break;
      case 's':
        arm('stop');
        break;
      case 'l':
        if (selected) select(selected);
        break;
      case '/':
        e.preventDefault();
        tab = 'logs';
        if (narrow && selected) sheetOpen = true;
        tick().then(() => filterInput?.focus());
        break;
      case 'Escape':
        armed = null;
        sheetOpen = false;
        break;
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="console">
  <TopBar hostname={meta.hostname} onpalette={() => (paletteOpen = true)} onsignout={signOut} />
  <HostTrace />

  <main class="split">
    <div class="left">
      <Rack
        providers={meta.providers}
        {selectedId}
        {pending}
        {armed}
        onselect={(u) => select(u)}
        onaction={run}
      />
    </div>

    {#if !narrow}
      <div class="right">
        <div class="tabs" role="tablist" aria-label="Detail view">
          <button role="tab" aria-selected={tab === 'logs'} onclick={() => (tab = 'logs')}>Logs</button>
          <button role="tab" aria-selected={tab === 'activity'} onclick={() => (tab = 'activity')}>
            Activity
            {#if live.audit.length}<span class="mono count">{live.audit.length}</span>{/if}
          </button>
        </div>
        {#if tab === 'logs'}
          <LogPane unit={selected} bind:filterInput />
        {:else}
          <Activity />
        {/if}
      </div>
    {:else}
      <section class="mobile-activity">
        <h2>Activity</h2>
        <Activity />
      </section>
      {#if sheetOpen && selected}
        <LogPane unit={selected} sheet onclose={() => (sheetOpen = false)} bind:filterInput />
      {/if}
    {/if}
  </main>
</div>

<Palette
  bind:open={paletteOpen}
  onlogs={(u) => select(u)}
  onaction={run}
  onsignout={signOut}
/>
<Notices />

<style>
  .console {
    display: grid;
    grid-template-rows: auto auto minmax(0, 1fr);
    height: 100dvh;
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
    min-height: 0;
  }

  .left {
    overflow: auto;
    min-height: 0;
    border-right: 1px solid var(--rule);
  }

  .right {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    min-height: 0;
  }

  .tabs {
    display: flex;
    gap: 4px;
    padding: 0 12px;
    border-bottom: 1px solid var(--rule);
    background: var(--plate);
  }

  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 40px;
    padding: 0 8px;
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--dim);
    font-size: var(--fs-s);
    font-weight: 500;
  }

  .tabs button[aria-selected='true'] {
    color: var(--ink);
    border-bottom-color: var(--trace);
  }

  .tabs button:focus-visible {
    outline-offset: -2px;
  }

  .count {
    font-size: 11px;
    color: var(--dim);
  }

  .mobile-activity h2 {
    margin: 0;
    padding: 20px 16px 10px;
    font-size: var(--fs-l);
    font-weight: 600;
    border-bottom: 1px solid var(--rule);
  }

  @media (max-width: 899px) {
    .console {
      display: block;
      height: auto;
    }

    .split {
      display: block;
    }

    .left {
      overflow: visible;
      border-right: 0;
    }

    .mobile-activity {
      padding-bottom: 80px;
    }
  }
</style>
