<script lang="ts">
  import { live } from '../lib/live.svelte';
  import { pastTense } from '../lib/format';

  const time = new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
</script>

<section class="tile tile-sky t-act" aria-labelledby="h-act">
  <header class="th">
    <h2 id="h-act">Activity</h2>
    {#if live.audit.length}<span class="pill plain">{live.audit.length}</span>{/if}
  </header>
  {#if live.audit.length === 0}
    <div class="empty">
      <b>Nothing yet</b>
      <p>Restarts, stops and starts done from this dashboard show up here.</p>
    </div>
  {:else}
    <ol class="alist" aria-live="polite">
      {#each live.audit as e, i (e.time + e.unit + i)}
        <li class="aitem" class:bad={!e.ok}>
          <div class="l1">
            <span class="res" class:bad={!e.ok}>{e.ok ? 'OK' : 'Failed'}</span>
            <span class="what">
              {#if e.ok}
                {pastTense[e.action] ?? e.action} <span class="mono unit">{e.unit}</span>
              {:else}
                Couldn't {e.action} <span class="mono unit">{e.unit}</span>
              {/if}
            </span>
          </div>
          <time class="mono" datetime={e.time}>{time.format(new Date(e.time))}</time>
          <span class="who">{e.user} from {e.ip}</span>
          {#if e.error}
            <span class="why mono">{e.error}</span>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .empty {
    padding: 18px;
    border: 3px dashed var(--line);
    border-radius: 16px;
    background: var(--card);
    color: #111;
    text-align: center;
    font-weight: 700;
  }

  .empty p {
    margin-top: 4px;
    font-weight: 500;
    color: var(--sub);
  }

  .alist {
    display: grid;
    align-content: start;
    gap: 10px;
    flex: 1 1 0;
    min-height: 200px;
    margin: 0 -6px 0 -2px;
    padding: 2px 6px 6px 2px;
    list-style: none;
    overflow: auto;
    overscroll-behavior: contain;
  }

  @media (max-width: 1000px) {
    .alist {
      flex: none;
      max-height: 420px;
    }
  }

  .aitem {
    display: grid;
    gap: 3px;
    padding: 8px 10px;
    border: 3px solid var(--line);
    border-radius: 14px;
    background: var(--card);
    color: #111;
    box-shadow: 3px 3px 0 var(--line);
  }

  .l1 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
    font-weight: 700;
  }

  .what {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .res {
    flex: none;
    padding: 0 8px;
    border: 2.5px solid #111;
    border-radius: 8px;
    background: var(--mint);
    font-size: 12px;
    font-weight: 800;
  }

  .res.bad {
    background: var(--tomato);
  }

  .unit {
    padding: 0 6px;
    border: 2px solid var(--line);
    border-radius: 6px;
    background: var(--lemon);
    font-size: 12px;
  }

  time,
  .who {
    font-size: 12px;
    font-weight: 600;
    color: var(--sub);
  }

  .why {
    padding: 2px 6px;
    border: 2px solid #111;
    border-radius: 8px;
    background: var(--tomato);
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  @media (prefers-reduced-motion: no-preference) {
    .aitem:first-child {
      animation: slidein 0.5s cubic-bezier(0.3, 1.6, 0.5, 1);
    }
  }

  @keyframes slidein {
    from {
      transform: translateX(30px) rotate(2deg);
      opacity: 0;
    }
  }
</style>
