<script lang="ts">
  import { Inbox, Bookmark } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { inboxUnread } from '$lib/stores/inbox';

  const path = $derived(page.url.pathname);
</script>

<div class="flex items-center gap-1">
  <button
    type="button"
    title="Boîte de réception"
    aria-label="Boîte de réception"
    onclick={() => goto('/app/me/inbox')}
    class="relative grid size-8 place-items-center rounded-md transition-colors duration-150 hover:bg-elevated hover:text-content
           {path.startsWith('/app/me/inbox') ? 'bg-elevated text-content' : 'text-muted'}"
  >
    <Inbox size={18} />
    {#if $inboxUnread > 0}
      <span
        class="absolute -right-0.5 -top-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-danger px-1 text-[0.625rem] font-semibold text-white ring-2 ring-base"
        >{$inboxUnread > 9 ? '9+' : $inboxUnread}</span
      >
    {/if}
  </button>
  <button
    type="button"
    title="Messages enregistrés"
    aria-label="Messages enregistrés"
    onclick={() => goto('/app/me/saved')}
    class="grid size-8 place-items-center rounded-md transition-colors duration-150 hover:bg-elevated hover:text-content
           {path.startsWith('/app/me/saved') ? 'bg-elevated text-content' : 'text-muted'}"
  >
    <Bookmark size={18} />
  </button>
</div>
