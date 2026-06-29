<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { Check, AlertCircle } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { Button } from '$lib/ui';

  let status = $state<'pending' | 'ok' | 'error'>('pending');
  let email = $state('');
  let err = $state('');

  onMount(async () => {
    const token = page.url.searchParams.get('token');
    if (!token) {
      status = 'error';
      err = 'Lien incomplet.';
      return;
    }
    try {
      const res = await api<{ email: string }>('/api/account/email/confirm', {
        method: 'POST',
        body: { token }
      });
      email = res.email;
      status = 'ok';
    } catch (e) {
      status = 'error';
      err = e instanceof Error ? e.message : 'Lien invalide ou expiré.';
    }
  });
</script>

<div class="grid min-h-screen place-items-center bg-base px-4">
  <div class="w-full max-w-sm rounded-xl border border-border bg-surface p-8 text-center">
    {#if status === 'pending'}
      <div class="mx-auto size-8 animate-spin rounded-full border-2 border-border border-t-accent"></div>
      <p class="mt-4 text-body text-muted">Confirmation en cours…</p>
    {:else if status === 'ok'}
      <div class="mx-auto grid size-12 place-items-center rounded-full bg-success/15">
        <Check size={24} class="text-success" />
      </div>
      <h1 class="mt-4 text-subtitle font-bold text-content">Adresse confirmée</h1>
      <p class="mt-1 text-body text-muted">
        Ton email est maintenant <span class="font-medium text-content">{email}</span>.
      </p>
      <div class="mt-6">
        <Button type="button" onclick={() => (window.location.href = '/app')}>Aller à l'app</Button>
      </div>
    {:else}
      <div class="mx-auto grid size-12 place-items-center rounded-full bg-danger/15">
        <AlertCircle size={24} class="text-danger" />
      </div>
      <h1 class="mt-4 text-subtitle font-bold text-content">Confirmation impossible</h1>
      <p class="mt-1 text-body text-muted">{err}</p>
      <div class="mt-6">
        <Button type="button" variant="secondary" onclick={() => (window.location.href = '/app/settings/account')}>
          Retour aux réglages
        </Button>
      </div>
    {/if}
  </div>
</div>
