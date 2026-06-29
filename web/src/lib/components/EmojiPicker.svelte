<script lang="ts">
  import { Search, Sparkles } from '@lucide/svelte';
  import { EMOJI_CATEGORIES, searchEmoji } from '$lib/emoji/data';
  import { emojisBySpace, loadEmojis, myEmojis, loadMyEmojis, emojiUrl, type CustomEmoji } from '$lib/stores/emojis';
  import { spaces } from '$lib/stores/spaces';

  type Props = {
    onpick: (emoji: string) => void;
    spaceId?: string;
    scope?: 'server' | 'krovara';
    allowCustom?: boolean;
  };
  let { onpick, spaceId, scope, allowCustom = true }: Props = $props();

  const sc = $derived(scope ?? (spaceId ? 'server' : 'krovara'));
  const customOn = $derived(allowCustom);

  let activeCat = $state(EMOJI_CATEGORIES[0].id);
  let query = $state('');
  const q = $derived(query.trim().toLowerCase());

  const shown = $derived(
    q ? searchEmoji(q) : (EMOJI_CATEGORIES.find((c) => c.id === activeCat) ?? EMOJI_CATEGORIES[0]).items
  );

  const spaceName = (id: string) => $spaces.data.find((s) => s.id === id)?.name ?? 'Serveur';
  const match = (e: CustomEmoji) => !q || e.name.includes(q);

  const serverShown = $derived(
    customOn && sc === 'server' && spaceId ? ($emojisBySpace[spaceId] ?? []).filter(match) : []
  );

  const groupedShown = $derived.by(() => {
    if (!customOn || sc !== 'krovara') return [] as { id: string; name: string; emojis: CustomEmoji[] }[];
    const byId = new Map<string, CustomEmoji[]>();
    for (const e of $myEmojis) {
      if (!match(e)) continue;
      (byId.get(e.space_id) ?? byId.set(e.space_id, []).get(e.space_id)!).push(e);
    }
    return [...byId.entries()]
      .map(([id, emojis]) => ({ id, name: spaceName(id), emojis }))
      .sort((a, b) => a.name.localeCompare(b.name));
  });

  const hasCustom = $derived(serverShown.length > 0 || groupedShown.length > 0);

  let thumbs = $state<Record<string, string>>({});

  $effect(() => {
    if (customOn && sc === 'server' && spaceId && $emojisBySpace[spaceId] === undefined) {
      void loadEmojis(spaceId).catch(() => {});
    }
  });
  $effect(() => {
    if (customOn && sc === 'krovara') void loadMyEmojis().catch(() => {});
  });
  $effect(() => {
    const keys = Array.from(
      new Set([...serverShown, ...groupedShown.flatMap((g) => g.emojis)].map((e) => e.file_key))
    );
    if (keys.length === 0) return;
    let cancelled = false;
    void Promise.all(keys.map(async (k) => [k, await emojiUrl(k)] as const)).then((entries) => {
      if (!cancelled) thumbs = { ...thumbs, ...Object.fromEntries(entries) };
    });
    return () => {
      cancelled = true;
    };
  });
</script>

{#snippet emojiGrid(list: CustomEmoji[], token: (e: CustomEmoji) => string)}
  <div class="mb-2 grid grid-cols-8 gap-0.5">
    {#each list as e (e.id)}
      <button
        type="button"
        title={':' + e.name + ':'}
        onclick={() => onpick(token(e))}
        class="grid size-8 place-items-center rounded transition-colors hover:bg-elevated"
      >
        {#if thumbs[e.file_key]}
          <img src={thumbs[e.file_key]} alt={e.name} class="size-6 object-contain" draggable="false" />
        {:else}
          <span class="size-6 animate-pulse rounded bg-elevated"></span>
        {/if}
      </button>
    {/each}
  </div>
{/snippet}

<div class="w-72 rounded-lg border border-border bg-overlay shadow-xl">
  <div class="border-b border-border p-2">
    <div class="relative">
      <Search size={14} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted" />
      <input
        bind:value={query}
        placeholder="Rechercher un emoji…"
        class="h-8 w-full rounded-md border border-border bg-base/50 pl-8 pr-2 text-label text-content outline-none focus:border-primary"
      />
    </div>
  </div>

  {#if !q}
    <div class="flex gap-0.5 border-b border-border px-1.5 py-1">
      {#each EMOJI_CATEGORIES as c (c.id)}
        <button
          type="button"
          title={c.label}
          aria-label={c.label}
          aria-pressed={activeCat === c.id}
          onclick={() => (activeCat = c.id)}
          class="grid size-7 place-items-center rounded text-base transition-colors
                 {activeCat === c.id ? 'bg-elevated' : 'opacity-60 hover:opacity-100 hover:bg-elevated/60'}"
        >
          {c.icon}
        </button>
      {/each}
    </div>
  {/if}

  <div class="max-h-56 overflow-y-auto p-2">
    {#if serverShown.length > 0}
      <p class="px-1 pb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">
        {spaceId ? spaceName(spaceId) : 'Serveur'}
      </p>
      {@render emojiGrid(serverShown, (e) => ':' + e.name + ':')}
    {/if}

    {#each groupedShown as g (g.id)}
      <p class="flex items-center gap-1 px-1 pb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">
        <Sparkles size={11} class="text-accent" /> {g.name}
      </p>
      {@render emojiGrid(g.emojis, (e) => '<:' + e.name + ':' + e.file_key + '>')}
    {/each}

    {#if shown.length === 0 && !hasCustom}
      <p class="px-1 py-6 text-center text-label text-muted">Aucun emoji.</p>
    {:else if shown.length > 0}
      {#if hasCustom}
        <p class="px-1 pb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Standard</p>
      {/if}
      <div class="grid grid-cols-8 gap-0.5">
        {#each shown as it (it.e)}
          <button
            type="button"
            title={it.k.split(' ')[0]}
            onclick={() => onpick(it.e)}
            class="grid size-8 place-items-center rounded text-lg transition-colors hover:bg-elevated"
          >
            {it.e}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>
