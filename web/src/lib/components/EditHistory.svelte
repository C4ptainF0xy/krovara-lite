<script lang="ts">
  import Modal from './Modal.svelte';
  import { fetchEditHistory, type Revision } from '$lib/stores/editHistory';
  import { wordDiff } from '$lib/render/diff';
  import { ArrowLeftRight, AlignLeft } from '@lucide/svelte';

  type Props = {
    open: boolean;
    channelId: string;
    archiveId: string | null;
    onclose: () => void;
  };
  let { open, channelId, archiveId, onclose }: Props = $props();

  let loading = $state(false);
  let error = $state<string | null>(null);
  let revisions = $state<Revision[]>([]);
  let showDiff = $state(true);
  let token = 0;

  $effect(() => {
    if (!open || !archiveId) return;
    const id = archiveId;
    const mine = ++token;
    loading = true;
    error = null;
    revisions = [];
    showDiff = true;
    fetchEditHistory(channelId, id)
      .then((r) => {
        if (mine === token) revisions = r;
      })
      .catch((e) => {
        if (mine === token) error = e instanceof Error ? e.message : 'Chargement impossible';
      })
      .finally(() => {
        if (mine === token) loading = false;
      });
  });

  function fmt(at?: string): string {
    if (!at) return '';
    const d = new Date(at);
    return Number.isNaN(d.getTime())
      ? ''
      : d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
  }

  function label(rev: Revision, i: number): string {
    if (rev.original) return 'Version d’origine';
    if (i === revisions.length - 1) return 'Version actuelle';
    return `Révision ${i}`;
  }

  const view = $derived(
    revisions.map((rev, i) => ({
      rev,
      label: label(rev, i),
      diff: !rev.original && i > 0 ? wordDiff(revisions[i - 1].body, rev.body) : null
    }))
  );
  const hasEdits = $derived(revisions.length > 1);
</script>

<Modal {open} title="Historique des modifications" {onclose}>
  {#if loading}
    <div class="space-y-3" aria-busy="true">
      {#each [0, 1, 2] as n (n)}
        <div class="rounded-md border border-border bg-base/40 p-3">
          <div class="mb-2 h-3 w-24 animate-pulse rounded bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-elevated"></div>
        </div>
      {/each}
    </div>
  {:else if error}
    <p class="text-label text-danger">{error}</p>
  {:else if !hasEdits}
    <p class="text-body text-muted">Ce message n’a pas été modifié.</p>
  {:else}
    <div class="mb-3 flex items-center justify-between">
      <p class="text-label text-muted">{revisions.length} versions</p>
      <button
        type="button"
        onclick={() => (showDiff = !showDiff)}
        class="flex items-center gap-1.5 rounded-md px-2 py-1 text-label text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        {#if showDiff}
          <AlignLeft size={14} /> Voir les versions
        {:else}
          <ArrowLeftRight size={14} /> Voir les différences
        {/if}
      </button>
    </div>
    <ol class="max-h-[55vh] space-y-2 overflow-y-auto pr-1">
      {#each view as item, i (i)}
        <li class="rounded-md border border-border bg-base/40 p-3">
          <div class="mb-1.5 flex items-baseline justify-between gap-2">
            <span class="text-label font-medium text-content">{item.label}</span>
            {#if fmt(item.rev.at)}
              <time class="shrink-0 text-[0.6875rem] text-muted">{fmt(item.rev.at)}</time>
            {/if}
          </div>
          {#if showDiff && item.diff}
            <p class="whitespace-pre-wrap break-words text-body leading-relaxed">
              {#each item.diff as part, j (j)}
                {#if part.type === 'add'}
                  <span class="sr-only">ajouté&nbsp;</span><span
                    class="rounded-sm bg-success/15 text-success underline decoration-success/60">{part.text}</span>
                {:else if part.type === 'del'}
                  <span class="sr-only">supprimé&nbsp;</span><span
                    class="rounded-sm bg-danger/15 text-danger line-through">{part.text}</span>
                {:else}
                  <span class="text-content/90">{part.text}</span>
                {/if}
              {/each}
            </p>
          {:else}
            <p class="whitespace-pre-wrap break-words text-body text-content/90">{item.rev.body}</p>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</Modal>
