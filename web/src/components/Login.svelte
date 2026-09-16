<script lang="ts">
  import { api } from '../lib/api';

  let { onsignin }: { onsignin: () => void } = $props();

  let username = $state('');
  let password = $state('');
  let code = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.login(username.trim(), password, code.replace(/\s/g, ''));
      onsignin();
    } catch (err) {
      error = (err as Error).message;
      code = '';
      busy = false;
    }
  }
</script>

<main class="login">
  <form onsubmit={submit} aria-describedby={error ? 'login-error' : undefined}>
    <svg class="flat" viewBox="0 0 320 24" preserveAspectRatio="none" aria-hidden="true">
      <path d="M0 12 H196 l6 -9 l8 18 l6 -9 H320" />
    </svg>
    <h1>Sign in to dashd</h1>

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

    {#if error}
      <p id="login-error" class="error" role="alert">{error}</p>
    {/if}

    <button type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
</main>

<style>
  .login {
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 24px 16px;
  }

  form {
    width: 100%;
    max-width: 360px;
    display: grid;
    gap: 16px;
    padding: 28px 24px 24px;
    background: var(--plate);
    border: 1px solid var(--rule);
    border-radius: 8px;
  }

  .flat {
    width: 100%;
    height: 24px;
    margin-bottom: 4px;
  }

  .flat path {
    fill: none;
    stroke: var(--trace);
    stroke-width: 1.5;
    vector-effect: non-scaling-stroke;
    stroke-linejoin: round;
  }

  h1 {
    margin: 0 0 4px;
    font-size: var(--fs-l);
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  label {
    display: grid;
    gap: 6px;
    font-size: var(--fs-s);
    color: var(--dim);
  }

  input {
    height: 40px;
    padding: 0 12px;
    border: 1px solid var(--rule);
    border-radius: 5px;
    background: var(--bay);
    color: var(--ink);
    font-size: var(--fs-m);
  }

  input:focus-visible {
    outline: none;
    border-color: var(--trace);
    box-shadow: 0 0 0 1px var(--trace);
  }

  .code {
    font-size: var(--fs-l);
    letter-spacing: 0.3em;
  }

  .code::placeholder {
    color: var(--rule);
  }

  .error {
    margin: 0;
    padding: 8px 10px;
    border-left: 2px solid var(--fail);
    background: var(--fail-soft);
    font-size: var(--fs-s);
  }

  button {
    height: 42px;
    margin-top: 4px;
    border: 0;
    border-radius: 5px;
    background: var(--trace);
    color: var(--bay);
    font-weight: 600;
  }

  button:disabled {
    opacity: 0.7;
    cursor: progress;
  }
</style>
