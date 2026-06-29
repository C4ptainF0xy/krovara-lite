<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { api, ApiError } from '$lib/api';
  import { Search } from '@lucide/svelte';
  import { spaces } from '$lib/stores/spaces';

  type Hit = {
    id: string;
    channel_id: string;
    space_id: string;
    author_id: string;
    content: string;
    created_at: number;
  };
  type SearchResp = { hits: Hit[]; total: number; took: number };

  let q = $state(page.url.searchParams.get('q') ?? '');
  let hits = $state<Hit[]>([]);
  let total = $state(0);
  let took = $state(0);
  let loading = $state(false);
  let err = $state<string | null>(null);

  async function run() {
    err = null;
    if (!q.trim()) {
      hits = [];
      total = 0;
      return;
    }
    loading = true;
    try {
      const data = await api<SearchResp>(`/api/search?q=${encodeURIComponent(q.trim())}`);
      hits = data.hits ?? [];
      total = data.total ?? 0;
      took = data.took ?? 0;
    } catch (e) {
      err = e instanceof ApiError ? e.message : 'Recherche impossible';
      hits = [];
    } finally {
      loading = false;
    }
  }

  function submit(e: Event) {
    e.preventDefault();
    void goto(`/app/search?q=${encodeURIComponent(q.trim())}`, {
      replaceState: true,
      noScroll: true,
      keepFocus: true
    });
    void run();
  }

  function spaceName(id: string): string {
    return $spaces.data.find((s) => s.id === id)?.name ?? 'Espace';
  }
  function open(h: Hit) {
    void goto(
      `/app/spaces/${h.space_id}/channels/${h.channel_id}?m=${encodeURIComponent(h.id)}`
    );
  }
  function fmt(ts: number): string {
    return new Date(ts * 1000).toLocaleString();
  }

  onMount(run);
</script>

<div class="mx-auto flex h-full max-w-2xl flex-col px-6 py-6">
  <h1 class="flex items-center gap-2 text-title font-bold"><Search size={24} /> Recherche globale</h1>
  <p class="mt-1 text-body text-muted">Cherche dans les messages de tous tes espaces.</p>

  <form onsubmit={submit} class="mt-5">
    <div class="relative">
      <Search size={16} class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
      <!-- svelte-ignore a11y_autofocus -->
      <input
        bind:value={q}
        type="search"
        autofocus
        placeholder="Rechercher partout… (from:, before:, after: supportés)"
        class="h-11 w-full rounded-lg border border-border bg-base/50 pl-9 pr-3 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
      />
    </div>
  </form>

  <div class="mt-5 flex-1 overflow-y-auto">
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
      <p class="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-label text-danger">{err}</p>
    {:else if hits.length === 0 && q.trim()}
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <Search size={24} class="text-muted" />
        <p class="mt-3 text-body text-muted">Aucun résultat pour « {q} ».</p>
      </div>
    {:else if hits.length === 0}
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <Search size={24} class="text-muted" />
        <p class="mt-3 text-body text-muted">Tape pour chercher dans tous tes espaces.</p>
      </div>
    {:else}
      <p class="mb-3 text-label text-muted">{total} résultat{total === 1 ? '' : 's'} · {took} ms</p>
      <ul class="space-y-2">
        {#each hits as h (h.id)}
          <li>
            <button
              type="button"
              onclick={() => open(h)}
              class="block w-full rounded-lg border border-border bg-surface/60 p-3 text-left transition-colors duration-150 hover:bg-elevated"
            >
              <p class="text-body text-content">{h.content}</p>
              <p class="mt-1 text-label text-muted">{spaceName(h.space_id)} · {fmt(h.created_at)}</p>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
