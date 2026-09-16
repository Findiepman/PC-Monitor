<script lang="ts">
  import { onMount } from 'svelte';
  import { live } from '../lib/live.svelte';
  import { bytesOf, pct, rate } from '../lib/format';

  const WINDOW = 300_000; // five minutes on screen
  const INTERVAL = 2_000; // host sample period

  let canvas: HTMLCanvasElement;
  let host = $derived(live.history.at(-1));

  function level(fraction: number): string {
    if (fraction >= 0.95) return 'fail';
    if (fraction >= 0.85) return 'warn';
    return '';
  }

  onMount(() => {
    const ctx = canvas.getContext('2d')!;
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    let colors = readColors();
    let raf = 0;
    let dirty = true;

    function readColors() {
      const s = getComputedStyle(canvas);
      return {
        trace: s.getPropertyValue('--trace').trim(),
        soft: s.getPropertyValue('--trace-soft').trim(),
        grid: s.getPropertyValue('--trace-grid').trim(),
        dim: s.getPropertyValue('--dim').trim(),
      };
    }

    const scheme = matchMedia('(prefers-color-scheme: light)');
    const onScheme = () => {
      colors = readColors();
      dirty = true;
    };
    scheme.addEventListener('change', onScheme);

    const ro = new ResizeObserver(() => (dirty = true));
    ro.observe(canvas);

    function frame() {
      raf = requestAnimationFrame(frame);
      const hist = live.history;
      if (hist.length === 0) return;
      const animate = !reduced.matches;
      if (!animate && !dirty && canvas.dataset.t === String(hist.at(-1)!.t)) return;
      dirty = false;
      canvas.dataset.t = String(hist.at(-1)!.t);
      draw(hist, animate);
    }

    function draw(hist: typeof live.history, animate: boolean) {
      const dpr = window.devicePixelRatio || 1;
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      // Draw one interval behind real time so the head can glide between
      // samples instead of jumping every two seconds.
      const last = hist.at(-1)!.t;
      const elapsed = animate ? Math.min(performance.now() - live.hostArrivedAt, INTERVAL) : INTERVAL;
      const now = last - INTERVAL + elapsed;
      const x = (t: number) => w - ((now - t) / WINDOW) * w;
      const pad = 6;
      const y = (v: number) => pad + (1 - Math.min(Math.max(v, 0), 100) / 100) * (h - pad * 2);

      // Graticule: horizontal quarters, vertical every 30s moving with time.
      ctx.lineWidth = 1;
      ctx.strokeStyle = colors.grid;
      ctx.beginPath();
      for (const v of [25, 50, 75]) {
        const yy = Math.round(y(v)) + 0.5;
        ctx.moveTo(0, yy);
        ctx.lineTo(w, yy);
      }
      for (let t = Math.floor(now / 30_000) * 30_000; t > now - WINDOW; t -= 30_000) {
        const xx = Math.round(x(t)) + 0.5;
        ctx.moveTo(xx, 0);
        ctx.lineTo(xx, h);
      }
      ctx.stroke();

      // Points up to `now`, with the last one interpolated.
      const series = (pick: (s: (typeof hist)[number]) => number) => {
        const pts: [number, number][] = [];
        for (let i = 0; i < hist.length; i++) {
          const s = hist[i];
          if (s.t <= now) {
            pts.push([x(s.t), y(pick(s))]);
          } else {
            const prev = hist[i - 1];
            if (prev) {
              const f = (now - prev.t) / (s.t - prev.t);
              pts.push([w, y(pick(prev) + (pick(s) - pick(prev)) * f)]);
            }
            break;
          }
        }
        return pts;
      };

      const mem = series((s) => (s.memTotal ? (s.memUsed / s.memTotal) * 100 : 0));
      ctx.strokeStyle = colors.dim;
      ctx.setLineDash([3, 3]);
      ctx.beginPath();
      mem.forEach(([px, py], i) => (i ? ctx.lineTo(px, py) : ctx.moveTo(px, py)));
      ctx.stroke();
      ctx.setLineDash([]);

      const cpu = series((s) => s.cpu);
      if (cpu.length > 1) {
        ctx.beginPath();
        cpu.forEach(([px, py], i) => (i ? ctx.lineTo(px, py) : ctx.moveTo(px, py)));
        ctx.lineTo(cpu.at(-1)![0], h);
        ctx.lineTo(cpu[0][0], h);
        ctx.closePath();
        ctx.fillStyle = colors.soft;
        ctx.fill();

        ctx.beginPath();
        cpu.forEach(([px, py], i) => (i ? ctx.lineTo(px, py) : ctx.moveTo(px, py)));
        ctx.strokeStyle = colors.trace;
        ctx.lineWidth = 1.75;
        ctx.lineJoin = 'round';
        ctx.stroke();

        const [hx, hy] = cpu.at(-1)!;
        ctx.fillStyle = colors.trace;
        ctx.beginPath();
        ctx.arc(hx - 1, hy, 3, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    raf = requestAnimationFrame(frame);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      scheme.removeEventListener('change', onScheme);
    };
  });
</script>

<section class="scope" aria-label="Host resources">
  <div class="screen">
    <canvas bind:this={canvas} aria-hidden="true"></canvas>
    <span class="axis top mono">100</span>
    <span class="axis mid mono">50</span>
    <span class="axis left mono">5 min ago</span>
    <span class="axis right mono">now</span>
  </div>

  <div class="readouts">
    <div class="cpu">
      <div class="row">
        <span class="label"><i class="key trace"></i>CPU</span>
        <span class="big mono">{host ? pct(host.cpu) : '–'}</span>
      </div>
      {#if host?.cores}
        <div class="cores" aria-label="Per-core usage">
          {#each host.cores as c, i (i)}
            <span class="core" style:--v={Math.max(c, 2) / 100} title="Core {i}: {pct(c)}"></span>
          {/each}
        </div>
      {/if}
      <span class="sub mono">
        {host?.cores?.length ?? '–'} cores{#if host?.temp != null}, {Math.round(host.temp)} °C{/if}
      </span>
    </div>

    {#if host}
      <dl>
        <div>
          <dt><i class="key mem"></i>Memory</dt>
          <dd class="mono">{bytesOf(host.memUsed, host.memTotal)}</dd>
          <span class="bar {level(host.memUsed / host.memTotal)}" style:--v={host.memUsed / host.memTotal}></span>
        </div>
        {#if host.swapTotal > 0}
          <div>
            <dt>Swap</dt>
            <dd class="mono">{bytesOf(host.swapUsed, host.swapTotal)}</dd>
            <span class="bar {level(host.swapUsed / host.swapTotal)}" style:--v={host.swapUsed / host.swapTotal}></span>
          </div>
        {/if}
        {#each host.disks ?? [] as d (d.mount)}
          <div>
            <dt class="mono path" title={d.mount}>{d.mount}</dt>
            <dd class="mono">{bytesOf(d.used, d.total)}</dd>
            <span class="bar {level(d.used / d.total)}" style:--v={d.used / d.total}></span>
          </div>
        {/each}
        <div>
          <dt>Network</dt>
          <dd class="mono net">
            <span title="Received">↓ {rate(host.netRx)}</span>
            <span title="Sent">↑ {rate(host.netTx)}</span>
          </dd>
        </div>
      </dl>
    {/if}
  </div>
</section>

<style>
  .scope {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    border-bottom: 1px solid var(--rule);
    background: var(--plate);
  }

  .screen {
    position: relative;
    min-height: 196px;
    border-right: 1px solid var(--rule);
    background: var(--bay);
  }

  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
  }

  .axis {
    position: absolute;
    font-size: 10px;
    color: var(--dim);
    pointer-events: none;
  }

  .top {
    top: 4px;
    left: 8px;
  }

  .mid {
    top: calc(50% - 8px);
    left: 8px;
  }

  .left {
    bottom: 4px;
    left: 8px;
  }

  .right {
    bottom: 4px;
    right: 8px;
  }

  .readouts {
    display: grid;
    align-content: start;
    gap: 14px;
    padding: 14px 16px;
  }

  .row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }

  .label,
  dt {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--fs-s);
    color: var(--dim);
  }

  .key {
    display: inline-block;
    width: 12px;
    height: 0;
    border-top: 2px solid var(--trace);
  }

  .key.mem {
    border-top: 1px dashed var(--dim);
  }

  .big {
    font-size: var(--fs-xl);
    line-height: 1;
    font-weight: 500;
    color: var(--trace);
    letter-spacing: -0.02em;
  }

  .cores {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    height: 22px;
    margin: 8px 0 4px;
  }

  .core {
    flex: 1;
    max-width: 10px;
    height: calc(var(--v) * 100%);
    background: var(--trace);
    opacity: 0.75;
  }

  .sub {
    font-size: 11px;
    color: var(--dim);
  }

  dl {
    display: grid;
    gap: 10px;
    margin: 0;
  }

  dl > div {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    column-gap: 12px;
    row-gap: 4px;
    align-items: baseline;
  }

  dd {
    margin: 0;
    font-size: var(--fs-xs);
    text-align: right;
  }

  .path {
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }

  .net {
    display: flex;
    gap: 12px;
  }

  .bar {
    grid-column: 1 / -1;
    height: 3px;
    background: linear-gradient(var(--ink), var(--ink)) no-repeat left / calc(var(--v) * 100%) 100%, var(--rule);
  }

  .bar.warn {
    background: linear-gradient(var(--warn), var(--warn)) no-repeat left / calc(var(--v) * 100%) 100%, var(--rule);
  }

  .bar.fail {
    background: linear-gradient(var(--fail), var(--fail)) no-repeat left / calc(var(--v) * 100%) 100%, var(--rule);
  }

  @media (max-width: 899px) {
    .scope {
      grid-template-columns: minmax(0, 1fr);
    }

    .screen {
      min-height: 132px;
      border-right: 0;
      border-bottom: 1px solid var(--rule);
    }

    .readouts {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1.3fr);
      column-gap: 20px;
    }
  }

  @media (max-width: 420px) {
    .readouts {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
