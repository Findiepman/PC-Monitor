<script lang="ts">
  import { tick } from 'svelte';
  import type { Action, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { capitalize } from '../lib/format';
  import { tone } from '../lib/status';
  import Led from './Led.svelte';

  let {
    open = $bindable(false),
    skip,
    view = 'home',
    botsLabel,
    onview,
    onlogs,
    onaction,
    onsignout,
  }: {
    open: boolean;
    /** Provider whose units get no commands (they live on their own page). */
    skip?: string;
    view?: 'home' | 'bots';
    /** Set when the bots page exists. */
    botsLabel?: string;
    onview?: (v: 'home' | 'bots') => void;
    onlogs: (u: Unit) => void;
    onaction: (u: Unit, a: Action) => void;
    onsignout: () => void;
  } = $props();

  type Command = { key: string; label: string; unit?: Unit; destructive: boolean; run: () => void };

  let dialog: HTMLDialogElement;
  let input: HTMLInputElement;
  let query = $state('');
  let active = $state(0);
  let confirming = $state<string | null>(null);
  let armedAt = 0;
  // Touch devices say "Tap", keyboards/mice say "Press".
  const coarse = typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches;

  let commands = $derived.by<Command[]>(() => {
    const out: Command[] = [];
    if (onview && botsLabel && view !== 'bots') {
      out.push({ key: 'view:bots', label: `Open ${botsLabel}`, destructive: false, run: () => onview('bots') });
    }
    if (onview && view !== 'home') {
      out.push({ key: 'view:home', label: 'Open dashboard', destructive: false, run: () => onview('home') });
    }
    for (const u of live.units) {
      if (u.provider === skip) continue;
      out.push({ key: `${u.id}:logs`, label: `Show logs for ${u.name}`, unit: u, destructive: false, run: () => onlogs(u) });
      for (const a of u.actions) {
        out.push({
          key: `${u.id}:${a}`,
          label: `${capitalize(a)} ${u.name}`,
          unit: u,
          // Every unit action, start included, needs a second press so a stray tap can't run it.
          destructive: true,
          run: () => onaction(u, a),
        });
      }
    }
    out.push({ key: 'signout', label: 'Sign out', destructive: false, run: onsignout });
    return out;
  });

  let results = $derived.by(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!words.length) return commands;
    return commands.filter((c) => {
      const hay = `${c.label} ${c.unit?.kind ?? ''} ${c.unit?.provider ?? ''}`.toLowerCase();
      return words.every((w) => hay.includes(w));
    });
  });

  $effect(() => {
    if (open && !dialog.open) {
      query = '';
      active = 0;
      confirming = null;
      dialog.showModal();
      tick().then(() => input.focus());
    } else if (!open && dialog.open) {
      dialog.close();
    }
  });

  $effect(() => {
    query;
    active = 0;
    confirming = null;
  });

  function choose(c: Command) {
    if (c.destructive && confirming !== c.key) {
      confirming = c.key;
      armedAt = Date.now();
      return;
    }
    // Ignore a confirm that lands right after arming (double-tap). Fine for keyboard too.
    if (c.destructive && Date.now() - armedAt < 500) return;
    open = false;
    c.run();
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      active = Math.min(active + 1, results.length - 1);
      confirming = null;
      scrollActive();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      active = Math.max(active - 1, 0);
      confirming = null;
      scrollActive();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      // A held-down Enter auto-repeats; only a fresh press may arm or confirm.
      if (e.repeat) return;
      const c = results[active];
      if (c) choose(c);
    }
  }

  function scrollActive() {
    tick().then(() => dialog.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }));
  }
</script>

<dialog bind:this={dialog} onclose={() => (open = false)} onclick={(e) => e.target === dialog && (open = false)}>
  <div class="box">
    <input
      bind:this={input}
      bind:value={query}
      {onkeydown}
      placeholder="Type a service or action"
      spellcheck="false"
      autocomplete="off"
      role="combobox"
      aria-expanded="true"
      aria-controls="palette-list"
      aria-activedescendant={results[active] ? `cmd-${active}` : undefined}
    />
    <ul id="palette-list" role="listbox" aria-label="Commands">
      {#each results as c, i (c.key)}
        <li
          id="cmd-{i}"
          role="option"
          aria-selected={i === active}
          class:active={i === active}
          class:danger={confirming === c.key}
          onclick={() => choose(c)}
          onkeydown={() => {}}
          onmousemove={() => i !== active && ((active = i), (confirming = null))}
        >
          {#if c.unit}<Led tone={tone(c.unit)} />{:else}<span class="nol"></span>{/if}
          <span class="label">
            {confirming === c.key ? `${coarse ? 'Tap' : 'Press Enter'} again to ${c.label.toLowerCase()}` : c.label}
          </span>
          {#if c.unit}<span class="kind">{c.unit.kind}</span>{/if}
        </li>
      {:else}
        <li class="none">Nothing matches “{query}”.</li>
      {/each}
    </ul>
  </div>
</dialog>

<style>
  dialog {
    width: min(580px, calc(100vw - 38px));
    max-height: min(540px, calc(100dvh - 96px));
    margin: 12vh auto auto;
    padding: 0;
    border: 3px solid var(--edge);
    border-radius: var(--r);
    background: var(--card);
    color: var(--fg);
    box-shadow: var(--sh);
    overflow: hidden;
  }

  dialog::backdrop {
    background: rgb(17 17 17 / 0.6);
  }

  .box {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    max-height: inherit;
  }

  input {
    min-height: 56px;
    padding: 0 18px;
    border: 0;
    border-bottom: 3px solid var(--edge);
    border-radius: 0;
    background: var(--lemon);
    color: #111;
    font-size: var(--fs-m);
    font-weight: 700;
    box-shadow: none;
  }

  input::placeholder {
    color: rgb(17 17 17 / 0.7);
  }

  input:focus-visible {
    outline: 4px solid #1f3fd6;
    outline-offset: -4px;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 10px;
    overflow: auto;
    display: grid;
    gap: 6px;
  }

  li {
    display: grid;
    grid-template-columns: 14px minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    min-height: 44px;
    padding: 6px 12px;
    border: 3px solid transparent;
    border-radius: 12px;
    font-size: var(--fs-s);
    font-weight: 700;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  li.active {
    background: var(--lemon);
    color: #111;
    --fg: #111;
    --line: #111;
    border-color: #111;
    box-shadow: 3px 3px 0 #111;
  }

  li.danger {
    background: var(--tomato);
    color: #111;
    --fg: #111;
    --line: #111;
    border-color: #111;
    box-shadow: 3px 3px 0 #111;
  }

  .kind {
    font-size: var(--fs-xs);
    font-weight: 600;
    color: var(--sub);
  }

  li.active .kind,
  li.danger .kind {
    color: rgb(17 17 17 / 0.75);
  }

  .none {
    display: block;
    color: var(--sub);
    cursor: default;
  }
</style>
