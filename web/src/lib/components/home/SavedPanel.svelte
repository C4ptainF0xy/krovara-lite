<script lang="ts">
  import { onMount } from 'svelte';
  import { Bookmark, Hash, Trash2, ExternalLink, Folder } from '@lucide/svelte';
  import { saved, reloadSaves, unsaveMessage, type SavedMessage } from '$lib/stores/saves';
  import { memberNames } from '$lib/stores/members';
  import { renderMarkup } from '$lib/render/markup';

  let loading = $state(true);
  let err = $state<string | null>(null);

  onMount(async () => {
    try {
      await reloadSaves();
    } catch (e) {
      err = e instanceof Error ? e.message : 'Chargement impossible';
    } finally {
      loading = false;
    }
  });

  const groups = $derived.by(() => {
    const by = new Map<string, SavedMessage[]>();
    for (const s of $saved) {
      const k = s.folder || '';
      (by.get(k) ?? by.set(k, []).get(k)!).push(s);
    }
    return [...by.entries()].sort((a, b) => {
      if (a[0] === '') return 1;
      if (b[0] === '') return -1;
      return a[0].localeCompare(b[0]);
    });
  });

  function authorName(id: string): string {
    return $memberNames[id] ?? (id ? id.slice(0, 8) : 'inconnu');
  }
  function fmt(iso?: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    return (
      d.toLocaleDateString([], { day: 'numeric', month: 'short', year: 'numeric' }) +
      ' · ' +
      d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    );
  }

  let busy = $state<string | null>(null);
  async function remove(id: string) {
    busy = id;
    try {
      await unsaveMessage(id);
    } catch (e) {
      console.error('unsave', e);
    } finally {
      busy = null;
    }
  }
</script>

<div class="mx-auto max-w-2xl p-6 md:p-8">
  <header class="flex items-center gap-3">
    <div class="grid size-10 shrink-0 place-items-center rounded-xl bg-surface text-accent">
      <Bookmark size={20} />
    </div>
    <div>
      <h1 class="text-subtitle font-semibold text-content">Messages enregistrés</h1>
      <p class="text-label text-muted">Tes favoris, rangés par dossier.</p>
    </div>
  </header>

  {#if loading}
    <div class="mt-8 space-y-3">
      {#each [0, 1, 2] as i (i)}
        <div class="h-20 animate-pulse rounded-lg border border-border bg-surface/50"></div>
      {/each}
    </div>
  {:else if err}
    <p class="mt-8 text-label text-danger">{err}</p>
  {:else if $saved.length === 0}
    <div class="mt-16 grid place-items-center text-center text-muted">
      <div class="mb-3 grid size-12 place-items-center rounded-2xl bg-surface">
        <Bookmark size={22} />
      </div>
      <p class="text-body">Aucun message enregistré.</p>
      <p class="text-label">Survole un message et clique sur le marque-page pour le garder ici.</p>
    </div>
  {:else}
    <div class="mt-8 space-y-8">
      {#each groups as [folder, items] (folder)}
        <section>
          <h2 class="mb-2 flex items-center gap-1.5 text-label font-semibold uppercase tracking-wide text-muted">
            <Folder size={13} />
            {folder || 'Sans dossier'}
            <span class="text-muted/60">· {items.length}</span>
          </h2>
          <ul class="space-y-2">
            {#each items as s (s.archive_id)}
              <li class="group rounded-lg border border-border bg-surface/60 p-3 transition-colors duration-150 hover:border-border-strong">
                <div class="flex items-baseline gap-2">
                  <span class="truncate text-label font-semibold text-content">{authorName(s.author_id)}</span>
                  <span class="text-[0.6875rem] text-muted">{fmt(s.at)}</span>
                  <button
                    type="button"
                    title="Retirer des favoris"
                    onclick={() => remove(s.archive_id)}
                    disabled={busy === s.archive_id}
                    class="ml-auto grid size-7 place-items-center rounded text-muted opacity-0 transition group-hover:opacity-100 hover:bg-elevated hover:text-danger disabled:opacity-50"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
                {#if s.missing}
                  <p class="mt-1 text-label italic text-muted">Ce message a été supprimé.</p>
                {:else}
                  <div class="mt-1 line-clamp-5 break-words text-body text-content/90 [&_a]:break-all">
                    {@html renderMarkup(s.body)}
                  </div>
                {/if}
                {#if s.space_id}
                  <a
                    href={`/app/spaces/${s.space_id}/channels/${s.channel_id}`}
                    class="mt-2 inline-flex items-center gap-1 text-[0.6875rem] text-accent transition-colors hover:underline"
                  >
                    <Hash size={11} /> Aller au salon <ExternalLink size={11} />
                  </a>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  {/if}
</div>
