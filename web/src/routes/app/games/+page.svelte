<script lang="ts">
  import { onMount } from 'svelte';
  import { Gamepad2, Plus, Check, X, Search, Image as ImageIcon } from '@lucide/svelte';
  import { api, ApiError, authedObjectURL } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import {
    listGames,
    submitGame,
    listPendingGames,
    reviewGame,
    type Game
  } from '$lib/stores/games';
  import ImageCropper from '$lib/components/ImageCropper.svelte';
  import { Button, Input } from '$lib/ui';

  const isAdmin = $derived(!!$auth.user?.is_admin);

  let tab = $state<'catalogue' | 'review'>('catalogue');
  let games = $state<Game[]>([]);
  let pending = $state<Game[]>([]);
  let loading = $state(true);
  let query = $state('');

  let submitOpen = $state(false);
  let newName = $state('');
  let submitBusy = $state(false);
  let submitErr = $state<string | null>(null);

  let coverKey = $state<string | null>(null);
  let coverUrl = $state<string | null>(null);
  let cropFile = $state<File | null>(null);
  let cropOpen = $state(false);
  let cropBusy = $state(false);

  function onCoverPick(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    cropFile = file;
    cropOpen = true;
  }
  async function onCoverCropped(blob: Blob) {
    cropBusy = true;
    try {
      const form = new FormData();
      form.append('file', new File([blob], 'cover.png', { type: 'image/png' }));
      const dto = await api<{ id: string }>('/api/files?kind=banner', { method: 'POST', body: form });
      coverKey = dto.id;
      if (coverUrl) URL.revokeObjectURL(coverUrl);
      coverUrl = URL.createObjectURL(blob);
      cropOpen = false;
    } catch {
      submitErr = 'Échec de l’upload de la jaquette';
    } finally {
      cropBusy = false;
    }
  }

  let coverUrls = $state<Record<string, string>>({});
  $effect(() => {
    for (const g of games) {
      if (g.cover_key && !coverUrls[g.id]) {
        void authedObjectURL(`/api/files/${g.cover_key}`)
          .then((u) => (coverUrls = { ...coverUrls, [g.id]: u }))
          .catch(() => {});
      }
    }
  });

  onMount(async () => {
    try {
      games = await listGames();
      if (isAdmin) pending = await listPendingGames();
    } finally {
      loading = false;
    }
  });

  let searchTimer: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(async () => {
      games = await listGames(query.trim());
    }, 200);
  }

  async function doSubmit(e: Event) {
    e.preventDefault();
    const name = newName.trim();
    if (name.length < 2) return;
    submitBusy = true;
    submitErr = null;
    try {
      await submitGame(name, [], coverKey);
      newName = '';
      coverKey = null;
      if (coverUrl) URL.revokeObjectURL(coverUrl);
      coverUrl = null;
      submitOpen = false;
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) submitErr = 'Ce jeu existe déjà.';
      else submitErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      submitBusy = false;
    }
  }

  async function review(g: Game, approve: boolean) {
    const reason = approve ? '' : (prompt('Raison du rejet ?') ?? '');
    await reviewGame(g.id, approve, reason);
    pending = pending.filter((p) => p.id !== g.id);
    if (approve) games = await listGames(query.trim());
  }

  function initials(n: string) {
    return n.slice(0, 2).toUpperCase();
  }
</script>

<div class="mx-auto max-w-3xl px-6 py-8">
  <div class="flex items-center justify-between">
    <h1 class="flex items-center gap-2 text-title font-bold"><Gamepad2 size={26} /> Jeux</h1>
    <Button type="button" onclick={() => (submitOpen = !submitOpen)}><Plus size={16} /> Proposer un jeu</Button>
  </div>

  {#if submitOpen}
    <form onsubmit={doSubmit} class="mt-4 flex items-end gap-3 rounded-lg border border-border p-3">
      <label class="group relative grid size-16 shrink-0 cursor-pointer place-items-center overflow-hidden rounded-lg border border-border bg-elevated/40">
        {#if coverUrl}
          <img src={coverUrl} alt="" class="size-full object-cover" />
        {:else}
          <ImageIcon size={20} class="text-muted" />
        {/if}
        <span class="absolute inset-0 hidden place-items-center bg-base/60 text-label text-content group-hover:grid">Jaquette</span>
        <input type="file" accept="image/*" class="sr-only" onchange={onCoverPick} />
      </label>
      <div class="flex-1">
        <Input label="Nom du jeu" bind:value={newName} placeholder="Ex. Hollow Knight" maxlength={128} />
        {#if submitErr}<p class="mt-1 text-label text-danger">{submitErr}</p>{/if}
      </div>
      <Button type="submit" loading={submitBusy}>Soumettre</Button>
    </form>
    <p class="mt-1.5 text-label text-muted">Les jeux proposés sont validés par le staff avant d'apparaître au catalogue.</p>
  {/if}

  {#if isAdmin}
    <nav class="mt-6 flex gap-1 border-b border-border">
      {#each [{ k: 'catalogue', l: `Catalogue (${games.length})` }, { k: 'review', l: `À valider (${pending.length})` }] as t (t.k)}
        <button
          type="button"
          onclick={() => (tab = t.k as typeof tab)}
          class="-mb-px border-b-2 px-3 py-2.5 text-body transition-colors duration-150
                 {tab === t.k ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
        >
          {t.l}
        </button>
      {/each}
    </nav>
  {/if}

  <div class="mt-6">
    {#if loading}
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {#each [0, 1, 2, 3] as i (i)}<div class="h-32 animate-pulse rounded-lg bg-elevated/50"></div>{/each}
      </div>
    {:else if isAdmin && tab === 'review'}
      {#if pending.length === 0}
        <p class="text-body text-muted">Aucun jeu en attente de validation.</p>
      {:else}
        <div class="space-y-2">
          {#each pending as g (g.id)}
            <div class="flex items-center gap-3 rounded-lg border border-border p-3">
              <span class="grid size-10 shrink-0 place-items-center rounded-md bg-elevated text-label font-semibold text-muted">{initials(g.name)}</span>
              <span class="min-w-0 flex-1 truncate text-body text-content">{g.name}</span>
              <button type="button" title="Approuver" onclick={() => review(g, true)} class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors hover:border-success/50 hover:text-success"><Check size={16} /></button>
              <button type="button" title="Rejeter" onclick={() => review(g, false)} class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors hover:border-danger/50 hover:text-danger"><X size={16} /></button>
            </div>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="relative mb-4">
        <Search size={16} class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
        <input
          bind:value={query}
          oninput={onSearch}
          placeholder="Rechercher un jeu…"
          class="h-10 w-full rounded border border-border bg-base/50 pl-9 pr-3 text-body text-content outline-none focus:border-primary"
        />
      </div>
      {#if games.length === 0}
        <div class="grid place-items-center gap-3 py-16 text-center">
          <div class="grid size-14 place-items-center rounded-full bg-elevated text-muted"><Gamepad2 size={26} /></div>
          <p class="text-body text-muted">Aucun jeu au catalogue. Propose-en un !</p>
        </div>
      {:else}
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {#each games as g (g.id)}
            <div class="flex flex-col items-center gap-2 rounded-lg border border-border p-4 text-center transition-colors duration-150 hover:bg-surface/50">
              {#if g.cover_key && coverUrls[g.id]}
                <img src={coverUrls[g.id]} alt="" class="size-14 rounded-lg object-cover" />
              {:else}
                <span class="grid size-14 place-items-center rounded-lg bg-elevated text-subtitle font-semibold text-muted">{initials(g.name)}</span>
              {/if}
              <span class="truncate text-body text-content">{g.name}</span>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</div>

<ImageCropper
  open={cropOpen}
  file={cropFile}
  busy={cropBusy}
  title="Recadrer la jaquette"
  aspect={1}
  shape="rounded"
  onclose={() => (cropOpen = false)}
  oncropped={onCoverCropped}
/>
