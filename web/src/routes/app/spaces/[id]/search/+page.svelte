<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { api, ApiError } from '$lib/api';
  import { Search, Bookmark, X } from '@lucide/svelte';
  import {
    listSavedSearches,
    createSavedSearch,
    deleteSavedSearch,
    type SavedSearch
  } from '$lib/stores/savedSearches';

  type Hit = {
    id: string;
    channel_id: string;
    space_id: string;
    author_id: string;
    content: string;
    created_at: number;
  };

  type SearchResp = { hits: Hit[]; total: number; took: number };

  const spaceId = $derived(page.params.id ?? '');
  let q = $state(page.url.searchParams.get('q') ?? '');
  let hits = $state<Hit[]>([]);
  let total = $state(0);
  let took = $state(0);
  let loading = $state(false);
  let err = $state<string | null>(null);

  async function run() {
    err = null;
    if (!q.trim() || !spaceId) {
      hits = [];
      total = 0;
      return;
    }
    loading = true;
    try {
      const data = await api<SearchResp>(
        `/api/spaces/${spaceId}/search?q=${encodeURIComponent(q.trim())}`
      );
      hits = data.hits ?? [];
      total = data.total ?? 0;
      took = data.took ?? 0;
    } catch (e) {
      err = e instanceof ApiError ? e.message : 'search failed';
      hits = [];
    } finally {
      loading = false;
    }
  }

  function submit(e: Event) {
    e.preventDefault();
    void goto(`/app/spaces/${spaceId}/search?q=${encodeURIComponent(q.trim())}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
    void run();
  }

  let saved = $state<SavedSearch[]>([]);
  async function loadSaved() {
    try {
      saved = await listSavedSearches();
    } catch {
    }
  }
  async function saveCurrent() {
    const query = q.trim();
    if (!query) return;
    const name = (typeof prompt === 'function' ? prompt('Nom de la recherche', query.slice(0, 40)) : query)?.trim();
    if (!name) return;
    try {
      const s = await createSavedSearch(name, query, spaceId);
      saved = [s, ...saved];
    } catch {
    }
  }
  async function removeSaved(id: string) {
    await deleteSavedSearch(id);
    saved = saved.filter((s) => s.id !== id);
  }
  function applySaved(s: SavedSearch) {
    q = s.query;
    void goto(`/app/spaces/${spaceId}/search?q=${encodeURIComponent(q.trim())}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
    void run();
  }

  onMount(() => {
    void run();
    void loadSaved();
  });

  function fmt(ts: number): string {
    return new Date(ts * 1000).toLocaleString();
  }
</script>

<div class="flex h-full flex-col">
  <form onsubmit={submit} class="border-b border-border p-3">
    <div class="flex items-center gap-2">
      <div class="relative flex-1">
        <Search
          size={16}
          strokeWidth={2}
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted"
        />
        <input
          bind:value={q}
          type="search"
          placeholder="Rechercher des messages…"
          class="h-10 w-full rounded border border-border bg-base/50 pl-9 pr-3 text-body text-content
                 outline-none transition-[box-shadow,border-color] duration-150 ease-smooth
                 placeholder:text-muted/60
                 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
        />
      </div>
      <button
        type="button"
        onclick={saveCurrent}
        disabled={!q.trim()}
        title="Enregistrer cette recherche"
        aria-label="Enregistrer cette recherche"
        class="grid size-10 shrink-0 place-items-center rounded border border-border text-muted transition-colors duration-150 enabled:hover:border-border-strong enabled:hover:text-content disabled:opacity-50"
      >
        <Bookmark size={16} strokeWidth={2} />
      </button>
    </div>

    {#if saved.length}
      <div class="mt-2 flex flex-wrap gap-1.5">
        {#each saved as s (s.id)}
          <span class="group inline-flex items-center gap-1 rounded-full border border-border bg-surface py-0.5 pl-2.5 pr-1 text-label text-muted">
            <button
              type="button"
              onclick={() => applySaved(s)}
              class="transition-colors duration-150 hover:text-content"
            >
              {s.name}
            </button>
            <button
              type="button"
              onclick={() => removeSaved(s.id)}
              aria-label={`Supprimer ${s.name}`}
              class="grid size-4 place-items-center rounded-full text-muted transition-colors duration-150 hover:bg-elevated hover:text-danger"
            >
              <X size={12} strokeWidth={2.5} />
            </button>
          </span>
        {/each}
      </div>
    {/if}
  </form>

  <div class="flex-1 overflow-y-auto p-4">
    {#if loading}
      <ul class="space-y-2">
        {#each Array(4) as _, i (i)}
          <li class="rounded-lg border border-border bg-surface/60 p-3">
            <div class="h-4 w-3/4 animate-pulse rounded bg-elevated"></div>
            <div class="mt-2 h-3 w-24 animate-pulse rounded bg-elevated"></div>
          </li>
        {/each}
      </ul>
    {:else if err}
      <p class="text-label text-danger">{err}</p>
    {:else if hits.length === 0 && q.trim()}
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <Search size={24} strokeWidth={2} class="text-muted" />
        <p class="mt-3 text-body text-muted">Aucun résultat pour « {q} ».</p>
      </div>
    {:else if hits.length === 0}
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <Search size={24} strokeWidth={2} class="text-muted" />
        <p class="mt-3 text-body text-muted">Cherche dans les messages de cet espace.</p>
      </div>
    {:else}
      <p class="mb-3 text-label text-muted">
        {total} résultat{total === 1 ? '' : 's'} · {took} ms
      </p>
      <ul class="space-y-2">
        {#each hits as h (h.id)}
          <li class="rounded-lg border border-border bg-surface/60 p-3 transition-colors duration-150 ease-smooth hover:bg-elevated">
            <p class="text-body text-content">{h.content}</p>
            <p class="mt-1 text-label text-muted">{fmt(h.created_at)}</p>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
