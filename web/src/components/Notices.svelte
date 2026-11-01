<script lang="ts">
  import { notices } from '../lib/notices.svelte';
  import Icon from './Icon.svelte';
  import Led from './Led.svelte';
</script>

<div class="notices" role="status" aria-live="polite">
  {#each notices.items as n (n.id)}
    <div class="notice" class:fail={n.tone === 'fail'} class:ok={n.tone === 'ok'}>
      <Led tone={n.tone} />
      <span class="text">{n.text}</span>
      <button onclick={() => notices.dismiss(n.id)} aria-label="Dismiss">
        <Icon name="close" size={12} />
      </button>
    </div>
  {/each}
</div>

<style>
  .notices {
    position: fixed;
    left: 16px;
    bottom: calc(16px + env(safe-area-inset-bottom));
    z-index: 40;
    display: grid;
    gap: 14px;
    width: min(420px, calc(100vw - 38px));
    pointer-events: none;
  }

  .notice {
    --fg: #111;
    --line: #111;
    --focus: #1f3fd6;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    padding: 8px 8px 8px 14px;
    border: 3px solid var(--edge);
    border-radius: 16px;
    background: var(--lemon);
    color: #111;
    box-shadow: 4px 4px 0 var(--edge);
    font-size: var(--fs-s);
    font-weight: 700;
    pointer-events: auto;
  }

  .ok {
    background: var(--mint);
  }

  .fail {
    background: var(--tomato);
  }

  .text {
    overflow-wrap: anywhere;
  }

  button {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 2.5px solid #111;
    border-radius: 10px;
    background: #fff;
    color: #111;
    touch-action: manipulation;
  }

  button:active {
    background: #111;
    color: #fff;
  }

  @media (pointer: coarse) {
    button {
      width: 44px;
      height: 44px;
    }
  }
</style>
