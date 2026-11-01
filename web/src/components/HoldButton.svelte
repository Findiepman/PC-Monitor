<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';

  let {
    label,
    onconfirm,
    children,
    duration = 650,
    danger = false,
    disabled = false,
    tone = 'plain',
  }: {
    label: string;
    onconfirm: () => void;
    children: Snippet;
    duration?: number;
    danger?: boolean;
    disabled?: boolean;
    tone?: 'plain' | 'mint' | 'lemon' | 'tomato';
  } = $props();

  let holding = $state(false);
  // Assistive tech sends a click without pointer or key events, so it can't
  // hold. A first such click arms the button; a second confirms.
  let armed = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let armTimer: ReturnType<typeof setTimeout> | undefined;
  let pressed = false;
  // Last time Enter or Space touched this button. Clicks the browser makes from
  // those keys (detail 0) are part of the key hold, never the assistive-tech path.
  let keyAt = -Infinity;
  let keyDown = false;

  $effect(() => {
    if (disabled) {
      keyDown = false;
      holding = false;
      clearTimeout(timer);
    }
  });

  onDestroy(() => {
    clearTimeout(timer);
    clearTimeout(armTimer);
  });

  function begin() {
    if (disabled || holding) return;
    pressed = true;
    holding = true;
    timer = setTimeout(() => {
      holding = false;
      // The action may disable this button; its keyup could then be lost.
      keyDown = false;
      onconfirm();
    }, duration);
  }

  function cancel() {
    holding = false;
    clearTimeout(timer);
    // The click that follows pointerup/keyup belongs to this press.
    setTimeout(() => (pressed = false), 0);
  }

  // Touch pointers are implicitly captured, so pointerleave never fires when a
  // finger slides off the button. Cancel by hit-testing the pointer instead.
  function move(e: PointerEvent) {
    if (!holding) return;
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) cancel();
  }

  function click(e: MouseEvent) {
    if (pressed) {
      pressed = false;
      return;
    }
    if (e.detail !== 0 || disabled) return;
    if (keyDown || performance.now() - keyAt < 600) return;
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
  class="hold {tone}"
  class:holding
  class:danger
  {disabled}
  aria-label={armed ? `Confirm: ${label}` : `${label} (press and hold)`}
  title={`Hold to ${label.toLowerCase()}`}
  style:--hold="{duration}ms"
  onpointerdown={(e) => e.button === 0 && begin()}
  onpointerup={cancel}
  onpointerleave={cancel}
  onpointermove={move}
  onblur={() => {
    keyDown = false;
    cancel();
  }}
  onpointercancel={cancel}
  oncontextmenu={(e) => e.preventDefault()}
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      // Always, repeats included: otherwise the browser turns each repeat into a click.
      e.preventDefault();
      keyDown = true;
      keyAt = performance.now();
      if (!e.repeat) begin();
    }
  }}
  onkeyup={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      keyDown = false;
      keyAt = performance.now();
      cancel();
    }
  }}
  onclick={click}
>
  <span class="fill" aria-hidden="true"></span>
  {@render children()}
</button>

<style>
  .hold {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    min-width: 40px;
    height: 40px;
    padding: 0 12px;
    overflow: hidden;
    border: 3px solid var(--line);
    border-radius: 12px;
    background: var(--card);
    color: var(--fg);
    font-weight: 700;
    font-size: var(--fs-s);
    box-shadow: 3px 3px 0 var(--line);
    transition:
      transform 0.08s,
      box-shadow 0.08s;
    /* A hold must not scroll, zoom, select text or open a callout/menu. */
    touch-action: manipulation;
    user-select: none;
    -webkit-user-select: none;
    -webkit-touch-callout: none;
  }

  .hold:hover:not(:disabled) {
    background: var(--lemon);
    color: #111;
  }

  .mint,
  .lemon,
  .tomato {
    color: #111;
    --line: #111;
  }

  .mint {
    background: var(--mint);
  }

  .lemon {
    background: var(--lemon);
  }

  .tomato {
    background: var(--tomato);
  }

  .mint:hover:not(:disabled) {
    background: var(--mint);
  }

  .lemon:hover:not(:disabled) {
    background: var(--lemon);
  }

  .tomato:hover:not(:disabled) {
    background: var(--tomato);
  }

  .mint:hover:not(:disabled),
  .lemon:hover:not(:disabled),
  .tomato:hover:not(:disabled) {
    transform: translate(-1px, -1px);
    box-shadow: 4px 4px 0 var(--line);
  }

  .mint .fill,
  .lemon .fill,
  .tomato .fill,
  .danger.tomato .fill {
    background: rgb(0 0 0 / 0.28);
  }

  .hold:disabled {
    opacity: 0.4;
    cursor: not-allowed;
    box-shadow: none;
    transform: translate(3px, 3px);
  }

  .fill {
    position: absolute;
    inset: 0;
    background: var(--mint);
    transform: scaleX(0);
    transform-origin: left;
    pointer-events: none;
  }

  .danger .fill {
    background: var(--tomato);
  }

  .holding {
    color: #111;
    transform: translate(2px, 2px);
    box-shadow: 1px 1px 0 var(--line);
  }

  .hold.holding:hover:not(:disabled) {
    transform: translate(2px, 2px);
    box-shadow: 1px 1px 0 var(--line);
  }

  .holding .fill {
    transform: scaleX(1);
    transition: transform var(--hold) linear;
  }

  /* Children (icon, label) sit above the fill. */
  .hold > :global(*:not(.fill)) {
    position: relative;
    pointer-events: none;
  }

  @media (pointer: coarse) {
    .hold {
      min-width: 44px;
      height: 44px;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .hold {
      transition: none;
    }
  }
</style>
