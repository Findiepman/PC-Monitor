<script lang="ts">
  import type { Tone } from '../lib/status';

  let { tone, pulse = false }: { tone: Tone; pulse?: boolean } = $props();
</script>

<span class="led {tone}" class:pulse aria-hidden="true"></span>

<style>
  /* Shape and fill carry the state, not colour alone:
     round = ok, hollow = idle, diamond = problem, striped = busy. */
  .led {
    display: inline-block;
    flex: none;
    width: 14px;
    height: 14px;
    border: 2.5px solid var(--line);
    border-radius: 50%;
    background: var(--lemon);
  }

  .ok {
    background: var(--mint);
  }

  .warn {
    background: var(--lemon);
  }

  .fail {
    background: var(--tomato);
    border-radius: 3px;
    transform: rotate(45deg);
  }

  .idle {
    background: transparent;
  }

  .pulse {
    animation: blink 0.9s steps(2, jump-none) infinite;
  }

  @keyframes blink {
    50% {
      opacity: 0.3;
    }
  }
</style>
