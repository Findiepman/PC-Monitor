<script lang="ts">
  import { onMount } from 'svelte';
  import { api, SIGNED_OUT } from './lib/api';
  import Login from './components/Login.svelte';
  import Console from './components/Console.svelte';

  let phase = $state<'checking' | 'out' | 'in'>('checking');

  onMount(() => {
    api.session().then(
      () => (phase = 'in'),
      () => (phase = 'out'),
    );
    const out = () => (phase = 'out');
    window.addEventListener(SIGNED_OUT, out);
    return () => window.removeEventListener(SIGNED_OUT, out);
  });
</script>

{#if phase === 'in'}
  <Console onsignout={() => (phase = 'out')} />
{:else if phase === 'out'}
  <Login onsignin={() => (phase = 'in')} />
{/if}
