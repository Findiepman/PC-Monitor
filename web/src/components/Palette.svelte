<script lang="ts">
  import { tick } from 'svelte';
  import type { Action, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { capitalize } from '../lib/format';
  import { tone } from '../lib/status';
  import Led from './Led.svelte';

  let {
    open = $bindable(false),
    onlogs,
    onaction,
    onsignout,
  }: {
    open: boolean;
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

  let commands = $derived.by<Command[]>(() => {
    const out: Command[] = [];
    for (const u of live.units) {
      out.push({ key: `${u.id}:logs`, label: `Show logs for ${u.name}`, unit: u, destructive: false, run: () => onlogs(u) });
      for (const a of u.actions) {
        out.push({
          key: `${u.id}:${a}`,
          label: `${capitalize(a)} ${u.name}`,
          unit: u,
          destructive: a !== 'start',
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
      return;
    }
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
            {confirming === c.key ? `Press Enter again to ${c.label.toLowerCase()}` : c.label}
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
    width: min(560px, calc(100vw - 32px));
    max-height: min(520px, calc(100dvh - 96px));
    margin: 12vh auto auto;
    padding: 0;
    border: 1px solid var(--rule);
    border-radius: 8px;
    background: var(--plate);
    color: var(--ink);
    box-shadow: 0 24px 60px rgb(0 0 0 / 0.45);
  }

  dialog::backdrop {
    background: rgb(10 12 15 / 0.55);
  }

  .box {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    max-height: inherit;
  }

  input {
    height: 48px;
    padding: 0 16px;
    border: 0;
    border-bottom: 1px solid var(--rule);
    background: transparent;
    font-size: var(--fs-m);
  }

  input:focus-visible {
    outline: none;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 6px;
    overflow: auto;
  }

  li {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr) auto;
    align-items: center;
    gap: 12px;
    padding: 8px 10px;
    border-radius: 5px;
    font-size: var(--fs-s);
    cursor: pointer;
  }

  li.active {
    background: var(--plate-hi);
    box-shadow: inset 2px 0 0 var(--trace);
  }

  li.danger {
    box-shadow: inset 2px 0 0 var(--fail);
    color: var(--fail);
  }

  .kind {
    font-size: var(--fs-xs);
    color: var(--dim);
  }

  .none {
    display: block;
    color: var(--dim);
    cursor: default;
  }
</style>
