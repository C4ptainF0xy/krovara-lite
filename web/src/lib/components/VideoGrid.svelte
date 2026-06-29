<script lang="ts">
  import {
    roomPeers,
    localVideo,
    requestLayer,
    peerVolumes,
    setPeerVolume,
    type Layer
  } from '$lib/voip/sfu';
  import VideoTile from './VideoTile.svelte';
  import { Maximize2, Minimize2 } from '@lucide/svelte';

  const MAX_TILES = 12;

  let spotlight = $state<string | null>(null);

  type Tile = { id: string; stream: MediaStream; label: string; isLocal: boolean };

  const tiles = $derived.by<Tile[]>(() => {
    const out: Tile[] = [];
    if ($localVideo) {
      out.push({ id: '__local', stream: $localVideo, label: 'You', isLocal: true });
    }
    for (const p of $roomPeers) {
      out.push({ id: p.peerId, stream: p.stream, label: p.peerId.slice(0, 8), isLocal: false });
    }
    return out.slice(0, MAX_TILES);
  });

  const spotlit = $derived(tiles.find((t) => t.id === spotlight) ?? null);
  const others = $derived(spotlit ? tiles.filter((t) => t.id !== spotlight) : tiles);

  function toggle(id: string) {
    spotlight = spotlight === id ? null : id;
  }

  let gridEl = $state<HTMLDivElement | null>(null);
  let isFullscreen = $state(false);

  function toggleFullscreen() {
    if (!gridEl) return;
    if (document.fullscreenElement) void document.exitFullscreen();
    else void gridEl.requestFullscreen().catch((e) => console.error('fullscreen', e));
  }

  $effect(() => {
    const onChange = () => (isFullscreen = !!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  });

  const sentLayers = new Map<string, Layer>();
  $effect(() => {
    for (const t of tiles) {
      if (t.isLocal) continue;
      const want: Layer = spotlight === null ? 'm' : t.id === spotlight ? 'h' : 'l';
      if (sentLayers.get(t.id) !== want) {
        sentLayers.set(t.id, want);
        requestLayer(t.id, want);
      }
    }
  });
</script>

<div
  bind:this={gridEl}
  class="relative flex flex-col gap-2 {isFullscreen ? 'h-full justify-center bg-base p-4' : ''}"
>
  {#if tiles.length > 0}
    <button
      type="button"
      onclick={toggleFullscreen}
      title={isFullscreen ? 'Quitter le plein écran' : 'Plein écran'}
      class="absolute right-1 top-1 z-10 grid size-7 place-items-center rounded-md
             bg-base/70 text-muted backdrop-blur transition-colors duration-150
             hover:text-content"
    >
      {#if isFullscreen}<Minimize2 size={15} />{:else}<Maximize2 size={15} />{/if}
    </button>
  {/if}
  {#if spotlit}
    <VideoTile
      stream={spotlit.stream}
      label={spotlit.label}
      muted={spotlit.isLocal}
      spotlighted
      volume={$peerVolumes[spotlit.id] ?? 1}
      onselect={() => toggle(spotlit.id)}
      onvolume={spotlit.isLocal ? undefined : (v) => setPeerVolume(spotlit.id, v)}
    />
  {/if}
  {#if others.length > 0}
    <div
      class="grid gap-2"
      style="grid-template-columns: repeat(auto-fill, minmax({spotlit ? '6rem' : '10rem'}, 1fr));"
    >
      {#each others as t (t.id)}
        <VideoTile
          stream={t.stream}
          label={t.label}
          muted={t.isLocal}
          volume={$peerVolumes[t.id] ?? 1}
          onselect={() => toggle(t.id)}
          onvolume={t.isLocal ? undefined : (v) => setPeerVolume(t.id, v)}
        />
      {/each}
    </div>
  {/if}
  {#if tiles.length === 0}
    <p class="text-label italic text-muted">Seul dans le salon.</p>
  {/if}
</div>
