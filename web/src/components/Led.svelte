<script lang="ts">
  import type { Tone } from '../lib/status';

  let { tone, pulse = false }: { tone: Tone; pulse?: boolean } = $props();
</script>

<span class="led {tone}" class:pulse aria-hidden="true"></span>

<style>
  .led {
    display: inline-block;
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--idle);
  }

  .ok {
    background: var(--ok);
    box-shadow: 0 0 6px color-mix(in srgb, var(--ok) 55%, transparent);
  }

  .warn {
    background: var(--warn);
    box-shadow: 0 0 6px color-mix(in srgb, var(--warn) 55%, transparent);
  }

  .fail {
    background: var(--fail);
    box-shadow: 0 0 6px color-mix(in srgb, var(--fail) 60%, transparent);
  }

  .pulse {
    animation: blink 0.9s steps(2, jump-none) infinite;
  }

  @keyframes blink {
    50% {
      opacity: 0.25;
    }
  }
</style>
