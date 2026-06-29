<script lang="ts">
  import { Hash, Volume2, Lock } from '@lucide/svelte';
  import type { Channel } from '$lib/stores/spaces';
  import { emojisBySpace, emojiUrl } from '$lib/stores/emojis';

  type Props = {
    channel: Channel;
    spaceId?: string;
    active?: boolean;
    unread?: number;
    mentions?: number;
    onclick?: () => void;
    ondblclick?: () => void;
  };
  let { channel, spaceId, active = false, unread = 0, mentions = 0, onclick, ondblclick }: Props = $props();

  const isVoice = $derived(channel.type === 'voice');

  let clickTimer: ReturnType<typeof setTimeout> | null = null;
  function handleClick() {
    if (!isVoice || !ondblclick) {
      onclick?.();
      return;
    }
    if (clickTimer) clearTimeout(clickTimer);
    clickTimer = setTimeout(() => {
      clickTimer = null;
      onclick?.();
    }, 220);
  }
  function handleDblClick() {
    if (clickTimer) {
      clearTimeout(clickTimer);
      clickTimer = null;
    }
    ondblclick?.();
  }

  const hasMention = $derived(mentions > 0 && !active);
  const isUnread = $derived(unread > 0 && !active);
  const highlight = $derived(hasMention || isUnread);

  const customName = $derived.by(() => {
    const m = /^:([a-z0-9_]{2,32}):$/.exec(channel.icon_emoji ?? '');
    return m ? m[1] : null;
  });
  let customIconUrl = $state<string | null>(null);
  $effect(() => {
    const nm = customName;
    if (!nm || !spaceId) {
      customIconUrl = null;
      return;
    }
    const e = ($emojisBySpace[spaceId] ?? []).find((x) => x.name === nm);
    if (!e) {
      customIconUrl = null;
      return;
    }
    let cancelled = false;
    void emojiUrl(e.file_key).then((u) => {
      if (!cancelled) customIconUrl = u;
    });
    return () => {
      cancelled = true;
    };
  });
</script>

<button
  type="button"
  onclick={handleClick}
  ondblclick={handleDblClick}
  aria-current={active ? 'page' : undefined}
  class="group relative flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-body
         transition-colors duration-150 ease-smooth
         {active
    ? 'bg-elevated text-content'
    : highlight
      ? 'font-medium text-content hover:bg-surface'
      : 'text-muted hover:bg-surface hover:text-content'}"
>
  {#if isUnread && !hasMention}
    <span
      class="absolute -left-1 top-1/2 h-2 w-1 -translate-y-1/2 rounded-r-full bg-content"
      aria-label="Messages non lus"
    ></span>
  {/if}
  {#if customName}
    {#if customIconUrl}
      <img src={customIconUrl} alt="" class="size-4 shrink-0 object-contain" />
    {:else}
      <Hash size={16} class="shrink-0 opacity-70" />
    {/if}
  {:else if channel.icon_emoji}
    <span class="shrink-0 text-center text-sm leading-none" aria-hidden="true">{channel.icon_emoji}</span>
  {:else if isVoice}
    <Volume2 size={16} class="shrink-0 opacity-70" />
  {:else}
    <Hash size={16} class="shrink-0 opacity-70" />
  {/if}
  <span class="truncate">{channel.name}</span>
  {#if channel.locked}
    <Lock size={13} class="shrink-0 opacity-60" aria-label="Salon verrouillé" />
  {/if}
  {#if hasMention}
    <span
      class="ml-auto grid h-5 min-w-5 shrink-0 place-items-center rounded-full bg-danger px-1.5 text-[0.625rem] font-semibold tabular-nums text-white"
      aria-label="{mentions} mention{mentions > 1 ? 's' : ''}"
    >
      {mentions > 9 ? '10+' : mentions}
    </span>
  {/if}
</button>
