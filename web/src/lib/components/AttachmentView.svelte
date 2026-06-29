<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Dialog } from 'bits-ui';
  import { FileText, Download, X, QrCode, EyeOff } from '@lucide/svelte';
  import jsQR from 'jsqr';
  import { authedObjectURL } from '$lib/api';

  type Props = { url: string; name?: string; sticker?: boolean; spoiler?: boolean };
  let { url, name, sticker = false, spoiler = false }: Props = $props();

  let lightboxOpen = $state(false);

  const DANGEROUS = /\.(exe|msi|bat|cmd|scr|com|ps1|vbs|js|jar|apk|zip|rar|7z|gz|iso|dmg|sh|deb)$/i;
  const isDangerous = $derived(DANGEROUS.test(name ?? url));
  let confirmDl = $state(false);
  function doDownload() {
    if (!objUrl) return;
    const a = document.createElement('a');
    a.href = objUrl;
    a.download = name || 'fichier';
    document.body.appendChild(a);
    a.click();
    a.remove();
    confirmDl = false;
  }
  let spoilerRevealed = $state(false);
  const blurred = $derived(spoiler && !spoilerRevealed);

  let qrUrl = $state<string | null>(null);
  let qrRevealed = $state(false);

  function scanQR(img: HTMLImageElement) {
    if (!objUrl || !objUrl.startsWith('blob:')) return;
    try {
      const w = img.naturalWidth;
      const h = img.naturalHeight;
      if (!w || !h) return;
      const scale = Math.min(1, 1024 / Math.max(w, h));
      const cw = Math.round(w * scale);
      const ch = Math.round(h * scale);
      const canvas = document.createElement('canvas');
      canvas.width = cw;
      canvas.height = ch;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.drawImage(img, 0, 0, cw, ch);
      const data = ctx.getImageData(0, 0, cw, ch);
      const res = jsQR(data.data, cw, ch);
      if (res?.data && /^https?:\/\//i.test(res.data.trim())) {
        qrUrl = res.data.trim();
      }
    } catch {
    }
  }

  let mime = $state('');
  const extSrc = $derived(`${url} ${name ?? ''}`);
  const isImage = $derived(
    mime.startsWith('image/') || (!mime && /\.(png|jpe?g|gif|webp|avif|bmp|svg)(\?|$)/i.test(extSrc))
  );
  const isVideo = $derived(
    mime.startsWith('video/') || (!mime && /\.(mp4|webm|ogv|mov|m4v)(\?|$)/i.test(extSrc))
  );
  const isAudio = $derived(
    mime.startsWith('audio/') || (!mime && /\.(mp3|wav|ogg|oga|m4a|aac|flac|opus)(\?|$)/i.test(extSrc))
  );

  let objUrl = $state<string | null>(null);
  let failed = $state(false);

  $effect(() => {
    const u = url;
    failed = false;
    if (/^https?:\/\//i.test(u)) {
      objUrl = u;
      return;
    }
    let cancelled = false;
    let blob: string | null = null;
    (async () => {
      for (let attempt = 0; attempt < 6 && !cancelled; attempt++) {
        try {
          const o = await authedObjectURL(u);
          if (cancelled) {
            URL.revokeObjectURL(o);
            return;
          }
          objUrl = o;
          blob = o;
          try {
            mime = (await (await fetch(o)).blob()).type || '';
          } catch {
            mime = '';
          }
          return;
        } catch {
          if (attempt < 5) await new Promise((r) => setTimeout(r, 1200));
        }
      }
      if (!cancelled) failed = true;
    })();
    return () => {
      cancelled = true;
      if (blob) URL.revokeObjectURL(blob);
    };
  });

  onDestroy(() => {
    if (objUrl && objUrl.startsWith('blob:')) URL.revokeObjectURL(objUrl);
  });
</script>

{#if failed}
  <p class="mt-1 text-label text-danger">Pièce jointe indisponible.</p>
{:else}
<div class="relative w-fit max-w-full">
  <div class={blurred ? 'pointer-events-none select-none blur-2xl' : ''}>
  {#if sticker}
  {#if objUrl}
    <img
      src={objUrl}
      alt={name ?? 'sticker'}
      data-sticker-key={url.split('/').pop()?.split('?')[0]}
      class="mt-1 max-h-32 max-w-[8rem] cursor-pointer object-contain"
    />
  {:else}
    <div class="mt-1 size-28 animate-pulse rounded-lg bg-elevated"></div>
  {/if}
{:else if isImage}
  {#if objUrl}
    <button
      type="button"
      onclick={() => (lightboxOpen = true)}
      class="mt-1 block w-fit cursor-zoom-in rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
    >
      <img
        src={objUrl}
        alt={name ?? 'image'}
        onload={(e) => scanQR(e.currentTarget as HTMLImageElement)}
        class="max-h-80 max-w-full sm:max-w-sm rounded-lg border border-border object-contain"
      />
    </button>
    {#if qrUrl}
      <div class="mt-1 flex max-w-full sm:max-w-sm flex-col gap-1 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2">
        <span class="flex items-center gap-1.5 text-label font-medium text-warning">
          <QrCode size={14} /> QR code détecté dans l'image
        </span>
        {#if qrRevealed}
          <p class="break-all text-label text-muted">{qrUrl}</p>
        {:else}
          <button
            type="button"
            onclick={() => (qrRevealed = true)}
            class="w-fit text-label text-accent underline underline-offset-2 hover:text-primary-hover"
          >
            Révéler l'URL (ne l'ouvre pas automatiquement)
          </button>
        {/if}
      </div>
    {/if}
    <Dialog.Root open={lightboxOpen} onOpenChange={(v) => (lightboxOpen = v)}>
      <Dialog.Portal>
        <Dialog.Overlay
          class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm data-[state=open]:animate-fade-in"
        />
        <Dialog.Content
          class="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2
                 data-[state=open]:animate-fade-in"
        >
          <Dialog.Title class="sr-only">{name ?? 'image'}</Dialog.Title>
          <img
            src={objUrl}
            alt={name ?? 'image'}
            class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl shadow-black/50"
          />
          <a
            href={objUrl}
            download={name || 'krovara-image'}
            title="Télécharger"
            class="absolute -top-3 right-9 grid size-9 place-items-center rounded-full border border-border
                   bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content"
          >
            <Download size={17} />
          </a>
          <Dialog.Close
            class="absolute -right-3 -top-3 grid size-9 place-items-center rounded-full border border-border
                   bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content"
            aria-label="Fermer"
          >
            <X size={18} />
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  {:else}
    <div class="mt-1 h-40 w-64 animate-pulse rounded-lg bg-elevated"></div>
  {/if}
{:else if isVideo}
  {#if objUrl}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video
      src={objUrl}
      controls
      preload="metadata"
      class="mt-1 max-h-80 max-w-full sm:max-w-sm rounded-lg border border-border bg-base"
    ></video>
  {:else}
    <div class="mt-1 h-44 w-64 animate-pulse rounded-lg bg-elevated"></div>
  {/if}
{:else if isAudio}
  {#if objUrl}
    <audio src={objUrl} controls preload="metadata" class="mt-1 w-full max-w-full sm:max-w-sm"></audio>
  {:else}
    <div class="mt-1 h-12 w-64 animate-pulse rounded-lg bg-elevated"></div>
  {/if}
{:else if isDangerous}
  <button
    type="button"
    onclick={() => (confirmDl = true)}
    class="mt-1 flex w-fit max-w-full sm:max-w-sm items-center gap-2.5 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2
           text-left transition-colors duration-150 hover:bg-warning/10"
  >
    <FileText size={20} class="shrink-0 text-warning" />
    <span class="truncate text-body text-content">{name ?? 'fichier'}</span>
    <Download size={16} class="ml-auto shrink-0 text-warning" />
  </button>
{:else}
  <a
    href={objUrl ?? undefined}
    download={name}
    class="mt-1 flex w-fit max-w-full sm:max-w-sm items-center gap-2.5 rounded-lg border border-border bg-surface/60 px-3 py-2
           transition-colors duration-150 hover:bg-elevated"
  >
    <FileText size={20} class="shrink-0 text-muted" />
    <span class="truncate text-body text-content">{name ?? 'fichier'}</span>
    <Download size={16} class="ml-auto shrink-0 text-muted" />
  </a>
{/if}
  </div>
  {#if blurred}
    <button
      type="button"
      onclick={() => (spoilerRevealed = true)}
      class="absolute inset-0 mt-1 grid place-items-center rounded-lg bg-base/30 transition-colors hover:bg-base/20"
      aria-label="Révéler le spoiler"
    >
      <span class="flex items-center gap-1.5 rounded-full bg-base/80 px-3 py-1.5 text-label font-semibold text-content shadow">
        <EyeOff size={14} /> Spoiler · cliquer pour révéler
      </span>
    </button>
  {/if}
</div>
{/if}

<Dialog.Root open={confirmDl} onOpenChange={(v) => (confirmDl = v)}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-fade-in" />
    <Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-surface p-5 shadow-2xl data-[state=open]:animate-fade-in">
      <div class="flex items-start gap-3">
        <EyeOff size={20} class="mt-0.5 shrink-0 text-warning" />
        <div class="min-w-0">
          <Dialog.Title class="text-subtitle font-semibold text-content">Télécharger ce fichier ?</Dialog.Title>
          <p class="mt-1 text-body text-muted">
            Ce type de fichier peut être dangereux pour ton appareil. Ne le télécharge que si tu fais confiance à son auteur.
          </p>
          <p class="mt-2 break-all rounded-md border border-border bg-base/60 px-2.5 py-1.5 text-label text-muted">{name ?? 'fichier'}</p>
        </div>
      </div>
      <div class="mt-4 flex justify-end gap-2">
        <button type="button" onclick={() => (confirmDl = false)} class="rounded-md px-3 py-1.5 text-label text-muted hover:bg-elevated hover:text-content">Annuler</button>
        <button type="button" onclick={doDownload} class="rounded-md bg-warning px-3 py-1.5 text-label font-medium text-white hover:brightness-110">Télécharger quand même</button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
