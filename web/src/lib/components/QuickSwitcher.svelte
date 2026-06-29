<script lang="ts">
  import { goto } from '$app/navigation';
  import { Hash, Volume2, Search } from '@lucide/svelte';
  import { Dialog } from 'bits-ui';
  import { spaces, channelsBySpace, loadChannels } from '$lib/stores/spaces';

  let open = $state(false);
  let query = $state('');
  let activeIndex = $state(0);
  let hydrated = false;

  $effect(() => {
    if (open && !hydrated) {
      hydrated = true;
      for (const sp of $spaces.data) {
        if (!$channelsBySpace[sp.id]) void loadChannels(sp.id);
      }
    }
  });

  type Entry = {
    kind: 'space' | 'channel';
    id: string;
    label: string;
    sub?: string;
    voice?: boolean;
    href: string;
  };

  const entries = $derived.by<Entry[]>(() => {
    const out: Entry[] = [];
    for (const sp of $spaces.data) {
      out.push({ kind: 'space', id: sp.id, label: sp.name, href: `/app/spaces/${sp.id}` });
      const chans = $channelsBySpace[sp.id]?.data ?? [];
      for (const c of chans) {
        out.push({
          kind: 'channel',
          id: c.id,
          label: c.name,
          sub: sp.name,
          voice: c.type === 'voice',
          href: `/app/spaces/${sp.id}/channels/${c.id}`
        });
      }
    }
    return out;
  });

  const results = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? entries.filter((e) => e.label.toLowerCase().includes(q) || (e.sub ?? '').toLowerCase().includes(q))
      : entries;
    return list.slice(0, 50);
  });

  $effect(() => {
    void results;
    activeIndex = 0;
  });

  function onWindowKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      open = true;
      query = '';
    }
  }

  function choose(e: Entry) {
    open = false;
    void goto(e.href);
  }

  function searchEverywhere() {
    const q = query.trim();
    open = false;
    void goto(`/app/search${q ? `?q=${encodeURIComponent(q)}` : ''}`);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, results.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const sel = results[activeIndex];
      if (sel) choose(sel);
    }
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-fade-in" />
    <Dialog.Content
      class="fixed left-1/2 top-[15vh] z-50 w-[calc(100vw-2rem)] max-w-lg -translate-x-1/2 overflow-hidden
             rounded-lg border border-border bg-surface shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
    >
      <Dialog.Title class="sr-only">Aller à…</Dialog.Title>
      <div class="flex items-center gap-2 border-b border-border px-4">
        <Search size={18} class="shrink-0 text-muted" />
        <!-- svelte-ignore a11y_autofocus -->
        <input
          autofocus
          bind:value={query}
          onkeydown={onKeydown}
          placeholder="Aller à un salon ou un espace…"
          class="h-12 flex-1 bg-transparent text-body text-content outline-none placeholder:text-muted"
        />
      </div>
      <div class="max-h-80 overflow-y-auto p-1.5">
        {#each results as e, i (e.kind + e.id)}
          <button
            type="button"
            onclick={() => choose(e)}
            onpointermove={() => (activeIndex = i)}
            class="flex w-full items-center gap-2.5 rounded px-2.5 py-2 text-left transition-colors duration-75
                   {i === activeIndex ? 'bg-elevated' : 'hover:bg-surface'}"
          >
            {#if e.kind === 'space'}
              <span class="grid size-6 shrink-0 place-items-center rounded-md bg-brand/20 text-[0.625rem] font-semibold text-accent">
                {e.label.slice(0, 2).toUpperCase()}
              </span>
            {:else if e.voice}
              <Volume2 size={16} class="shrink-0 text-muted" />
            {:else}
              <Hash size={16} class="shrink-0 text-muted" />
            {/if}
            <span class="min-w-0 flex-1 truncate text-body text-content">{e.label}</span>
            {#if e.sub}
              <span class="shrink-0 truncate text-label text-muted">{e.sub}</span>
            {/if}
          </button>
        {:else}
          <p class="px-3 py-6 text-center text-label text-muted">Aucun salon trouvé.</p>
        {/each}
        {#if query.trim()}
          <button
            type="button"
            onclick={searchEverywhere}
            class="mt-1 flex w-full items-center gap-2.5 rounded border-t border-border px-2.5 py-2 text-left text-body text-muted transition-colors duration-75 hover:bg-elevated hover:text-content"
          >
            <Search size={16} class="shrink-0" />
            <span class="min-w-0 flex-1 truncate">Rechercher « {query.trim()} » dans tous les messages</span>
          </button>
        {/if}
      </div>
      <div class="flex items-center gap-3 border-t border-border px-4 py-2 text-label text-muted">
        <span><kbd class="rounded bg-base px-1.5 py-0.5 font-mono text-[0.625rem]">↑↓</kbd> naviguer</span>
        <span><kbd class="rounded bg-base px-1.5 py-0.5 font-mono text-[0.625rem]">↵</kbd> ouvrir</span>
        <span><kbd class="rounded bg-base px-1.5 py-0.5 font-mono text-[0.625rem]">Échap</kbd> fermer</span>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
