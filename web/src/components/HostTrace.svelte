<script lang="ts">
  import { live } from '../lib/live.svelte';
  import { bytes, bytesOf, duration, rate } from '../lib/format';

  // Eight separate bento tiles for the host. Each one is a direct child of the
  // page grid, which places it (see app.css).
  const SAMPLES = 40; // about 80 seconds of CPU and network history

  let host = $derived(live.history.at(-1));
  let recent = $derived(live.history.slice(-SAMPLES));

  const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));
  const share = (used: number, total: number) => (total > 0 ? clamp((used / total) * 100, 0, 100) : 0);

  let mem = $derived(host ? share(host.memUsed, host.memTotal) : 0);
  let swap = $derived(host ? share(host.swapUsed, host.swapTotal) : 0);
  let hasTemp = $derived(host?.temp != null);
  let tempBar = $derived(host?.temp != null ? clamp(((host.temp - 30) / 60) * 100, 0, 100) : 0);

  /** "12.4 MB/s" split into the big number and its small unit. */
  function split(bps: number | undefined): [string, string] {
    if (bps == null) return ['–', ''];
    const [n, ...u] = rate(bps).split(' ');
    return [n, u.join(' ')];
  }
  let rx = $derived(split(host?.netRx));
  let tx = $derived(split(host?.netTx));

  function path(values: number[], max: number): string {
    const n = values.length;
    if (n < 2) return '';
    return values
      .map((v, i) => `${i ? 'L' : 'M'}${((i / (n - 1)) * 100).toFixed(1)} ${(38 - clamp(v / max, 0, 1) * 34).toFixed(1)}`)
      .join(' ');
  }
  let netMax = $derived(Math.max(...recent.map((s) => Math.max(s.netRx, s.netTx)), 64 * 1024));
  let rxPath = $derived(path(recent.map((s) => s.netRx), netMax));
  let txPath = $derived(path(recent.map((s) => s.netTx), netMax));
</script>

<section class="tile tile-lemon t-cpu" aria-labelledby="h-cpu">
  <header class="th"><h2 id="h-cpu">CPU</h2></header>
  <div class="big big-xl" aria-label="CPU {host ? Math.round(host.cpu) + ' percent' : 'unknown'}">
    <span>{host ? Math.round(host.cpu) : '–'}</span>{#if host}<small>%</small>{/if}
  </div>
  <div class="spark" aria-hidden="true">
    {#each recent as s, i (i)}<i style:height="{clamp(s.cpu, 2, 100)}%"></i>{/each}
  </div>
  {#if host?.cores?.length}
    <div class="cores" role="list" aria-label="Per-core usage">
      {#each host.cores as c, i (i)}
        <div class="core" role="listitem" title="Core {i}: {Math.round(c)}%">
          <i style:height="{clamp(c, 0, 100)}%"></i><span>{Math.round(c)}</span>
        </div>
      {/each}
    </div>
  {/if}
</section>

<section class="tile tile-mint t-mem" aria-labelledby="h-mem">
  <header class="th"><h2 id="h-mem">Memory</h2></header>
  <div class="big big-l"><span>{host ? Math.round(mem) : '–'}</span>{#if host}<small>%</small>{/if}</div>
  <div class="bar" class:warn={mem >= 85}><i style:width="{mem}%"></i></div>
  <div class="sub">{host ? bytesOf(host.memUsed, host.memTotal) : 'Waiting for the first sample'}</div>
</section>

<section class="tile tile-sky t-swap" aria-labelledby="h-swap">
  <header class="th"><h2 id="h-swap">Swap</h2></header>
  {#if host && host.swapTotal <= 0}
    <div class="big big-l"><span>–</span></div>
    <div class="bar"><i style:width="0%"></i></div>
    <div class="sub">No swap on this host</div>
  {:else}
    <div class="big big-l"><span>{host ? Math.round(swap) : '–'}</span>{#if host}<small>%</small>{/if}</div>
    <div class="bar" class:warn={swap >= 85}><i style:width="{swap}%"></i></div>
    <div class="sub">{host ? bytesOf(host.swapUsed, host.swapTotal) : 'Waiting for the first sample'}</div>
  {/if}
</section>

<section class="tile t-load" aria-labelledby="h-load">
  <header class="th">
    <h2 id="h-load">Load</h2>
    {#if host?.cores?.length}<span class="pill plain">{host.cores.length} cores</span>{/if}
  </header>
  <div class="big big-l">{host?.load.length ? host.load[0].toFixed(2) : '–'}</div>
  <div class="mini" title="Load average over 1, 5 and 15 minutes">
    {#each ['1 min', '5 min', '15 min'] as label, i (label)}
      <div class:on={i === 0}><em>{label}</em><span>{host?.load[i] != null ? host.load[i].toFixed(2) : '–'}</span></div>
    {/each}
  </div>
</section>

<section class="tile tile-tomato t-temp" aria-labelledby="h-temp">
  <header class="th"><h2 id="h-temp">Temp</h2></header>
  <div class="big big-l"><span>{hasTemp ? Math.round(host!.temp!) : '–'}</span>{#if hasTemp}<small>°C</small>{/if}</div>
  {#if hasTemp}
    <div class="thermo">
      <div class="bar"><i style:width="{tempBar}%"></i></div>
      <div class="scale"><span>30</span><span>60</span><span>90°C</span></div>
    </div>
  {:else}
    <div class="sub">{host ? 'No temperature sensor found' : 'Waiting for the first sample'}</div>
  {/if}
</section>

<section class="tile tile-lilac t-net" aria-labelledby="h-net">
  <header class="th"><h2 id="h-net">Network</h2></header>
  <div class="netrow">
    <div class="nm" title="Received">
      <span class="sub">Download</span>
      <div class="big big-m"><span>{rx[0]}</span><small>{rx[1]}</small></div>
    </div>
    <div class="nm" title="Sent">
      <span class="sub">Upload</span>
      <div class="big big-m"><span>{tx[0]}</span><small>{tx[1]}</small></div>
    </div>
  </div>
  <svg class="netsvg" viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden="true">
    <path d={rxPath} />
    <path class="tx" d={txPath} />
  </svg>
</section>

<section class="tile t-disk" aria-labelledby="h-disk">
  <header class="th"><h2 id="h-disk">Disks</h2></header>
  {#if host?.disks?.length}
    <div class="disks">
      {#each host.disks as d (d.mount)}
        {@const p = share(d.used, d.total)}
        <div class="disk">
          <div class="dl">
            <code title={d.mount}>{d.mount}</code>
            <span>{bytesOf(d.used, d.total)}, {Math.round(p)}%</span>
          </div>
          <div class="bar thin" class:warn={p >= 85}><i style:width="{p}%"></i></div>
        </div>
      {/each}
    </div>
  {:else}
    <p class="sub">{host ? 'No disks reported.' : 'Waiting for the first sample'}</p>
  {/if}
</section>

<section class="tile tile-ink t-up" aria-labelledby="h-up">
  <header class="th"><h2 id="h-up">Uptime</h2></header>
  <div class="uptime">{host ? duration(host.uptime) : '–'}</div>
  <div class="sub">since the last reboot</div>
</section>

<style>
  .big {
    font-weight: 800;
    line-height: 0.82;
    letter-spacing: -0.05em;
    font-stretch: 75%;
    font-variation-settings: 'opsz' 96;
    font-variant-numeric: tabular-nums;
  }

  .big small {
    margin-left: 4px;
    font-size: 0.3em;
    font-weight: 700;
    letter-spacing: 0;
  }

  .big-xl {
    font-size: clamp(88px, 15vw, 196px);
  }
  .big-l {
    font-size: clamp(52px, 7vw, 92px);
  }
  .big-m {
    font-size: clamp(42px, 5vw, 64px);
  }

  @media (max-width: 1000px) {
    .big-xl {
      font-size: clamp(96px, 28vw, 200px);
    }
    .big-l {
      font-size: clamp(50px, 11vw, 84px);
    }
  }

  .bar {
    position: relative;
    height: 22px;
    border: 3px solid var(--line);
    border-radius: 999px;
    background: rgb(255 255 255 / 0.5);
    overflow: hidden;
  }

  .bar i {
    display: block;
    height: 100%;
    background: #111;
    border-radius: 999px 0 0 999px;
    transition: width 0.6s cubic-bezier(0.3, 1.4, 0.5, 1);
  }

  .bar.warn i {
    background: var(--tomato);
    border-right: 3px solid var(--line);
  }

  .bar.thin {
    height: 14px;
    border-width: 2.5px;
  }

  .t-disk .bar {
    background: rgb(128 128 128 / 0.25);
  }

  .t-disk .bar i {
    background: var(--fg);
  }

  .spark {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    height: 64px;
  }

  .spark i {
    flex: 1;
    min-width: 2px;
    background: #111;
    border-radius: 3px 3px 0 0;
    transition: height 0.5s;
  }

  .cores {
    display: grid;
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: 8px;
    margin-top: auto;
  }

  .core {
    position: relative;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    height: 76px;
    border: 3px solid var(--line);
    border-radius: 10px;
    background: rgb(255 255 255 / 0.55);
    overflow: hidden;
  }

  .core i {
    position: absolute;
    inset: auto 0 0;
    background: #111;
    transition: height 0.6s cubic-bezier(0.3, 1.4, 0.5, 1);
  }

  .core span {
    position: relative;
    margin-bottom: 3px;
    padding: 0 4px;
    border: 2px solid #111;
    border-radius: 6px;
    background: #fff;
    color: #111;
    font-size: 12px;
    font-weight: 800;
    line-height: 1.3;
  }

  @media (max-width: 560px) {
    .cores {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }

  .mini {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 6px;
    margin-top: auto;
  }

  .mini div {
    padding: 3px 8px;
    border: 2.5px solid var(--line);
    border-radius: 10px;
    background: rgb(255 255 255 / 0.55);
    color: #111;
    font-size: 18px;
    font-weight: 800;
    line-height: 1.2;
    font-variant-numeric: tabular-nums;
  }

  .mini div.on {
    background: #111;
    color: #f4f2ec;
  }

  .mini em {
    display: block;
    font-size: 12px;
    font-style: normal;
    font-weight: 700;
    opacity: 0.7;
    white-space: nowrap;
  }

  @media (max-width: 560px) {
    .mini {
      gap: 4px;
    }

    .mini div {
      min-width: 0;
      padding: 3px 3px;
      font-size: 14px;
      text-align: center;
    }

    .mini em {
      font-size: 10px;
      letter-spacing: -0.02em;
    }
  }

  .thermo {
    margin-top: auto;
  }

  .scale {
    display: flex;
    justify-content: space-between;
    margin-top: 4px;
    font-size: 12px;
    font-weight: 700;
  }

  .netrow {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }

  .nm {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .netsvg {
    width: 100%;
    height: 64px;
    margin-top: auto;
    border: 3px solid var(--line);
    border-radius: 12px;
    background: rgb(255 255 255 / 0.5);
  }

  .netsvg path {
    fill: none;
    stroke: #111;
    stroke-width: 3;
    stroke-linejoin: round;
    vector-effect: non-scaling-stroke;
  }

  .netsvg path.tx {
    stroke: var(--tomato);
    stroke-width: 4;
  }

  .disks {
    display: grid;
    gap: 12px;
    margin-top: 4px;
  }

  .disk {
    display: grid;
    gap: 4px;
  }

  .dl {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: var(--fs-s);
    font-weight: 700;
  }

  .dl code {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--mono);
    font-size: 13px;
    font-weight: 600;
  }

  .dl span {
    flex: none;
  }

  .uptime {
    font-size: clamp(30px, 4vw, 46px);
    font-weight: 800;
    line-height: 1;
    letter-spacing: -0.03em;
    font-stretch: 75%;
    font-variant-numeric: tabular-nums;
  }
</style>
