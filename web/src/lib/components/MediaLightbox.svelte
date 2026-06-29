<script lang="ts">
  import { Dialog } from 'bits-ui';
  import { X, Link as LinkIcon, Check, ExternalLink, Download } from '@lucide/svelte';

  let { open = $bindable(false), url, kind }: { open?: boolean; url: string; kind: 'image' | 'video' } =
    $props();

  let copied = $state(false);
  function copyLink() {
    void navigator.clipboard?.writeText(url).then(() => {
      copied = true;
      setTimeout(() => (copied = false), 1500);
    });
  }

  function fileName(): string {
    try {
      const p = new URL(url).pathname.split('/').pop();
      return p && p.includes('.') ? p : 'krovara-media';
    } catch {
      return 'krovara-media';
    }
  }

  let downloading = $state(false);
  async function download() {
    if (downloading) return;
    downloading = true;
    try {
      const res = await fetch(url);
      const blob = await res.blob();
      const obj = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = obj;
      a.download = fileName();
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(obj), 1000);
    } catch {
      window.open(url, '_blank', 'noopener');
    } finally {
      downloading = false;
    }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm data-[state=open]:animate-fade-in" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 data-[state=open]:animate-fade-in"
    >
      <Dialog.Title class="sr-only">Aperçu du média</Dialog.Title>
      {#if kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={url} controls autoplay class="max-h-[90vh] max-w-[90vw] rounded-lg shadow-2xl shadow-black/50"></video>
      {:else}
        <img src={url} alt="" class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl shadow-black/50" />
      {/if}

      <div class="absolute -top-3 right-9 flex gap-1.5">
        <button
          type="button"
          onclick={download}
          disabled={downloading}
          title="Télécharger"
          class="grid size-9 place-items-center rounded-full border border-border bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content disabled:opacity-50"
        >
          <Download size={17} />
        </button>
        <button
          type="button"
          onclick={copyLink}
          title="Copier le lien"
          class="grid size-9 place-items-center rounded-full border border-border bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          {#if copied}<Check size={17} class="text-success" />{:else}<LinkIcon size={17} />{/if}
        </button>
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          title="Ouvrir dans un nouvel onglet"
          class="grid size-9 place-items-center rounded-full border border-border bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          <ExternalLink size={17} />
        </a>
      </div>
      <Dialog.Close
        class="absolute -right-3 -top-3 grid size-9 place-items-center rounded-full border border-border bg-surface text-muted shadow-lg transition-colors duration-150 hover:bg-elevated hover:text-content"
        aria-label="Fermer"
      >
        <X size={18} />
      </Dialog.Close>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
