<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, X, Ban, UserMinus, Users } from '@lucide/svelte';
  import { ApiError, authedObjectURL } from '$lib/api';
  import {
    friends,
    incoming,
    outgoing,
    blocked,
    loadFriends,
    loadRequests,
    loadBlocks,
    sendRequest,
    acceptRequest,
    removeFriendship,
    blockHandle,
    unblock,
    setWhoCanAdd,
    type WhoCanAdd
  } from '$lib/stores/friends';
  import { Button, Input } from '$lib/ui';

  let { heading = true }: { heading?: boolean } = $props();

  let tab = $state<'all' | 'pending' | 'blocked' | 'add'>('all');
  let loading = $state(true);

  let handle = $state('');
  let addBusy = $state(false);
  let addErr = $state<string | null>(null);
  let addOk = $state<string | null>(null);

  let whoCanAdd = $state<WhoCanAdd>('everyone');

  onMount(async () => {
    try {
      await Promise.all([loadFriends(), loadRequests(), loadBlocks()]);
    } finally {
      loading = false;
    }
  });

  async function submitAdd(e: Event) {
    e.preventDefault();
    const h = handle.trim();
    if (!h) return;
    addBusy = true;
    addErr = null;
    addOk = null;
    try {
      await sendRequest(h);
      addOk = `Demande envoyée à ${h}.`;
      handle = '';
      await loadRequests();
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) addErr = 'Aucun utilisateur avec ce handle.';
      else if (err instanceof ApiError && err.status === 409) addErr = 'Demande déjà en cours ou déjà amis.';
      else if (err instanceof ApiError && err.status === 403) addErr = 'Cet utilisateur n’accepte pas ta demande.';
      else addErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      addBusy = false;
    }
  }

  async function accept(id: string) {
    await acceptRequest(id);
    await Promise.all([loadRequests(), loadFriends()]);
  }
  async function decline(id: string) {
    await removeFriendship(id);
    await loadRequests();
  }
  async function unfriend(id: string) {
    if (!confirm('Retirer cet ami ?')) return;
    await removeFriendship(id);
    await loadFriends();
  }
  async function doBlock(handleName: string) {
    if (!confirm(`Bloquer ${handleName} ?`)) return;
    await blockHandle(handleName);
    await Promise.all([loadFriends(), loadRequests(), loadBlocks()]);
  }
  async function doUnblock(userId: string) {
    await unblock(userId);
    await loadBlocks();
  }
  async function changeWhoCanAdd(who: WhoCanAdd) {
    whoCanAdd = who;
    await setWhoCanAdd(who);
  }

  function initials(name: string) {
    return name.slice(0, 2).toUpperCase();
  }

  const pendingCount = $derived($incoming.length + $outgoing.length);

  const sortedFriends = $derived(
    [...$friends].sort(
      (a, b) => (b.since ? Date.parse(b.since) : 0) - (a.since ? Date.parse(a.since) : 0)
    )
  );
</script>

{#snippet avatar(key: string | null, who: string)}
  {#if key}
    {#await authedObjectURL(`/api/files/${key}`) then src}
      <img {src} alt={who} class="size-9 shrink-0 rounded-full object-cover" />
    {:catch}
      <span class="grid size-9 shrink-0 place-items-center rounded-full bg-elevated text-label font-medium text-muted">{initials(who)}</span>
    {/await}
  {:else}
    <span class="grid size-9 shrink-0 place-items-center rounded-full bg-elevated text-label font-medium text-muted">{initials(who)}</span>
  {/if}
{/snippet}

<div class="mx-auto max-w-3xl px-6 py-8">
  {#if heading}
    <h1 class="flex items-center gap-2 text-title font-bold">
      <Users size={26} /> Amis
    </h1>
  {/if}

  <nav class="flex overflow-x-auto no-scrollbar gap-1 border-b border-border {heading ? 'mt-6' : ''}">
    {#each [{ k: 'all', l: `Tous (${$friends.length})` }, { k: 'pending', l: `En attente${pendingCount ? ` (${pendingCount})` : ''}` }, { k: 'blocked', l: `Bloqués (${$blocked.length})` }, { k: 'add', l: 'Ajouter' }] as t (t.k)}
      <button
        type="button"
        onclick={() => (tab = t.k as typeof tab)}
        class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-3 py-2.5 text-body transition-colors duration-150
               {tab === t.k ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
      >
        {t.l}
      </button>
    {/each}
  </nav>

  <div class="mt-6">
    {#if loading}
      <p class="text-body text-muted">Chargement…</p>
    {:else if tab === 'add'}
      <form onsubmit={submitAdd} class="max-w-md space-y-3">
        <p class="text-body text-muted">
          Ajoute un ami par son pseudo — la casse n'a pas d'importance.
        </p>
        <Input label="Handle" bind:value={handle} placeholder="ex. sloth" />
        {#if addErr}<p class="text-label text-danger">{addErr}</p>{/if}
        {#if addOk}<p class="text-label text-success">{addOk}</p>{/if}
        <Button type="submit" loading={addBusy}>Envoyer la demande</Button>

        <div class="mt-8 border-t border-border pt-5">
          <h2 class="text-body font-semibold text-content">Qui peut m'ajouter ?</h2>
          <div class="mt-3 inline-flex flex-wrap gap-1 rounded-lg border border-border p-1">
            {#each [{ v: 'everyone', l: 'Tout le monde' }, { v: 'friends_of_friends', l: 'Amis d’amis' }, { v: 'nobody', l: 'Personne' }] as o (o.v)}
              <button
                type="button"
                onclick={() => changeWhoCanAdd(o.v as WhoCanAdd)}
                class="rounded px-3 py-1.5 text-label font-medium transition-colors duration-150
                       {whoCanAdd === o.v ? 'bg-primary text-white' : 'text-muted hover:text-content'}"
              >
                {o.l}
              </button>
            {/each}
          </div>
        </div>
      </form>
    {:else if tab === 'pending'}
      {#if pendingCount === 0}
        <p class="text-body text-muted">Aucune demande en attente.</p>
      {:else}
        {#if $incoming.length}
          <h2 class="mb-2 text-label font-semibold uppercase tracking-wide text-muted">Reçues</h2>
          <div class="mb-6 space-y-1">
            {#each $incoming as r (r.id)}
              <div class="flex items-center gap-3 rounded-lg border border-border p-3">
                {@render avatar(r.avatar_key, r.username)}
                <span class="min-w-0 flex-1 truncate text-body text-content">{r.username}</span>
                <button type="button" title="Accepter" onclick={() => accept(r.id)} class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors hover:border-success/50 hover:text-success"><Check size={16} /></button>
                <button type="button" title="Refuser" onclick={() => decline(r.id)} class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors hover:border-danger/50 hover:text-danger"><X size={16} /></button>
              </div>
            {/each}
          </div>
        {/if}
        {#if $outgoing.length}
          <h2 class="mb-2 text-label font-semibold uppercase tracking-wide text-muted">Envoyées</h2>
          <div class="space-y-1">
            {#each $outgoing as r (r.id)}
              <div class="flex items-center gap-3 rounded-lg border border-border p-3">
                {@render avatar(r.avatar_key, r.username)}
                <span class="min-w-0 flex-1 truncate text-body text-content">{r.username}</span>
                <span class="text-label text-muted">En attente</span>
                <button type="button" title="Annuler" onclick={() => decline(r.id)} class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors hover:border-danger/50 hover:text-danger"><X size={16} /></button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    {:else if tab === 'blocked'}
      {#if $blocked.length === 0}
        <p class="text-body text-muted">Aucun utilisateur bloqué.</p>
      {:else}
        <div class="space-y-1">
          {#each $blocked as b (b.id)}
            <div class="flex items-center gap-3 rounded-lg border border-border p-3">
              {@render avatar(b.avatar_key, b.username)}
              <span class="min-w-0 flex-1 truncate text-body text-content">{b.username}</span>
              <Button type="button" variant="ghost" onclick={() => doUnblock(b.id)}>Débloquer</Button>
            </div>
          {/each}
        </div>
      {/if}
    {:else}
      {#if $friends.length === 0}
        <p class="text-body text-muted">Pas encore d'amis. Ajoute quelqu'un par son handle.</p>
      {:else}
        <div class="space-y-1">
          {#each sortedFriends as f (f.id)}
            <div class="group flex items-center gap-3 rounded-lg border border-border p-3">
              {@render avatar(f.avatar_key, f.username)}
              <span class="min-w-0 flex-1 truncate text-body text-content">{f.username}</span>
              <button type="button" title="Bloquer" onclick={() => doBlock(f.username)} class="grid size-8 place-items-center rounded-md text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"><Ban size={16} /></button>
              <button type="button" title="Retirer" onclick={() => unfriend(f.id)} class="grid size-8 place-items-center rounded-md text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"><UserMinus size={16} /></button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</div>
