<script lang="ts">
  import { api, ApiError } from '../lib/api';
  import { currentTheme, onThemeChange, setTheme } from '../lib/theme';
  import { onMount } from 'svelte';

  let { onsignin }: { onsignin: () => void } = $props();

  let username = $state('');
  let password = $state('');
  let code = $state('');
  let error = $state('');
  let locked = $state('');
  let busy = $state(false);
  let dark = $state(currentTheme() === 'dark');

  onMount(() => onThemeChange(() => (dark = currentTheme() === 'dark')));

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    locked = '';
    try {
      await api.login(username.trim(), password, code.replace(/\s/g, ''));
      onsignin();
    } catch (err) {
      // Too many attempts gets its own tile; every other failure stays in the form.
      if (err instanceof ApiError && err.status === 429) locked = err.message;
      else error = (err as Error).message;
      code = '';
      busy = false;
    }
  }
</script>

<main class="login">
  <div class="wrap">
    <div class="lt">
      <button
        class="switch"
        role="switch"
        aria-checked={dark}
        aria-label="Dark mode"
        title="Dark mode"
        onclick={() => setTheme(dark ? 'light' : 'dark')}
      >
        <span class="trk"><span class="knob"></span></span>
        <span>Dark</span>
      </button>
    </div>

    <div class="lgrid">
      <section class="tile tile-lemon l-brand" aria-hidden="true">
        <span class="sticker">private dashboard</span>
        <span class="word">dashd</span>
        <div class="blobs"><i></i><i></i><i></i><i></i></div>
      </section>

      <section class="tile l-form" aria-labelledby="login-h">
        <h1 id="login-h">Sign in to dashd</h1>
        <form onsubmit={submit} aria-describedby={error ? 'login-error' : undefined}>
          {#if error}
            <p id="login-error" class="error" role="alert">{error}</p>
          {/if}
          <label>
            <span>Username</span>
            <input bind:value={username} autocomplete="username" autocapitalize="off" spellcheck="false" required />
          </label>
          <label>
            <span>Password</span>
            <input type="password" bind:value={password} autocomplete="current-password" required />
          </label>
          <label>
            <span>Code from your authenticator app</span>
            <input
              class="mono code"
              bind:value={code}
              inputmode="numeric"
              autocomplete="one-time-code"
              pattern="[0-9 ]*"
              maxlength="7"
              placeholder="000000"
              required
            />
          </label>
          <button type="submit" class="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
        </form>
      </section>

      <section class="tile tile-lilac l-note">
        <h2>Three things to sign in</h2>
        <p class="sub">Your username, your password and the 6-digit code from your authenticator app. Too many wrong attempts lock this device out for a while.</p>
      </section>

      {#if locked}
        <section class="tile tile-mint l-sticker" role="alert">
          <h2>Locked out</h2>
          <p class="msg">{locked}</p>
        </section>
      {/if}
    </div>
  </div>
</main>

<style>
  .login {
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 20px 22px 30px 16px;
  }

  .wrap {
    width: min(980px, 100%);
  }

  .lt {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 12px;
  }

  .lgrid {
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: 18px;
  }

  .l-brand {
    grid-column: span 3;
    grid-row: span 2;
    justify-content: space-between;
    min-height: 340px;
    overflow: hidden;
  }

  .l-form {
    grid-column: span 3;
    grid-row: span 2;
  }

  .l-note,
  .l-sticker {
    grid-column: span 3;
  }

  .sticker {
    align-self: flex-start;
    padding: 3px 12px;
    border: 3px solid #111;
    border-radius: 14px;
    background: #fff;
    box-shadow: 3px 3px 0 #111;
    font-weight: 800;
    transform: rotate(-4deg);
  }

  .word {
    font-size: clamp(76px, 13vw, 150px);
    font-weight: 800;
    line-height: 0.8;
    letter-spacing: -0.07em;
    font-stretch: 75%;
    font-variation-settings: 'opsz' 96;
  }

  .blobs {
    display: flex;
    gap: 10px;
  }

  .blobs i {
    width: 44px;
    height: 44px;
    border: 3px solid #111;
    border-radius: 50%;
    background: var(--tomato);
    transition: transform 0.4s cubic-bezier(0.3, 2, 0.5, 1);
  }

  .blobs i:nth-child(2) {
    background: var(--mint);
    border-radius: 12px;
  }

  .blobs i:nth-child(3) {
    background: var(--lilac);
    border-radius: 50% 50% 0 50%;
  }

  .blobs i:nth-child(4) {
    background: var(--sky);
    border-radius: 12px 50%;
  }

  .l-brand:hover .blobs i:nth-child(odd) {
    transform: translateY(-10px) rotate(20deg);
  }

  .l-brand:hover .blobs i:nth-child(even) {
    transform: translateY(-4px) rotate(-20deg);
  }

  h1 {
    font-size: 28px;
    font-weight: 800;
    letter-spacing: -0.03em;
    line-height: 1.1;
  }

  h2 {
    font-size: 20px;
    font-weight: 800;
  }

  .msg {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  form {
    display: grid;
    gap: 12px;
  }

  label {
    display: grid;
    gap: 5px;
    font-size: var(--fs-s);
    font-weight: 800;
  }

  input {
    min-height: 48px;
    font-size: var(--fs-m);
  }

  input:focus-visible {
    outline-offset: 2px;
  }

  .code {
    font-size: 24px;
    letter-spacing: 0.3em;
    text-align: center;
  }

  .error {
    padding: 8px 12px;
    border: 3px solid #111;
    border-radius: 14px;
    background: var(--tomato);
    color: #111;
    font-size: var(--fs-s);
    font-weight: 700;
    overflow-wrap: anywhere;
    animation: shake 0.45s;
  }

  @keyframes shake {
    20% {
      transform: translateX(-8px);
    }
    40% {
      transform: translateX(8px);
    }
    60% {
      transform: translateX(-5px);
    }
    80% {
      transform: translateX(5px);
    }
  }

  .submit {
    min-height: 60px;
    margin-top: 4px;
    border: 3px solid var(--edge);
    border-radius: 16px;
    background: var(--lemon);
    color: #111;
    font-size: 22px;
    font-weight: 800;
    box-shadow: 5px 5px 0 var(--edge);
    transition:
      transform 0.08s,
      box-shadow 0.08s;
    touch-action: manipulation;
  }

  .submit:active:not(:disabled) {
    transform: translate(5px, 5px);
    box-shadow: 0 0 0 var(--edge);
  }

  .submit:disabled {
    background: var(--mint);
    cursor: progress;
    transform: translate(5px, 5px);
    box-shadow: 0 0 0 var(--edge);
  }

  @media (max-width: 760px) {
    .l-brand,
    .l-form,
    .l-note,
    .l-sticker {
      grid-column: span 6;
      grid-row: auto;
    }

    .l-brand {
      min-height: 230px;
    }
  }

  @media (max-width: 560px) {
    .login {
      padding: 12px 17px 24px 12px;
    }

    .lgrid {
      gap: 14px;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .submit,
    .blobs i {
      transition: none;
    }

    .error {
      animation: none;
    }
  }
</style>
