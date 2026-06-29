<script lang="ts">
  import { MessagesSquare } from '@lucide/svelte';
  import { page } from '$app/state';
  import { channelsBySpace, spaces } from '$lib/stores/spaces';
  import QuickAccess from '$lib/components/QuickAccess.svelte';

  const id = $derived(page.params.id ?? '');
  const space = $derived($spaces.data.find((s) => s.id === id));
  const channels = $derived(id ? ($channelsBySpace[id]?.data ?? []) : []);
</script>

<div class="flex h-full flex-col">
  <header class="flex items-center gap-3 border-b border-border px-4 py-3">
    <h1 class="min-w-0 truncate text-body font-semibold leading-tight">{space?.name ?? 'Espace'}</h1>
    <div class="ml-auto"><QuickAccess /></div>
  </header>
  <div class="grid flex-1 place-items-center p-6 text-center">
    <div class="max-w-sm animate-fade-in">
    <div class="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-surface text-muted">
      <MessagesSquare size={26} />
    </div>
    <h1 class="text-subtitle font-bold">{space?.name ?? 'Espace'}</h1>
    {#if channels.length === 0}
      <p class="mt-2 text-body text-muted">
        Aucun salon pour l'instant. Crée-en un avec le bouton
        <span class="text-success">+</span> en haut.
      </p>
    {:else}
      <p class="mt-2 text-body text-muted">Choisis un salon à gauche pour discuter.</p>
    {/if}
    </div>
  </div>
</div>
