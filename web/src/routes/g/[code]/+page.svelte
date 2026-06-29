<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { hasSession } from '$lib/stores/auth';
  import { joinGroup } from '$lib/stores/groups';

  let err = $state<string | null>(null);

  onMount(async () => {
    const code = page.params.code ?? '';
    if (!code) { err = 'Lien invalide.'; return; }
    if (!hasSession()) {
      await goto(`/login?next=${encodeURIComponent(`/g/${code}`)}`);
      return;
    }
    try {
      const gid = await joinGroup(code);
      await goto(`/app?tab=messages&group=${gid}`);
    } catch {
      err = 'Invitation invalide, expirée, ou groupe complet.';
    }
  });
</script>

<div class="grid min-h-screen place-items-center px-4">
  <div class="max-w-sm text-center">
    {#if err}
      <h1 class="text-subtitle font-bold text-content">Impossible de rejoindre</h1>
      <p class="mt-2 text-body text-danger">{err}</p>
      <a href="/app" class="mt-6 inline-block font-medium text-accent hover:text-content">Aller à l'app</a>
    {:else}
      <p class="flex items-center justify-center gap-2 text-body text-muted">
        <span class="size-4 animate-spin rounded-full border-2 border-muted/30 border-t-accent"></span>
        Connexion au groupe…
      </p>
    {/if}
  </div>
</div>
