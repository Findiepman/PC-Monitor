<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    label,
    onconfirm,
    children,
    duration = 650,
    danger = false,
    disabled = false,
  }: {
    label: string;
    onconfirm: () => void;
    children: Snippet;
    duration?: number;
    danger?: boolean;
    disabled?: boolean;
  } = $props();

  let holding = $state(false);
  // Assistive tech sends a click without pointer or key events, so it can't
  // hold. A first such click arms the button; a second confirms.
  let armed = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let armTimer: ReturnType<typeof setTimeout> | undefined;
  let pressed = false;

  function begin() {
    if (disabled || holding) return;
    pressed = true;
    holding = true;
    timer = setTimeout(() => {
      holding = false;
      onconfirm();
    }, duration);
  }

  function cancel() {
    holding = false;
    clearTimeout(timer);
    // The click that follows pointerup/keyup belongs to this press.
    setTimeout(() => (pressed = false), 0);
  }

  function click(e: MouseEvent) {
    if (pressed) {
      pressed = false;
      return;
    }
    if (e.detail !== 0 || disabled) return;
    if (armed) {
      armed = false;
      clearTimeout(armTimer);
      onconfirm();
    } else {
      armed = true;
      armTimer = setTimeout(() => (armed = false), 4000);
    }
  }
</script>

<button
  type="button"
  class="hold"
  class:holding
  class:danger
  {disabled}
  aria-label={armed ? `Confirm: ${label}` : `${label} (press and hold)`}
  title={`Hold to ${label.toLowerCase()}`}
  style:--hold="{duration}ms"
  onpointerdown={(e) => e.button === 0 && begin()}
  onpointerup={cancel}
  onpointerleave={cancel}
  onpointercancel={cancel}
  oncontextmenu={(e) => e.preventDefault()}
  onkeydown={(e) => {
    if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) {
      e.preventDefault();
      begin();
    }
  }}
  onkeyup={(e) => {
    if (e.key === 'Enter' || e.key === ' ') cancel();
  }}
  onclick={click}
>
  <span class="fill" aria-hidden="true"></span>
  {@render children()}
</button>

<style>
  .hold {
    position: relative;
    display: inline-grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    overflow: hidden;
    border: 1px solid transparent;
    border-radius: 5px;
    background: transparent;
    color: var(--dim);
    touch-action: none;
    user-select: none;
    -webkit-user-select: none;
  }

  .hold:hover:not(:disabled) {
    color: var(--ink);
    border-color: var(--rule);
  }

  .hold:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .fill {
    position: absolute;
    inset: 0;
    background: var(--trace-soft);
    border-bottom: 2px solid var(--trace);
    transform: scaleX(0);
    transform-origin: left;
  }

  .danger .fill {
    background: var(--fail-soft);
    border-bottom-color: var(--fail);
  }

  .holding {
    color: var(--ink);
    border-color: var(--rule);
  }

  .holding .fill {
    transform: scaleX(1);
    transition: transform var(--hold) linear;
  }

  .hold :global(svg) {
    position: relative;
  }
</style>
