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
  import Bots from './Bots.svelte';

  let { onsignout }: { onsignout: () => void } = $props();

  const SELECTED_KEY = 'dashd:selected';
  // The pm2 provider gets its own page instead of a group in the rack.
  const BOTS = 'pm2';

  type View = 'home' | 'bots';
  const readView = (): View => (location.hash === '#/bots' ? 'bots' : 'home');

  let meta = $state<Meta>({ hostname: '', providers: [] });
  let selectedId = $state<string | undefined>(readSelected());
  let metaState = $state<'loading' | 'ok' | 'failed'>('loading');
  let pfilter = $state('all');
  let sheetOpen = $state(false);
  let paletteOpen = $state(false);
  let pending = $state<Record<string, Action>>({});
  let armed = $state<{ id: string; action: Action } | null>(null);
  let filterInput = $state<HTMLInputElement>();
  let narrow = $state(false);
  let view = $state<View>(readView());

  let botsProvider = $derived(meta.providers.find((p) => p.name === BOTS));
  let rackProviders = $derived(meta.providers.filter((p) => p.name !== BOTS));
  // Units in the order they appear on screen, grouped by provider.
  let ordered = $derived(rackProviders.flatMap((p) => live.units.filter((u) => u.provider === p.name)));
  let selected = $derived(ordered.find((u) => u.id === selectedId));
  // j and k only walk the units the provider filter leaves on screen.
  let onscreen = $derived(pfilter === 'all' ? ordered : ordered.filter((u) => u.provider === pfilter));

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
    const what = view === 'bots' ? 'bots' : 'dashboard';
    document.title = meta.hostname ? `${meta.hostname} ${what}` : 'dashd';
  });

  // Switching pages from the palette or from the tabs/Back (via hashchange).
  function show(v: View) {
    view = v;
    armed = null;
    sheetOpen = false;
    window.scrollTo({ top: 0 });
  }

  function go(v: View) {
    if (v === view) return;
    location.hash = v === 'bots' ? '#/bots' : '#/';
    show(v);
  }

  onMount(() => {
    live.connect();
    api.meta().then(
      (m) => {
        meta = m;
        metaState = 'ok';
      },
      () => (metaState = 'failed'),
    );
    api.audit().then((a) => live.setAudit(a), () => {});

    const mq = matchMedia('(max-width: 560px)');
    narrow = mq.matches;
    const onMq = () => (narrow = mq.matches);
    mq.addEventListener('change', onMq);
    const onHash = () => {
      // go() already switched; only Back/Forward or a typed URL gets here with news.
      const v = readView();
      if (v !== view) show(v);
    };
    window.addEventListener('hashchange', onHash);
    return () => {
      live.close();
      mq.removeEventListener('change', onMq);
      window.removeEventListener('hashchange', onHash);
    };
  });

  function select(u: Unit, openSheet = true) {
    selectedId = u.id;
    armed = null;
    if (narrow && openSheet) sheetOpen = true;
  }

  // From the palette or the l key: select, then bring the logs tile into view.
  function showLogs(u: Unit) {
    select(u);
    if (narrow) return;
    tick().then(() =>
      document.querySelector('.t-logs')?.scrollIntoView({
        block: 'start',
        behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
      }),
    );
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
    if (!u || !onscreen.some((x) => x.id === u.id) || !u.actions.includes(action)) return;
    if (armed?.id === u.id && armed.action === action) {
      run(u, action);
      return;
    }
    armed = { id: u.id, action };
    clearTimeout(armTimer);
    armTimer = setTimeout(() => (armed = null), 4000);
  }

  function move(delta: number) {
    if (!onscreen.length) return;
    const i = onscreen.findIndex((u) => u.id === selectedId);
    const next = onscreen[Math.max(0, Math.min(onscreen.length - 1, i + delta))];
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
    // The unit keys only act on the rack, which the bots page doesn't show.
    if (view !== 'home') return;
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
        if (selected && onscreen.some((x) => x.id === selected.id)) showLogs(selected);
        break;
      case '/':
        e.preventDefault();
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
  <TopBar
    hostname={meta.hostname}
    {view}
    showBots={!!botsProvider}
    botsLabel={botsProvider?.label}
    onpalette={() => (paletteOpen = true)}
    onsignout={signOut}
  />

  {#if view === 'bots'}
    <Bots provider={botsProvider} {metaState} />
  {:else}
    <main class="bento">
      <HostTrace />

      <Rack
        providers={rackProviders}
        botsElsewhere={!!botsProvider}
        {metaState}
        {selectedId}
        {pending}
        {armed}
        bind:filter={pfilter}
        onselect={(u) => select(u)}
        onaction={run}
      />

      <Activity />

      {#if !narrow}
        <LogPane unit={selected} bind:filterInput />
      {:else}
        <section class="tile tile-lilac t-logs" aria-labelledby="h-logs">
          <header class="th"><h2 id="h-logs">Logs</h2></header>
          {#if selected}
            <p class="sub"><b>{selected.name}</b> is selected.</p>
            <button class="btn" onclick={() => (sheetOpen = true)}>Open logs</button>
          {:else}
            <p class="sub">Pick a service to follow its logs.</p>
          {/if}
        </section>
        {#if sheetOpen && selected}
          <LogPane unit={selected} sheet onclose={() => (sheetOpen = false)} bind:filterInput />
        {/if}
      {/if}
    </main>
  {/if}
</div>

<Palette
  bind:open={paletteOpen}
  skip={BOTS}
  {view}
  botsLabel={botsProvider?.label}
  onview={go}
  onlogs={(u) => {
    go('home');
    showLogs(u);
  }}
  onaction={run}
  onsignout={signOut}
/>
<Notices />

<style>
  .console {
    max-width: 1360px;
    margin: 0 auto;
    /* room on the right and bottom for the hard shadows */
    padding: 16px 22px 40px 16px;
  }

  @media (max-width: 560px) {
    .console {
      padding: 12px 17px 32px 12px;
    }
  }
</style>
