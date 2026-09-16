<script lang="ts">
  import { notices } from '../lib/notices.svelte';
  import Icon from './Icon.svelte';
  import Led from './Led.svelte';
</script>

<div class="notices" role="status" aria-live="polite">
  {#each notices.items as n (n.id)}
    <div class="notice" class:fail={n.tone === 'fail'}>
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
    gap: 8px;
    width: min(420px, calc(100vw - 32px));
    pointer-events: none;
  }

  .notice {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    padding: 10px 8px 10px 12px;
    border: 1px solid var(--rule);
    border-radius: 6px;
    background: var(--plate-hi);
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.35);
    font-size: var(--fs-s);
    pointer-events: auto;
  }

  .fail {
    border-color: color-mix(in srgb, var(--fail) 45%, var(--rule));
  }

  .text {
    overflow-wrap: anywhere;
  }

  button {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    padding: 0;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--dim);
  }

  button:hover {
    color: var(--ink);
  }
</style>
