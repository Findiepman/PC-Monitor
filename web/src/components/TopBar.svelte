<script lang="ts">
  import { onMount } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { currentTheme, onThemeChange, setTheme } from '../lib/theme';
  import Led from './Led.svelte';

  let {
    hostname,
    view = 'home',
    showBots = false,
    botsLabel = 'Discord bots',
    onpalette,
    onsignout,
  }: {
    hostname: string;
    view?: 'home' | 'bots';
    showBots?: boolean;
    botsLabel?: string;
    onpalette: () => void;
    onsignout: () => void;
  } = $props();

  const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
  let dark = $state(currentTheme() === 'dark');

  onMount(() => onThemeChange(() => (dark = currentTheme() === 'dark')));
</script>

<header class="top">
  <span class="logo"><i class="dot" aria-hidden="true"></i><b>dashd</b></span>
  <span class="hostchip mono" title={hostname}>{hostname || '…'}</span>
  <span class="conn pill plain" role="status">
    <Led tone={live.status === 'live' ? 'ok' : 'warn'} pulse={live.status !== 'live'} />
    <span>{live.status === 'live' ? 'Live' : live.status === 'connecting' ? 'Connecting' : 'Reconnecting'}</span>
  </span>

  {#if showBots || view === 'bots'}
    <nav class="tabs" aria-label="Pages">
      <a href="#/" aria-current={view === 'home' ? 'page' : undefined}>Dashboard</a>
      <a href="#/bots" aria-current={view === 'bots' ? 'page' : undefined} title={botsLabel}><span>{botsLabel}</span></a>
    </nav>
  {/if}

  <span class="grow"></span>

  <button class="btn palette" onclick={onpalette} aria-keyshortcuts="Control+K Meta+K">
    Commands <kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd>
  </button>
  <button
    class="switch"
    role="switch"
    aria-checked={dark}
    aria-label="Dark mode"
    title="Dark mode"
    onclick={() => setTheme(dark ? 'light' : 'dark')}
  >
    <span class="trk"><span class="knob"></span></span>
  </button>
  <button class="btn" onclick={onsignout}>Sign out</button>
</header>

<style>
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 12px;
    margin-bottom: 18px;
  }

  .grow {
    flex: 1;
  }

  .logo {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 4px 14px 4px 10px;
    background: var(--lemon);
    color: #111;
    border: 3px solid var(--edge);
    border-radius: 16px;
    box-shadow: 4px 4px 0 var(--edge);
    transform: rotate(-1.5deg);
    transition: transform 0.25s cubic-bezier(0.3, 1.9, 0.5, 1);
  }

  .logo:hover {
    transform: rotate(2deg) scale(1.04);
  }

  .logo b {
    font-size: 28px;
    font-weight: 800;
    letter-spacing: -0.04em;
    font-stretch: 75%;
    line-height: 1.1;
  }

  .dot {
    flex: none;
    width: 22px;
    height: 22px;
    border: 3px solid #111;
    border-radius: 50%;
    background: var(--tomato);
    animation: swap 2.4s infinite;
  }

  @keyframes swap {
    50% {
      background: var(--mint);
    }
  }

  .hostchip {
    min-width: 0;
    max-width: 100%;
    padding: 4px 10px;
    border: 3px solid var(--edge);
    border-radius: 12px;
    background: var(--card);
    color: var(--fg);
    font-size: 15px;
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conn {
    --line: var(--edge);
    gap: 8px;
    min-height: 34px;
    padding-left: 8px;
    background: var(--card);
    color: var(--fg);
  }

  .tabs {
    display: inline-flex;
    gap: 6px;
    padding: 3px;
    border: 3px solid var(--edge);
    border-radius: 14px;
    background: var(--card);
  }

  .tabs a {
    display: inline-flex;
    align-items: center;
    min-height: 34px;
    padding: 0 12px;
    border: 2.5px solid transparent;
    border-radius: 10px;
    color: var(--fg);
    font-size: var(--fs-s);
    font-weight: 800;
    min-width: 0;
    text-decoration: none;
    white-space: nowrap;
  }

  .tabs a span {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tabs a:hover {
    border-color: var(--edge);
  }

  .tabs a[aria-current='page'] {
    border-color: #111;
    background: var(--mint);
    color: #111;
    box-shadow: 2px 2px 0 #111;
  }

  .palette {
    min-width: 200px;
    justify-content: flex-start;
  }

  .palette kbd {
    margin-left: auto;
  }

  .switch:focus-visible {
    border-radius: 999px;
  }

  @media (max-width: 560px) {
    .grow {
      display: none;
    }

    .tabs {
      order: 10;
      flex: 1 1 100%;
      min-width: 0;
    }

    .tabs a {
      flex: 1;
      justify-content: center;
    }

    /* Live, the theme switch, Commands and Sign out don't fit one phone row,
       so Commands gets a full-width row of its own, above the page tabs. */
    .palette {
      order: 9;
      flex: 1 1 100%;
    }

    .palette kbd {
      display: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }

    .logo {
      transform: none;
      transition: none;
    }
  }
</style>
