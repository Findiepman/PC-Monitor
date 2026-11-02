<script lang="ts">
  import type { Action, Provider, Unit } from '../lib/types';
  import { live } from '../lib/live.svelte';
  import { bytes, duration, pct } from '../lib/format';
  import { isTransitional, stateLabel, tone } from '../lib/status';
  import Led from './Led.svelte';
  import HoldButton from './HoldButton.svelte';
  import Icon from './Icon.svelte';

  let {
    provider,
    metaState,
    pending,
    onaction,
    onlogs,
  }: {
    provider: Provider | undefined;
    metaState: 'loading' | 'ok' | 'failed';
    pending: Record<string, Action>;
    onaction: (u: Unit, a: Action) => void;
    onlogs: (u: Unit) => void;
  } = $props();

  let bots = $derived(provider ? live.units.filter((u) => u.provider === provider.name) : []);
  let error = $derived(provider ? live.errors[provider.name] : undefined);
  let running = $derived(bots.filter((b) => b.state === 'running').length);
  let loading = $derived(metaState === 'loading' || (live.status !== 'live' && bots.length === 0 && !error));

  // One line per account: "name" or "name<TAB>Discord name".
  const accounts = (m: Record<string, string> | undefined) =>
    (m?.accounts ? m.accounts.split('\n') : []).map((line) => {
      const [name, discord] = line.split('\t');
      return { name, discord: discord && discord !== name ? discord : '' };
    });

  const progressive: Record<Action, string> = {
    start: 'starting…',
    stop: 'stopping…',
    restart: 'restarting…',
    kill: 'killing…',
  };
</script>

<main class="bento bots">
  <section class="tile tile-lemon t-bhead" aria-labelledby="h-bots">
    <header class="th">
      <h2 id="h-bots">{provider?.label ?? 'Discord bots'}</h2>
      {#if bots.length}
        <span class="pill plain count" role="status">{running} of {bots.length} running</span>
      {/if}
    </header>
    <p class="sub">
      What pm2 runs on this server, refreshed every few seconds. Hold a button until it fills to start, restart or
      stop a bot; Logs opens its output on the dashboard.
    </p>
  </section>

  {#if metaState === 'ok' && !provider}
    <section class="tile t-bnote">
      <div class="empty">
        <b>The bot list isn't set up</b>
        <p>Add a <span class="mono">pm2:</span> block under providers in the dashd config and start dashd-pm2.service (see DEPLOY.md).</p>
      </div>
    </section>
  {:else if metaState === 'failed' && !provider}
    <section class="tile t-bnote">
      <div class="empty" role="alert"><b>Couldn't load the dashboard setup</b><p>Reload the page to try again.</p></div>
    </section>
  {:else if error}
    <section class="tile tile-tomato t-bnote" role="alert">
      <b>Can't see the bots right now</b>
      <p class="mono err">{error}</p>
    </section>
  {:else if loading}
    <div class="t-bnote skels" aria-busy="true" aria-label="Loading bots">
      <div class="skel"></div><div class="skel"></div>
    </div>
  {:else if bots.length === 0}
    <section class="tile t-bnote">
      <div class="empty"><b>pm2 isn't running any processes</b><p>Bots you start with pm2 show up here.</p></div>
    </section>
  {/if}

  {#if !error && bots.length}
    <ul class="blist">
      {#each bots as b (b.id)}
        {@const t = tone(b)}
        {@const accts = accounts(b.meta)}
        {@const busy = !!pending[b.id] || isTransitional(b)}
        <li class="tile bot" class:off={b.state !== 'running'} class:bad={t === 'fail'}>
          <div class="bh">
            <Led tone={t} />
            <h3 class="bname" title={b.name}>{b.name}</h3>
            <span class="pill {busy ? 'warn' : t}">{pending[b.id] ? progressive[pending[b.id]] : stateLabel(b)}</span>
          </div>

          {#if b.meta?.discord}
            <p class="discord" title="Discord name, from the bot's own log">
              <span class="dlabel">On Discord as</span> <b>{b.meta.discord}</b>
            </p>
          {/if}

          {#if b.about}<p class="about">{b.about}</p>{/if}

          <dl class="facts">
            <div><dt>Up</dt><dd class="mono">{b.state === 'running' ? duration(b.uptime) : '–'}</dd></div>
            <div><dt>CPU</dt><dd class="mono">{pct(b.cpu)}</dd></div>
            <div><dt>Mem</dt><dd class="mono">{bytes(b.mem)}</dd></div>
            <div><dt>Restarts</dt><dd class="mono">{b.meta?.restarts ?? '0'}</dd></div>
          </dl>

          {#if b.meta?.description}
            <p class="runtime"><span class="chip mono">{b.meta.description}</span></p>
          {/if}

          {#if accts.length}
            <div class="accts">
              <p class="alabel">Signs in {accts.length} {accts.length === 1 ? 'account' : 'accounts'}</p>
              <ul class="chips" aria-label="Bot accounts in {b.name}">
                {#each accts as a, i (i)}
                  <li class="chip" title={a.discord ? `${a.name} is ${a.discord} on Discord` : a.name}>
                    {a.name}{#if a.discord}<span class="sr-only"> is </span><span class="as">{a.discord}</span><span class="sr-only"> on Discord</span>{/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <div class="bacts">
            <button class="btn" onclick={() => onlogs(b)} aria-label="Show logs for {b.name}">
              <Icon name="logs" size={16} /><span>Logs</span>
            </button>
            {#if b.actions.includes('start')}
              <HoldButton label="Start {b.name}" tone="mint" disabled={busy} onconfirm={() => onaction(b, 'start')}>
                <Icon name="start" size={16} /><span>Start</span>
              </HoldButton>
            {/if}
            {#if b.actions.includes('restart')}
              <HoldButton label="Restart {b.name}" tone="lemon" disabled={busy} onconfirm={() => onaction(b, 'restart')}>
                <Icon name="restart" size={16} /><span>Restart</span>
              </HoldButton>
            {/if}
            {#if b.actions.includes('stop')}
              <HoldButton label="Stop {b.name}" tone="tomato" danger disabled={busy} onconfirm={() => onaction(b, 'stop')}>
                <Icon name="stop" size={16} /><span>Stop</span>
              </HoldButton>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  .t-bhead,
  .t-bnote {
    grid-column: span 12;
  }

  .count {
    min-height: 28px;
    background: var(--card);
  }

  .blist {
    grid-column: span 12;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
    gap: 18px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .bot {
    gap: 12px;
  }

  .bot:nth-child(3n + 1) {
    --accent: var(--mint);
  }
  .bot:nth-child(3n + 2) {
    --accent: var(--sky);
  }
  .bot:nth-child(3n) {
    --accent: var(--lilac);
  }

  .bot::before {
    content: '';
    position: absolute;
    inset: 0 0 auto;
    height: 11px;
    border-bottom: 3px solid var(--edge);
    /* inside the tile's border, so the corners follow its curve */
    border-radius: calc(var(--r) - 3px) calc(var(--r) - 3px) 0 0;
    background: var(--accent);
  }

  .bot.off::before {
    background: repeating-linear-gradient(-45deg, var(--card) 0 6px, rgb(128 128 128 / 0.35) 6px 12px);
  }

  .bot.bad::before {
    background: var(--tomato);
  }

  .bh {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    margin-top: 8px;
  }

  .bname {
    flex: 1;
    min-width: 0;
    font-size: var(--fs-l);
    font-weight: 800;
    letter-spacing: -0.02em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .off .bname {
    opacity: 0.75;
  }

  .bh .pill {
    flex: none;
  }

  .discord {
    margin-top: -4px;
    font-size: var(--fs-s);
    overflow-wrap: anywhere;
  }

  .dlabel {
    color: var(--sub);
    font-weight: 600;
  }

  .chip .as {
    margin-left: 6px;
    padding-left: 6px;
    border-left: 2px solid var(--line);
    font-weight: 600;
    color: var(--sub);
  }

  .bacts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: auto;
  }

  .about {
    font-size: 13px;
    font-weight: 500;
    color: var(--sub);
  }

  .facts {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 8px;
    margin: 0;
  }

  .facts div {
    min-width: 0;
    padding: 6px 8px;
    border: 2.5px solid var(--line);
    border-radius: 12px;
    background: var(--paper);
  }

  .facts dt {
    font-size: var(--fs-xs);
    font-weight: 700;
    color: var(--sub);
  }

  .facts dd {
    margin: 0;
    font-size: 15px;
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .runtime {
    display: flex;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    min-height: 26px;
    padding: 0 10px;
    border: 2.5px solid var(--line);
    border-radius: 999px;
    background: var(--card);
    color: var(--fg);
    font-size: 13px;
    font-weight: 700;
    max-width: 100%;
  }

  /* Account chips wrap instead of clipping long Discord names. */
  .chips .chip {
    display: inline-block;
    padding: 2px 10px;
    border-radius: 14px;
    line-height: 1.35;
    overflow-wrap: anywhere;
  }

  .runtime .chip {
    background: var(--accent);
    color: #111;
    --line: #111;
  }

  .accts {
    display: grid;
    gap: 6px;
  }

  .alabel {
    font-size: var(--fs-s);
    font-weight: 700;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .off .chips .chip {
    opacity: 0.6;
  }

  .err {
    font-size: var(--fs-s);
    overflow-wrap: anywhere;
  }

  .skels {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
    gap: 18px;
  }

  .skel {
    height: 200px;
    border: 3px solid var(--line);
    border-radius: var(--r);
    background: linear-gradient(90deg, rgb(128 128 128 / 0.12) 25%, rgb(128 128 128 / 0.28) 50%, rgb(128 128 128 / 0.12) 75%);
    background-size: 300% 100%;
    animation: shim 1.1s linear infinite;
  }

  @keyframes shim {
    to {
      background-position: -300% 0;
    }
  }

  .empty {
    padding: 18px;
    border: 3px dashed var(--line);
    border-radius: 16px;
    text-align: center;
    font-weight: 700;
  }

  .empty p {
    margin-top: 4px;
    font-weight: 500;
    color: var(--sub);
  }

  @media (max-width: 420px) {
    .facts {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .skel {
      animation: none;
    }
  }
</style>
