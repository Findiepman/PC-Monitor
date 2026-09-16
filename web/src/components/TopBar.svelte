<script lang="ts">
  import { live } from '../lib/live.svelte';
  import { duration } from '../lib/format';
  import Led from './Led.svelte';

  let {
    hostname,
    onpalette,
    onsignout,
  }: { hostname: string; onpalette: () => void; onsignout: () => void } = $props();

  let host = $derived(live.history.at(-1));
  const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
</script>

<header>
  <div class="id">
    <span class="host mono">{hostname || '…'}</span>
    {#if host}
      <span class="stat mono">up {duration(host.uptime)}</span>
      {#if host.load.length}
        <span class="stat mono load" title="Load average over 1, 5 and 15 minutes">
          load {host.load.map((l) => l.toFixed(2)).join(' ')}
        </span>
      {/if}
    {/if}
  </div>

  <div class="tools">
    <span class="conn" role="status">
      <Led tone={live.status === 'live' ? 'ok' : 'warn'} pulse={live.status !== 'live'} />
      <span class="conn-text">{live.status === 'live' ? 'Live' : live.status === 'connecting' ? 'Connecting' : 'Reconnecting'}</span>
    </span>
    <button class="btn" onclick={onpalette}>
      Commands <kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd>
    </button>
    <button class="btn quiet" onclick={onsignout}>Sign out</button>
  </div>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    min-height: 48px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--rule);
    background: var(--plate);
  }

  .id {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    column-gap: 16px;
    min-width: 0;
  }

  .host {
    font-size: var(--fs-m);
    font-weight: 600;
    font-stretch: 100%;
  }

  .stat {
    font-size: var(--fs-xs);
    color: var(--dim);
    white-space: nowrap;
  }

  .tools {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .conn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    margin-right: 8px;
    font-size: var(--fs-s);
    color: var(--dim);
  }

  .quiet {
    border-color: transparent;
    background: transparent;
    color: var(--dim);
  }

  @media (max-width: 640px) {
    .load,
    .tools kbd {
      display: none;
    }

    .conn-text {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip: rect(0, 0, 0, 0);
      white-space: nowrap;
    }

    header {
      padding-right: 8px;
    }

    .tools {
      gap: 4px;
    }

    .conn {
      margin-right: 0;
    }
  }
</style>
