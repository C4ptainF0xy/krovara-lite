<script lang="ts">
  import { myEmojis } from '$lib/stores/emojis';
  import { myStickers } from '$lib/stores/stickers';
  import { spaces } from '$lib/stores/spaces';

  type Props = {
    src: string;
    name: string;
    fileKey?: string;
    isSticker?: boolean;
    x: number;
    y: number;
    onclose: () => void;
  };
  let { src, name, fileKey, isSticker = false, x, y, onclose }: Props = $props();

  const resolved = $derived.by(() => {
    const pool = isSticker ? $myStickers : $myEmojis;
    const item = fileKey
      ? pool.find((m) => m.file_key === fileKey)
      : pool.find((m) => m.name === name);
    if (!item) return { label: name, server: null as string | null };
    return {
      label: item.name,
      server: $spaces.data.find((s) => s.id === item.space_id)?.name ?? null
    };
  });

  const left = $derived(Math.max(8, Math.min(x - 96, (typeof window !== 'undefined' ? window.innerWidth : 9999) - 232)));
  const top = $derived(Math.max(8, y - 180));
</script>

<div
  class="pointer-events-none fixed z-50 w-56 rounded-lg border border-border bg-overlay p-3 shadow-xl animate-fade-in"
  style="left:{left}px; top:{top}px"
>
  <div class="flex flex-col items-center gap-2">
    <img {src} alt={resolved.label} class="size-24 object-contain" draggable="false" />
    <p class="text-body font-semibold text-content">{isSticker ? resolved.label : `:${resolved.label}:`}</p>
    {#if resolved.server}
      <p class="text-label text-muted">
        {isSticker ? 'Sticker' : 'Emoji'} de <span class="font-medium text-content">{resolved.server}</span>
      </p>
    {:else}
      <p class="text-label text-muted">{isSticker ? 'Sticker' : 'Emoji'} personnalisé</p>
    {/if}
  </div>
</div>
