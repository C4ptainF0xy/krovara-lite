<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { Compass, Search, Users, ArrowRight } from '@lucide/svelte';
  import { explore, openJoin, CATEGORIES, type Listing } from '$lib/stores/discovery';
  import { ApiError } from '$lib/api';
  import { loadSpaces } from '$lib/stores/spaces';
  import { applyDefaultNotifOnJoin } from '$lib/stores/inbox';

  const CAT_LABELS: Record<string, string> = {
    gaming: 'Gaming',
    tech: 'Tech',
    art: 'Art',
    music: 'Musique',
    education: 'Éducation',
    community: 'Communauté',
    other: 'Autre'
  };

  let listings = $state<Listing[]>([]);
  let loading = $state(true);
  let category = $state('');
  let query = $state('');

  onMount(load);
  async function load() {
    loading = true;
    try {
      listings = await explore(category, query.trim());
    } finally {
      loading = false;
    }
  }

  let timer: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(timer);
    timer = setTimeout(load, 200);
  }
  function pickCategory(c: string) {
    category = category === c ? '' : c;
    load();
  }
  function initials(n: string) {
    return n.slice(0, 2).toUpperCase();
  }

  let joining = $state<string | null>(null);
  let joinErr = $state<{ id: string; msg: string } | null>(null);
  async function join(l: Listing) {
    if (joining) return;
    joining = l.space_id;
    joinErr = null;
    try {
      await openJoin(l.space_id);
      await applyDefaultNotifOnJoin(l.space_id);
      await loadSpaces();
      await goto(`/app/spaces/${l.space_id}`);
    } catch (err) {
      if (err instanceof ApiError && err.status === 409 && (err.body as { requires_form?: boolean })?.requires_form) {
        await goto(`/app/spaces/${l.space_id}/join`);
        return;
      }
      joinErr = { id: l.space_id, msg: err instanceof ApiError ? err.message : 'Impossible de rejoindre' };
    } finally {
      joining = null;
    }
  }
</script>

<div class="mx-auto max-w-4xl px-6 py-8">
  <h1 class="flex items-center gap-2 text-title font-bold"><Compass size={26} /> Explorer</h1>
  <p class="mt-1 text-body text-muted">Découvre des espaces publics de la communauté Krovara.</p>

  <div class="relative mt-6">
    <Search size={16} class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
    <input
      bind:value={query}
      oninput={onSearch}
      placeholder="Rechercher un espace…"
      class="h-10 w-full rounded border border-border bg-base/50 pl-9 pr-3 text-body text-content outline-none focus:border-primary"
    />
  </div>

  <div class="mt-3 flex flex-wrap gap-1.5">
    {#each CATEGORIES as c (c)}
      <button
        type="button"
        onclick={() => pickCategory(c)}
        aria-pressed={category === c}
        class="rounded-full border px-3 py-1 text-label transition-colors duration-150
               {category === c ? 'border-primary bg-primary/10 text-content' : 'border-border text-muted hover:border-border-strong'}"
      >
        {CAT_LABELS[c]}
      </button>
    {/each}
  </div>

  <div class="mt-6">
    {#if loading}
      <div class="grid gap-3 sm:grid-cols-2">
        {#each [0, 1, 2, 3] as i (i)}<div class="h-28 animate-pulse rounded-lg bg-elevated/50"></div>{/each}
      </div>
    {:else if listings.length === 0}
      <div class="grid place-items-center gap-3 py-16 text-center">
        <div class="grid size-14 place-items-center rounded-full bg-elevated text-muted"><Compass size={26} /></div>
        <p class="text-body text-muted">Aucun espace public pour l'instant.</p>
      </div>
    {:else}
      <div class="grid gap-3 sm:grid-cols-2">
        {#each listings as l (l.space_id)}
          <div class="overflow-hidden rounded-lg border border-border transition-colors duration-150 hover:bg-surface/50">
            <div class="h-16 bg-gradient-to-r from-primary/30 to-brand/20"></div>
            <div class="p-4">
              <div class="-mt-9 mb-2 flex items-end justify-between">
                <span class="grid size-12 place-items-center rounded-xl bg-elevated text-body font-semibold text-muted ring-4 ring-surface">
                  {initials(l.name)}
                </span>
                <span class="flex items-center gap-1 text-label text-muted"><Users size={13} /> {l.member_count}</span>
              </div>
              <p class="truncate text-body font-semibold text-content">{l.name}</p>
              {#if l.description}
                <p class="mt-0.5 line-clamp-2 text-label text-muted">{l.description}</p>
              {/if}
              <div class="mt-2 flex flex-wrap items-center gap-1">
                <span class="rounded-full bg-elevated px-2 py-0.5 text-[0.625rem] font-medium text-muted">{CAT_LABELS[l.category]}</span>
                {#each (l.tags ?? []).slice(0, 3) as tag (tag)}
                  <span class="rounded-full border border-border px-2 py-0.5 text-[0.625rem] text-muted">{tag}</span>
                {/each}
              </div>
              <button
                type="button"
                onclick={() => join(l)}
                disabled={joining === l.space_id}
                class="mt-3 flex w-full items-center justify-center gap-1.5 rounded border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content disabled:opacity-50"
              >
                {joining === l.space_id ? 'Connexion…' : 'Rejoindre'} <ArrowRight size={14} />
              </button>
              {#if joinErr?.id === l.space_id}
                <p class="mt-1.5 text-center text-[0.625rem] text-danger">{joinErr.msg}</p>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
