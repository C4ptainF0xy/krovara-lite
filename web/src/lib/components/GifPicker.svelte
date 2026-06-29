<script lang="ts">
  import { Search } from '@lucide/svelte';
  import { api } from '$lib/api';

  type GifItem = {
    id: number;
    slug: string;
    title: string;
    preview: string;
    url: string;
    width: number;
    height: number;
  };

  type Props = { onpick: (gif: { url: string; name: string }) => void };
  let { onpick }: Props = $props();

  let q = $state('');
  let items = $state<GifItem[]>([]);
  let loading = $state(false);
  let failed = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  const skeletons = [0, 1, 2, 3, 4, 5];

  async function run(term: string) {
    const t = term.trim();
    if (!t) {
      items = [];
      failed = false;
      loading = false;
      return;
    }
    loading = true;
    failed = false;
    try {
      const res = await api<{ items: GifItem[] }>(`/api/gif/search?q=${encodeURIComponent(t)}`);
      items = res.items;
    } catch {
      failed = true;
      items = [];
    } finally {
      loading = false;
    }
  }

  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(() => run(q), 300);
  }

  function pick(g: GifItem) {
    const base = (g.title || g.slug || 'gif').replace(/[^\w-]+/g, '-').slice(0, 40) || 'gif';
    onpick({ url: g.url, name: `${base}.gif` });
  }
</script>

<div class="flex w-72 flex-col gap-2">
  <div class="flex items-center gap-2 rounded-md border border-border bg-base px-2.5 py-1.5">
    <Search size={16} class="shrink-0 text-muted" />
    <!-- svelte-ignore a11y_autofocus -->
    <input
      bind:value={q}
      oninput={onInput}
      autofocus
      placeholder="Rechercher un GIF…"
      class="w-full bg-transparent text-body text-content outline-none placeholder:text-muted/60"
    />
  </div>
  <div class="h-64 overflow-y-auto">
    {#if loading}
      <div class="grid grid-cols-2 gap-1.5">
        {#each skeletons as i (i)}
          <div class="h-24 animate-pulse rounded-md bg-elevated"></div>
        {/each}
      </div>
    {:else if failed}
      <p class="py-10 text-center text-label text-muted">GIF indisponible.</p>
    {:else if !q.trim()}
      <p class="py-10 text-center text-label text-muted">Cherche un GIF à envoyer.</p>
    {:else if !items.length}
      <p class="py-10 text-center text-label text-muted">Aucun résultat.</p>
    {:else}
      <div class="grid grid-cols-2 gap-1.5">
        {#each items as g (g.slug)}
          <button
            type="button"
            onclick={() => pick(g)}
            title={g.title}
            class="overflow-hidden rounded-md border border-border transition-colors duration-150
                   hover:border-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            <img src={g.preview} alt={g.title} loading="lazy" class="h-24 w-full object-cover" />
          </button>
        {/each}
      </div>
    {/if}
  </div>
  <p class="text-center text-label text-muted/70">Propulsé par KLIPY</p>
</div>
