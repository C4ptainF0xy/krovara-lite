<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api, ApiError } from '$lib/api';
  import { hasSession } from '$lib/stores/auth';
  import { loadSpaces } from '$lib/stores/spaces';
  import { applyDefaultNotifOnJoin } from '$lib/stores/inbox';

  let busy = $state(true);
  let err = $state<string | null>(null);

  onMount(async () => {
    const code = page.params.code;
    if (!code) {
      err = 'missing code';
      busy = false;
      return;
    }
    if (!hasSession()) {
      await goto(`/login?return=${encodeURIComponent('/join/' + code)}`);
      return;
    }
    try {
      const r = await api<{ space_id: string }>(`/api/invites/${code}/accept`, {
        method: 'POST'
      });
      await applyDefaultNotifOnJoin(r.space_id);
      await loadSpaces();
      await goto(`/app/spaces/${r.space_id}`);
    } catch (e) {
      err = e instanceof ApiError ? e.message : 'invite failed';
      busy = false;
    }
  });
</script>

<div class="grid min-h-screen place-items-center px-4">
  <div class="max-w-md animate-fade-in text-center">
    <img
      src="/krovara.png"
      alt=""
      width="56"
      height="56"
      class="mx-auto mb-5 size-14 rounded-2xl shadow-lg shadow-primary/15"
    />
    {#if busy}
      <h1 class="text-subtitle font-bold">Connexion à l'espace…</h1>
      <p class="mt-2 flex items-center justify-center gap-2 text-body text-muted">
        <span class="size-4 animate-spin rounded-full border-2 border-muted/30 border-t-accent"></span>
        Acceptation de l'invitation
      </p>
    {:else if err}
      <h1 class="text-subtitle font-bold">Invitation invalide</h1>
      <p class="mt-3 text-body text-danger">{err}</p>
      <a
        href="/app"
        class="mt-6 inline-block font-medium text-accent hover:text-content"
      >Retour à l'application</a>
    {/if}
  </div>
</div>
