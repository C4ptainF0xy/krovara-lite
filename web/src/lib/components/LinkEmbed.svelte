<script lang="ts">
  import { Play } from '@lucide/svelte';
  import type { MediaLink } from '$lib/render/links';
  import MediaLightbox from './MediaLightbox.svelte';

  let { link }: { link: MediaLink } = $props();

  let playing = $state(false);
  let lightbox = $state(false);
  const ytThumb = $derived(link.kind === 'youtube' ? `https://i.ytimg.com/vi/${link.id}/hqdefault.jpg` : '');
  const twitchParent = typeof window !== 'undefined' ? window.location.hostname : 'krovara.com';
</script>

{#if link.kind === 'image'}
  <button type="button" onclick={() => (lightbox = true)} class="block w-fit cursor-zoom-in">
    <img
      src={link.url}
      alt=""
      loading="lazy"
      class="max-h-80 max-w-full rounded-lg object-contain"
    />
  </button>
  <MediaLightbox bind:open={lightbox} url={link.url} kind="image" />
{:else if link.kind === 'video'}
  <!-- svelte-ignore a11y_media_has_caption -->
  <video src={link.url} controls preload="metadata" class="max-h-80 max-w-full rounded-lg"></video>
{:else if link.kind === 'youtube'}
  <div class="aspect-video w-full max-w-md overflow-hidden rounded-lg bg-black">
    {#if playing}
      <iframe
        src={`https://www.youtube-nocookie.com/embed/${link.id}?autoplay=1`}
        title="YouTube"
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowfullscreen
        class="size-full"
      ></iframe>
    {:else}
      <button type="button" onclick={() => (playing = true)} class="group relative block size-full">
        <img src={ytThumb} alt="" class="size-full object-cover" />
        <span class="absolute inset-0 grid place-items-center bg-black/30 transition-colors group-hover:bg-black/40">
          <span class="grid size-14 place-items-center rounded-full bg-danger/90 text-white shadow-lg">
            <Play size={26} class="ml-0.5 fill-current" />
          </span>
        </span>
      </button>
    {/if}
  </div>
{:else if link.kind === 'twitch-clip'}
  <div class="aspect-video w-full max-w-md overflow-hidden rounded-lg bg-black">
    <iframe
      src={`https://clips.twitch.tv/embed?clip=${link.slug}&parent=${twitchParent}`}
      title="Twitch clip"
      allowfullscreen
      class="size-full"
    ></iframe>
  </div>
{:else if link.kind === 'twitch-channel'}
  <div class="aspect-video w-full max-w-md overflow-hidden rounded-lg bg-black">
    <iframe
      src={`https://player.twitch.tv/?channel=${link.channel}&parent=${twitchParent}`}
      title="Twitch"
      allowfullscreen
      class="size-full"
    ></iframe>
  </div>
{/if}
