<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { Inbox, AtSign, Megaphone, CheckCheck, CornerUpLeft, UserCheck, UserX } from '@lucide/svelte';
  import {
    inboxItems,
    inboxUnread,
    loadInbox,
    markRead,
    markAllRead,
    type InboxItem
  } from '$lib/stores/inbox';
  import { memberNames } from '$lib/stores/members';

  let { heading = true }: { heading?: boolean } = $props();

  let loading = $state(true);

  onMount(async () => {
    try {
      await loadInbox();
    } finally {
      loading = false;
    }
  });

  async function open(it: InboxItem) {
    if (!it.read) await markRead(it.id);
    if (it.space_id && it.channel_id) {
      await goto(`/app/spaces/${it.space_id}/channels/${it.channel_id}?m=${encodeURIComponent(it.archive_id)}`);
    } else if (it.kind === 'join_approved' && it.space_id) {
      await goto(`/app/spaces/${it.space_id}`);
    }
  }

  function authorName(it: InboxItem): string {
    return (it.author_id && $memberNames[it.author_id]) || 'Quelqu’un';
  }

  function fmt(ts: string): string {
    const d = new Date(ts);
    return d.toLocaleString('fr-FR', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
  }
</script>

<div class="mx-auto max-w-2xl px-6 py-8">
  <div class="flex items-center justify-between">
    {#if heading}
      <h1 class="flex items-center gap-2 text-title font-bold">
        <Inbox size={26} /> Boîte de réception
        {#if $inboxUnread > 0}
          <span class="grid h-6 min-w-6 place-items-center rounded-full bg-danger px-2 text-label font-semibold text-white">
            {$inboxUnread}
          </span>
        {/if}
      </h1>
    {:else}
      <span></span>
    {/if}
    {#if $inboxItems.some((i) => !i.read)}
      <button
        type="button"
        onclick={markAllRead}
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-label text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        <CheckCheck size={16} /> Tout marquer lu
      </button>
    {/if}
  </div>

  <div class="mt-6 space-y-1">
    {#if loading}
      {#each [0, 1, 2] as i (i)}
        <div class="h-16 animate-pulse rounded-lg bg-elevated/50"></div>
      {/each}
    {:else if $inboxItems.length === 0}
      <div class="grid place-items-center gap-3 py-16 text-center">
        <div class="grid size-14 place-items-center rounded-full bg-elevated text-muted"><Inbox size={26} /></div>
        <p class="text-body text-muted">Aucune mention pour l'instant.</p>
      </div>
    {:else}
      {#each $inboxItems as it (it.id)}
        <button
          type="button"
          onclick={() => open(it)}
          class="flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors duration-150
                 {it.read ? 'border-border hover:bg-surface/50' : 'border-brand/40 bg-brand/5 hover:bg-brand/10'}"
        >
          <span class="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full {it.kind === 'everyone' ? 'bg-warning/15 text-warning' : it.kind === 'reply' || it.kind === 'join_approved' ? 'bg-success/15 text-success' : it.kind === 'join_rejected' ? 'bg-danger/15 text-danger' : 'bg-primary/15 text-accent'}">
            {#if it.kind === 'everyone'}<Megaphone size={16} />{:else if it.kind === 'reply'}<CornerUpLeft size={16} />{:else if it.kind === 'join_approved'}<UserCheck size={16} />{:else if it.kind === 'join_rejected'}<UserX size={16} />{:else}<AtSign size={16} />{/if}
          </span>
          <span class="min-w-0 flex-1">
            <span class="flex items-baseline gap-2">
              <span class="truncate text-body font-medium text-content">{authorName(it)}</span>
              <span class="shrink-0 text-label text-muted">{fmt(it.created_at)}</span>
            </span>
            <span class="mt-0.5 line-clamp-2 block text-label text-muted">{it.preview || '(sans aperçu)'}</span>
          </span>
          {#if !it.read}
            <span class="mt-1.5 size-2 shrink-0 rounded-full bg-brand" aria-label="non lu"></span>
          {/if}
        </button>
      {/each}
    {/if}
  </div>
</div>
