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

<section class="pane" class:sheet class:t-logs={!sheet} aria-label={unit ? `Logs for ${unit.name}` : 'Logs'}>
  <div class="shell tile tile-lilac">
    <header class="th">
      {#if sheet}
        <button class="btn back" onclick={onclose}>Back</button>
      {/if}
      <h2>Logs</h2>
      {#if unit}
        <span class="uname"><Led tone={tone(unit)} /><span class="un">{unit.name}</span></span>
        <span class="kind pill plain">{stateLabel(unit)}, {unit.kind}</span>
      {/if}
    </header>

    {#if !unit}
      <div class="blank">
        <b>No service selected</b>
        <p>Pick a service to follow its logs.</p>
      </div>
    {:else}
      <div class="controls">
        <label class="filter">
          <span class="sr-only">Filter log lines</span>
          <input
            bind:this={filterInput}
            bind:value={filter}
            type="search"
            placeholder="Filter lines"
            spellcheck="false"
            onkeydown={(e) => e.key === 'Escape' && ((filter = ''), (e.currentTarget as HTMLInputElement).blur())}
          />
        </label>
        <button class="btn" aria-pressed={showTime} onclick={() => (showTime = !showTime)}>Time</button>
        <button class="btn" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>Wrap</button>
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
  </div>
</section>

<style>
  .pane {
    min-width: 0;
  }

  .shell {
    min-height: 0;
  }

  /* Phone: the log takes over the screen as one big tile. */
  .sheet {
    position: fixed;
    inset: 0;
    z-index: 20;
    height: 100dvh;
    padding: 12px 17px calc(17px + env(safe-area-inset-bottom)) 12px;
    background-color: var(--paper);
    background-image: radial-gradient(var(--dots) 1.4px, transparent 1.5px);
    background-size: 26px 26px;
  }

  .sheet .shell {
    height: 100%;
    overflow: hidden;
  }

  .blank {
    padding: 28px 12px;
    border: 3px dashed var(--line);
    border-radius: 16px;
    background: var(--card);
    color: #111;
    text-align: center;
    font-weight: 700;
  }

  .blank p {
    margin-top: 4px;
    font-weight: 500;
    color: var(--sub);
  }

  .uname {
    display: inline-flex;
    flex: 0 1 auto;
    align-items: center;
    gap: 8px;
    min-width: 0;
    font-weight: 800;
  }

  .un {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .th h2 {
    margin-right: 0;
    flex: none;
  }

  .th {
    flex-wrap: wrap;
  }

  .kind {
    flex: none;
    min-height: 26px;
    margin-left: auto;
    font-size: 12px;
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .filter {
    flex: 1 1 200px;
  }

  .filter input {
    width: 100%;
    min-height: 44px;
    font-size: var(--fs-s);
  }

  .icon {
    width: 44px;
    padding: 0;
  }

  .scroller {
    height: clamp(260px, 46vh, 420px);
    overflow: auto;
    overscroll-behavior: contain;
    padding: 8px 0;
    border: 3px solid #111;
    border-radius: 14px;
    background: var(--term);
    color: var(--termfg);
    scrollbar-color: #666 transparent;
  }

  :global([data-theme='dark']) .scroller {
    border-color: var(--edge);
  }

  .sheet .scroller {
    flex: 1 1 0;
    height: auto;
    min-height: 0;
  }

  .scroller:focus-visible {
    outline-offset: 2px;
  }

  .note {
    margin: 4px 16px 8px;
    font-size: var(--fs-xs);
    color: var(--termdim);
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    font-size: 12.5px;
    line-height: 1.6;
  }

  .line {
    position: relative;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) 24px;
    column-gap: 12px;
    padding: 0 8px 0 12px;
    border-left: 6px solid transparent;
  }

  .line:hover,
  .line.picked {
    background: rgb(255 255 255 / 0.08);
  }

  .line.err {
    border-left-color: var(--tomato);
    background: rgb(255 91 63 / 0.18);
  }

  time {
    color: var(--termdim);
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
    width: 24px;
    height: 20px;
    padding: 0;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--termfg);
    opacity: 0;
  }

  .copy:hover {
    color: var(--lemon);
  }

  .copy:focus-visible {
    outline-color: #8fb0ff;
  }

  .line:hover .copy,
  .line.picked .copy,
  .copy:focus-visible {
    opacity: 1;
  }

  @media (hover: none) {
    .copy {
      width: 36px;
      height: 26px;
      opacity: 0.55;
    }

    .line.picked .copy {
      opacity: 1;
    }
  }

  footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 12px;
    min-height: 40px;
    font-size: var(--fs-s);
    font-weight: 700;
  }

  .bad {
    color: #7a1200;
  }

  :global(body:has(.pane.sheet)) {
    overflow: hidden;
  }

  @media (max-width: 560px) {
    .controls .btn:not(.icon) {
      flex: 1 1 auto;
    }

    .kind {
      margin-left: 0;
    }
  }
</style>
