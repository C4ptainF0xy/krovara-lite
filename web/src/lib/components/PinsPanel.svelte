<script lang="ts">
  import { Pin, X, CornerUpRight, StickyNote, Pencil } from '@lucide/svelte';
  import { pinsByChannel, unpinMessage, pinMessage, type Pin as PinType } from '$lib/stores/pins';
  import { memberNames } from '$lib/stores/members';
  import { renderMarkup } from '$lib/render/markup';

  type Props = {
    open: boolean;
    channelId: string;
    canManage?: boolean;
    onjump: (archiveId: string) => void;
    onclose: () => void;
  };
  let { open, channelId, canManage = false, onjump, onclose }: Props = $props();

  const pins = $derived<PinType[]>($pinsByChannel[channelId] ?? []);

  function authorName(id: string): string {
    return $memberNames[id] ?? (id ? id.slice(0, 8) : 'inconnu');
  }
  function fmt(iso?: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    return d.toLocaleDateString([], { day: 'numeric', month: 'short' }) + ' · ' +
      d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  let busy = $state<string | null>(null);
  async function unpin(archiveId: string) {
    busy = archiveId;
    try {
      await unpinMessage(channelId, archiveId);
    } catch (e) {
      console.error('unpin', e);
    } finally {
      busy = null;
    }
  }

  let editingNote = $state<string | null>(null);
  let noteDraft = $state('');
  function startNote(p: PinType) {
    editingNote = p.archive_id;
    noteDraft = p.note;
  }
  async function saveNote(archiveId: string) {
    try {
      await pinMessage(channelId, archiveId, noteDraft.trim());
      editingNote = null;
    } catch (e) {
      console.error('note', e);
    }
  }
</script>

{#if open}
  <button
    type="button"
    aria-label="Fermer les épingles"
    class="absolute inset-0 z-20 bg-base/40 backdrop-blur-[1px]"
    onclick={onclose}
  ></button>
  <aside
    class="absolute right-0 top-0 z-30 flex h-full w-80 max-w-[88%] flex-col border-l border-border bg-surface shadow-2xl shadow-black/40 animate-slide-in"
  >
    <header class="flex items-center gap-2 border-b border-border px-4 py-3">
      <Pin size={16} class="text-accent" />
      <h2 class="text-body font-semibold text-content">Messages épinglés</h2>
      <span class="text-label text-muted">{pins.length}</span>
      <button
        type="button"
        title="Fermer"
        onclick={onclose}
        class="ml-auto grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
      >
        <X size={16} />
      </button>
    </header>

    <div class="flex-1 space-y-2 overflow-y-auto p-3">
      {#each pins as p (p.archive_id)}
        <article class="rounded-lg border border-border bg-base/40 p-3">
          <div class="flex items-baseline gap-2">
            <span class="truncate text-label font-semibold text-content">{authorName(p.author_id)}</span>
            <span class="text-[0.6875rem] text-muted">{fmt(p.at)}</span>
          </div>
          {#if p.missing}
            <p class="mt-1 text-label italic text-muted">Ce message a été supprimé.</p>
          {:else}
            <div class="mt-1 line-clamp-4 break-words text-label text-content/90 [&_a]:break-all">
              {@html renderMarkup(p.body)}
            </div>
          {/if}
          {#if editingNote === p.archive_id}
            <div class="mt-2">
              <textarea
                bind:value={noteDraft}
                rows="2"
                maxlength="500"
                placeholder="Pourquoi ce message ?"
                class="w-full resize-y rounded border border-border bg-base/60 px-2 py-1 text-[0.6875rem] text-content outline-none focus:border-brand"
              ></textarea>
              <div class="mt-1 flex gap-3 text-[0.6875rem]">
                <button type="button" class="text-accent hover:underline" onclick={() => saveNote(p.archive_id)}>Enregistrer</button>
                <button type="button" class="text-muted hover:underline" onclick={() => (editingNote = null)}>annuler</button>
              </div>
            </div>
          {:else if p.note}
            <p class="group/note mt-2 flex items-start gap-1.5 rounded border border-border bg-elevated/40 px-2 py-1 text-[0.6875rem] text-muted">
              <StickyNote size={12} class="mt-0.5 shrink-0 text-accent" />
              <span class="break-words">{p.note}</span>
              {#if canManage}
                <button
                  type="button"
                  title="Modifier la note"
                  onclick={() => startNote(p)}
                  class="ml-auto shrink-0 opacity-0 transition-opacity group-hover/note:opacity-100 hover:text-content"
                >
                  <Pencil size={11} />
                </button>
              {/if}
            </p>
          {/if}
          <div class="mt-2 flex items-center gap-3 text-[0.6875rem]">
            {#if !p.missing}
              <button
                type="button"
                onclick={() => onjump(p.archive_id)}
                class="inline-flex items-center gap-1 text-accent transition-colors hover:underline"
              >
                <CornerUpRight size={12} /> Aller au message
              </button>
            {/if}
            {#if canManage && !p.note && editingNote !== p.archive_id}
              <button
                type="button"
                onclick={() => startNote(p)}
                class="inline-flex items-center gap-1 text-muted transition-colors hover:text-content"
              >
                <StickyNote size={12} /> Note
              </button>
            {/if}
            {#if canManage}
              <button
                type="button"
                onclick={() => unpin(p.archive_id)}
                disabled={busy === p.archive_id}
                class="text-muted transition-colors hover:text-danger disabled:opacity-50"
              >
                Désépingler
              </button>
            {/if}
          </div>
        </article>
      {:else}
        <div class="grid h-full place-items-center px-6 text-center">
          <div class="text-muted">
            <div class="mx-auto mb-3 grid size-11 place-items-center rounded-2xl bg-elevated">
              <Pin size={20} />
            </div>
            <p class="text-body">Aucun message épinglé.</p>
            <p class="text-label">Survole un message et clique sur l'épingle.</p>
          </div>
        </div>
      {/each}
    </div>
  </aside>
{/if}
