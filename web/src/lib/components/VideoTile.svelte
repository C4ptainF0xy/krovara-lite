<script lang="ts">
  import { Volume2 } from '@lucide/svelte';

  type Props = {
    stream: MediaStream;
    label: string;
    muted?: boolean;
    spotlighted?: boolean;
    volume?: number;
    onselect?: () => void;
    onvolume?: (v: number) => void;
  };
  let {
    stream,
    label,
    muted = false,
    spotlighted = false,
    volume = 1,
    onselect,
    onvolume
  }: Props = $props();

  let videoEl: HTMLVideoElement | null = $state(null);
  let hasVideo = $state(false);

  $effect(() => {
    if (videoEl) videoEl.srcObject = stream;
    const sync = () => (hasVideo = stream.getVideoTracks().some((t) => t.readyState === 'live'));
    sync();
    stream.addEventListener('addtrack', sync);
    stream.addEventListener('removetrack', sync);
    return () => {
      stream.removeEventListener('addtrack', sync);
      stream.removeEventListener('removetrack', sync);
    };
  });

  $effect(() => {
    if (videoEl) videoEl.volume = muted ? 0 : volume;
  });
</script>

<div
  class="group relative aspect-video overflow-hidden rounded-lg bg-base ring-1 transition-shadow duration-150 {spotlighted
    ? 'ring-brand'
    : 'ring-border'}"
>
  <button
    type="button"
    onclick={onselect}
    aria-label={`Mettre en avant ${label}`}
    class="absolute inset-0 h-full w-full"
  >
    <!-- svelte-ignore a11y_media_has_caption -->
    <video
      bind:this={videoEl}
      autoplay
      playsinline
      {muted}
      class="h-full w-full object-cover {hasVideo ? '' : 'invisible'}"
    ></video>
    {#if !hasVideo}
      <div class="absolute inset-0 flex items-center justify-center">
        <span class="flex size-12 items-center justify-center rounded-full bg-elevated text-subtitle font-semibold text-muted">
          {label.slice(0, 2).toUpperCase()}
        </span>
      </div>
    {/if}
  </button>

  <span class="pointer-events-none absolute bottom-1 left-1 rounded bg-base/70 px-1.5 py-0.5 text-label text-content backdrop-blur">
    {label}
  </span>

  {#if !muted && onvolume}
    <div
      class="absolute right-1 top-1 flex items-center gap-1 rounded-full bg-base/70 px-2 py-1 opacity-0 backdrop-blur transition-opacity duration-150 focus-within:opacity-100 group-hover:opacity-100"
    >
      <Volume2 size={14} strokeWidth={2} class="shrink-0 text-muted" />
      <input
        type="range"
        min="0"
        max="1"
        step="0.05"
        value={volume}
        aria-label={`Volume de ${label}`}
        oninput={(e) => onvolume?.(Number(e.currentTarget.value))}
        onclick={(e) => e.stopPropagation()}
        class="h-1 w-16 cursor-pointer accent-primary"
      />
    </div>
  {/if}
</div>
