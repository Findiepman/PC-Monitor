<script lang="ts">
  let { name, size = 14 }: { name: 'restart' | 'stop' | 'start' | 'kill' | 'copy' | 'check' | 'download' | 'close'; size?: number } =
    $props();
</script>

<svg width={size} height={size} viewBox="0 0 16 16" aria-hidden="true" focusable="false">
  {#if name === 'restart'}
    <path d="M13 8a5 5 0 1 1-1.5-3.55" />
    <path d="M12 1.8v3h-3" />
  {:else if name === 'stop'}
    <rect x="4" y="4" width="8" height="8" rx="1" />
  {:else if name === 'start'}
    <path d="M5 3.5v9l7.5-4.5z" />
  {:else if name === 'kill'}
    <path d="M4 4l8 8M12 4l-8 8" />
  {:else if name === 'copy'}
    <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
    <path d="M10.5 3.5v-.5a1 1 0 0 0-1-1h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h.5" />
  {:else if name === 'check'}
    <path d="M3 8.5l3 3 7-7" />
  {:else if name === 'download'}
    <path d="M8 2.5v8M4.5 7.5L8 11l3.5-3.5M3 13.5h10" />
  {:else if name === 'close'}
    <path d="M4 4l8 8M12 4l-8 8" />
  {/if}
</svg>

<style>
  svg {
    flex: none;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
