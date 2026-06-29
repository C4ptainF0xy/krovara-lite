<script lang="ts">
  import type { Space } from '$lib/stores/spaces';
  import { emojisBySpace, loadEmojis, emojiUrl } from '$lib/stores/emojis';
  import { authedObjectURL } from '$lib/api';

  type Props = { space: Space; active?: boolean; onclick?: () => void };
  let { space, active = false, onclick }: Props = $props();

  const initials = $derived((space.name ?? '').trim().slice(0, 2).toUpperCase() || '?');

  const EMOJI_RE = /^:([a-z0-9_]{2,32}):$/;
  const iconName = $derived.by(() => {
    const m = EMOJI_RE.exec(space.icon_key ?? '');
    return m ? m[1] : null;
  });
  const rawUrl = $derived.by(() => {
    const k = space.icon_key ?? '';
    return /^https?:\/\//.test(k) ? k : null;
  });
  const fileId = $derived.by(() => {
    const k = space.icon_key ?? '';
    return k && !EMOJI_RE.test(k) && !/^https?:\/\//.test(k) ? k : null;
  });

  let iconUrl = $state<string | null>(null);
  $effect(() => {
    const nm = iconName;
    if (!nm) {
      iconUrl = null;
      return;
    }
    if ($emojisBySpace[space.id] === undefined) {
      void loadEmojis(space.id).catch(() => {});
      return;
    }
    const e = ($emojisBySpace[space.id] ?? []).find((x) => x.name === nm);
    if (!e) {
      iconUrl = null;
      return;
    }
    let cancelled = false;
    void emojiUrl(e.file_key).then((u) => {
      if (!cancelled) iconUrl = u;
    });
    return () => {
      cancelled = true;
    };
  });

  let fileUrl = $state<string | null>(null);
  $effect(() => {
    const fid = fileId;
    if (!fid) {
      fileUrl = null;
      return;
    }
    let cancelled = false;
    let created: string | null = null;
    void authedObjectURL(`/api/files/${fid}`)
      .then((u) => {
        if (cancelled) {
          URL.revokeObjectURL(u);
          return;
        }
        created = u;
        fileUrl = u;
      })
      .catch(() => {});
    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  });
</script>

<button
  type="button"
  {onclick}
  title={space.name}
  aria-current={active ? 'page' : undefined}
  class="group relative grid size-12 place-items-center rounded-2xl text-label font-semibold
         transition-[border-radius,background-color,color] duration-150 ease-smooth
         hover:rounded-xl
         {active
    ? 'rounded-xl bg-primary text-white'
    : 'bg-elevated text-muted hover:bg-primary hover:text-white'}"
>
  {#if iconName}
    {#if iconUrl}
      <img src={iconUrl} alt="" class="size-3/4 rounded-[inherit] object-contain" />
    {:else}
      {initials}
    {/if}
  {:else if fileId}
    {#if fileUrl}
      <img src={fileUrl} alt="" class="size-full rounded-[inherit] object-cover" />
    {:else}
      {initials}
    {/if}
  {:else if rawUrl}
    <img src={rawUrl} alt="" class="size-full rounded-[inherit] object-cover" />
  {:else}
    {initials}
  {/if}

  <span
    class="absolute -left-3 top-1/2 w-1 -translate-y-1/2 rounded-r-full bg-content
           transition-all duration-150 ease-smooth
           {active ? 'h-6' : 'h-0 group-hover:h-3'}"
    aria-hidden="true"
  ></span>
</button>
