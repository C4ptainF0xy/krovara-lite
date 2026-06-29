<script lang="ts">
  import {
    stickersBySpace,
    loadStickers,
    myStickers,
    loadMyStickers,
    stickerUrl,
    type CustomSticker
  } from '$lib/stores/stickers';
  import { spaces } from '$lib/stores/spaces';

  type Props = { spaceId?: string; onpick: (s: CustomSticker) => void };
  let { spaceId, onpick }: Props = $props();

  const spaceName = (id: string) => $spaces.data.find((s) => s.id === id)?.name ?? 'Serveur';

  const groups = $derived.by(() => {
    if (spaceId) {
      const list = $stickersBySpace[spaceId] ?? [];
      return list.length ? [{ id: spaceId, name: spaceName(spaceId), stickers: list }] : [];
    }
    const byId = new Map<string, CustomSticker[]>();
    for (const s of $myStickers) {
      (byId.get(s.space_id) ?? byId.set(s.space_id, []).get(s.space_id)!).push(s);
    }
    return [...byId.entries()]
      .map(([id, stickers]) => ({ id, name: spaceName(id), stickers }))
      .sort((a, b) => a.name.localeCompare(b.name));
  });

  let thumbs = $state<Record<string, string>>({});

  $effect(() => {
    if (spaceId) {
      if ($stickersBySpace[spaceId] === undefined) void loadStickers(spaceId).catch(() => {});
    } else {
      void loadMyStickers().catch(() => {});
    }
  });
  $effect(() => {
    const keys = Array.from(new Set(groups.flatMap((g) => g.stickers).map((s) => s.file_key)));
    if (keys.length === 0) return;
    let cancelled = false;
    void Promise.all(keys.map(async (k) => [k, await stickerUrl(k)] as const)).then((entries) => {
      if (!cancelled) thumbs = { ...thumbs, ...Object.fromEntries(entries) };
    });
    return () => {
      cancelled = true;
    };
  });
</script>

<div class="w-72 rounded-lg border border-border bg-overlay p-2 shadow-xl">
  {#if groups.length === 0}
    <p class="px-2 py-6 text-center text-label text-muted">
      Aucun sticker. Un admin peut en ajouter dans les paramètres de l'espace.
    </p>
  {:else}
    <div class="max-h-60 overflow-y-auto">
      {#each groups as g (g.id)}
        <p class="px-1 pb-1 pt-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">{g.name}</p>
        <div class="mb-2 grid grid-cols-3 gap-1.5">
          {#each g.stickers as s (s.id)}
            <button
              type="button"
              title={s.name}
              onclick={() => onpick(s)}
              class="grid aspect-square place-items-center rounded-lg p-1 transition-colors hover:bg-elevated"
            >
              {#if thumbs[s.file_key]}
                <img src={thumbs[s.file_key]} alt={s.name} class="max-h-full max-w-full object-contain" draggable="false" />
              {:else}
                <span class="size-full animate-pulse rounded bg-elevated"></span>
              {/if}
            </button>
          {/each}
        </div>
      {/each}
    </div>
  {/if}
</div>
