<script lang="ts">
  import { tick } from 'svelte';
  import type { LogLine, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { copyText, downloadText } from '../lib/clipboard';
  import { clock, stamp } from '../lib/format';
  import { stateLabel, tone } from '../lib/status';
  import Icon from './Icon.svelte';
  import Led from './Led.svelte';

  const KEEP = 5000; // lines held in memory per stream
  const RENDER = 1500; // lines in the DOM at once

  let {
    unit,
    sheet = false,
    onclose,
    filterInput = $bindable(),
  }: {
    unit: Unit | undefined;
    sheet?: boolean;
    onclose?: () => void;
    filterInput?: HTMLInputElement;
  } = $props();

  type Row = LogLine & { seq: number };
  let seq = 0;
  let lines = $state.raw<Row[]>([]);
  let ended = $state<{ error?: string } | null>(null);
  let filter = $state('');
  let follow = $state(true);
  let showTime = $state(true);
  let wrap = $state(true);
  let picked = $state(-1);
  let copied = $state<string | null>(null);
  let restartKey = $state(0);
  let scroller: HTMLDivElement | undefined = $state();

  let unitId = $derived(unit?.id);

  $effect(() => {
    const id = unitId;
    restartKey; // resubscribe when the user reconnects a finished stream
    lines = [];
    ended = null;
    picked = -1;
    follow = true;
    if (!id) return;

    let stop: (() => void) | undefined;
    // Debounce so holding j/k through the list doesn't open a stream per row.
    const t = setTimeout(() => {
      stop = live.subscribeLogs(
        id,
        500,
        (batch) => {
          ended = null;
          const rows = batch.map((l) => ({ ...l, seq: seq++ }));
          const merged = [...lines, ...rows];
          lines = merged.length > KEEP ? merged.slice(-KEEP) : merged;
        },
        (error) => (ended = { error }),
      );
    }, 180);
    return () => {
      clearTimeout(t);
      stop?.();
    };
  });

  let query = $derived(filter.trim().toLowerCase());
  let visible = $derived(query ? lines.filter((l) => l.text.toLowerCase().includes(query)) : lines);
  let shown = $derived(visible.length > RENDER ? visible.slice(-RENDER) : visible);
  let offset = $derived(visible.length - shown.length);

  $effect(() => {
    shown;
    if (follow && scroller) {
      tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
    }
  });

  // Only a person scrolling up should pause following. Layout changes and
  // our own scrollTop writes also fire scroll events.
  let userScrollAt = 0;
  const markUser = () => (userScrollAt = performance.now());

  function onScroll() {
    if (!scroller) return;
    const atBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 32;
    if (atBottom) {
      follow = true;
    } else if (performance.now() - userScrollAt < 800) {
      follow = false;
    } else if (follow) {
      scroller.scrollTop = scroller.scrollHeight;
    }
  }

  function jumpToLatest() {
    follow = true;
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }

  const asText = (ls: LogLine[]) => ls.map((l) => (showTime && l.t ? `${stamp(l.t)}  ${l.text}` : l.text)).join('\n');

  async function copy(key: string, text: string) {
    if (await copyText(text)) {
      copied = key;
      setTimeout(() => copied === key && (copied = null), 1500);
    }
  }

  function download() {
    const name = (unit?.name ?? 'logs').replace(/[^\w.-]+/g, '_');
    const d = new Date();
    const date = `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`;
    downloadText(`${name}-${date}.log`, asText(visible) + '\n');
  }

  function pickLine(n: number) {
    // Don't steal a click that ended a text selection.
    if (window.getSelection()?.toString()) return;
    picked = picked === n ? -1 : n;
  }
</script>

<section class="pane" class:sheet aria-label={unit ? `Logs for ${unit.name}` : 'Logs'}>
  {#if !unit}
    <div class="blank">
      <p>Pick a service to follow its logs.</p>
    </div>
  {:else}
    <header>
      <div class="title">
        {#if sheet}
          <button class="btn back" onclick={onclose}>Back</button>
        {/if}
        <Led tone={tone(unit)} />
        <h2>{unit.name}</h2>
        <span class="kind">{stateLabel(unit)}, {unit.kind}</span>
      </div>

      <div class="controls">
        <label class="filter">
          <span class="sr-only">Filter log lines</span>
          <input
            bind:this={filterInput}
            bind:value={filter}
            type="search"
            placeholder="Filter"
            spellcheck="false"
            onkeydown={(e) => e.key === 'Escape' && ((filter = ''), (e.currentTarget as HTMLInputElement).blur())}
          />
        </label>
        <button class="btn" aria-pressed={showTime} onclick={() => (showTime = !showTime)}>Time</button>
        <button class="btn" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>Wrap</button>
        <span class="spacer"></span>
        <button class="btn" disabled={!visible.length} onclick={() => copy('all', asText(visible))}>
          <Icon name={copied === 'all' ? 'check' : 'copy'} />
          {#if copied === 'all'}
            Copied
          {:else}
            Copy {query ? `${visible.length.toLocaleString()} matching` : `${visible.length.toLocaleString()} lines`}
          {/if}
        </button>
        <button class="btn icon" disabled={!visible.length} onclick={download} aria-label="Download as .log file" title="Download as .log file">
          <Icon name="download" />
        </button>
      </div>
    </header>

    <!-- A focusable scroll region lets keyboard users scroll the log. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      class="scroller"
      class:nowrap={!wrap}
      role="log"
      tabindex="0"
      aria-label="Log output"
      bind:this={scroller}
      onscroll={onScroll}
      onwheel={markUser}
      ontouchmove={markUser}
      onpointerdown={markUser}
      onkeydown={markUser}
    >
      {#if offset > 0}
        <p class="note">Showing the last {RENDER.toLocaleString()} of {visible.length.toLocaleString()} lines. Copy and download include all of them.</p>
      {/if}
      {#if lines.length === 0 && !ended}
        <p class="note">Waiting for output…</p>
      {:else if query && visible.length === 0}
        <p class="note">No lines match “{filter}”.</p>
      {/if}
      <ol class="mono">
        {#each shown as l (l.seq)}
          <li class="line" class:err={l.err} class:picked={picked === l.seq}>
            {#if showTime}<time>{clock(l.t)}</time>{/if}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
            <span class="text" onclick={() => pickLine(l.seq)}>{l.text}</span>
            <button
              class="copy"
              aria-label="Copy this line"
              title="Copy line"
              onclick={() => copy(`l${l.seq}`, asText([l]))}
            >
              <Icon name={copied === `l${l.seq}` ? 'check' : 'copy'} size={12} />
            </button>
          </li>
        {/each}
      </ol>
    </div>

    <footer>
      {#if ended}
        <span class:bad={!!ended.error}>{ended.error ?? 'The log stream ended.'}</span>
        <button class="btn" onclick={() => restartKey++}>Reconnect</button>
      {:else if !follow}
        <span>Paused while you scroll.</span>
        <button class="btn" onclick={jumpToLatest}>Jump to latest</button>
      {:else}
        <span>Following new output</span>
      {/if}
    </footer>
  {/if}
</section>

<style>
  .pane {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    min-width: 0;
    min-height: 0;
    height: 100%;
    background: var(--bay);
  }

  .sheet {
    position: fixed;
    inset: 0;
    z-index: 20;
    height: 100dvh;
    padding-bottom: env(safe-area-inset-bottom);
  }

  .blank {
    display: grid;
    place-items: center;
    padding: 32px 16px;
    color: var(--dim);
    font-size: var(--fs-s);
  }

  header {
    display: grid;
    gap: 10px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--rule);
    background: var(--plate);
  }

  .title {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  h2 {
    margin: 0;
    font-size: var(--fs-m);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .kind {
    font-size: var(--fs-s);
    color: var(--dim);
    white-space: nowrap;
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }

  .filter input {
    width: 180px;
    height: 30px;
    padding: 0 10px;
    border: 1px solid var(--rule);
    border-radius: 5px;
    background: var(--bay);
    font-size: var(--fs-s);
  }

  .filter input:focus-visible {
    outline: none;
    border-color: var(--trace);
  }

  .spacer {
    flex: 1;
  }

  .icon {
    width: 30px;
    padding: 0;
    justify-content: center;
  }

  .scroller {
    overflow: auto;
    overscroll-behavior: contain;
    padding: 6px 0;
  }

  .note {
    margin: 4px 16px 8px;
    font-size: var(--fs-xs);
    color: var(--dim);
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    font-size: 12px;
    line-height: 1.6;
  }

  .line {
    position: relative;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) 24px;
    column-gap: 12px;
    padding: 0 8px 0 16px;
    border-left: 2px solid transparent;
  }

  .line:hover,
  .line.picked {
    background: var(--plate);
  }

  .line.err {
    border-left-color: var(--fail);
    background: var(--fail-soft);
  }

  time {
    color: var(--dim);
    user-select: none;
    -webkit-user-select: none;
  }

  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .nowrap .text {
    white-space: pre;
    overflow-wrap: normal;
  }

  .nowrap ol {
    width: max-content;
    min-width: 100%;
  }

  .copy {
    align-self: start;
    display: grid;
    place-items: center;
    width: 22px;
    height: 19px;
    margin-top: 0;
    padding: 0;
    border: 0;
    border-radius: 3px;
    background: transparent;
    color: var(--dim);
    opacity: 0;
  }

  .copy:hover {
    color: var(--trace);
  }

  .line:hover .copy,
  .line.picked .copy,
  .copy:focus-visible {
    opacity: 1;
  }

  @media (hover: none) {
    .copy {
      width: 32px;
      height: 24px;
    }
  }

  footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    min-height: 40px;
    padding: 5px 16px;
    border-top: 1px solid var(--rule);
    background: var(--plate);
    font-size: var(--fs-xs);
    color: var(--dim);
  }

  footer .btn {
    height: 26px;
  }

  .bad {
    color: var(--fail);
  }

  :global(body:has(.pane.sheet)) {
    overflow: hidden;
  }

  @media (max-width: 560px) {
    .filter {
      flex: 1 1 100%;
    }

    .filter input {
      width: 100%;
    }

    .spacer {
      display: none;
    }
  }
</style>
