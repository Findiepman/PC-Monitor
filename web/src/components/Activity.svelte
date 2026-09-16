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

<section class="activity" aria-label="Activity">
  {#if live.audit.length === 0}
    <p class="blank">Restarts, stops and starts done from this dashboard show up here.</p>
  {:else}
    <ol>
      {#each live.audit as e, i (e.time + e.unit + i)}
        <li class:bad={!e.ok}>
          <time class="mono" datetime={e.time}>{time.format(new Date(e.time))}</time>
          <span class="what">
            {#if e.ok}
              {pastTense[e.action] ?? e.action} <span class="mono unit">{e.unit}</span>
            {:else}
              Couldn't {e.action} <span class="mono unit">{e.unit}</span>
            {/if}
          </span>
          <span class="who mono">{e.user} from {e.ip}</span>
          {#if e.error}
            <span class="error mono">{e.error}</span>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .activity {
    overflow: auto;
    min-height: 0;
  }

  .blank {
    margin: 0;
    padding: 32px 16px;
    font-size: var(--fs-s);
    color: var(--dim);
    text-align: center;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  li {
    display: grid;
    grid-template-columns: 150px minmax(0, 1fr);
    column-gap: 12px;
    row-gap: 2px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--rule);
    font-size: var(--fs-s);
  }

  li.bad {
    border-left: 2px solid var(--fail);
    padding-left: 14px;
  }

  time {
    font-size: 11px;
    color: var(--dim);
    padding-top: 2px;
  }

  .unit {
    font-size: 12px;
  }

  .who,
  .error {
    grid-column: 2;
    font-size: 11px;
    color: var(--dim);
  }

  .error {
    color: var(--fail);
  }

  @media (max-width: 560px) {
    li {
      grid-template-columns: minmax(0, 1fr);
    }

    .who,
    .error {
      grid-column: 1;
    }
  }
</style>
