<script lang="ts">
  import { onMount } from 'svelte';
  import { HardDrive, Trash2, FileText, Image, Film, Music } from '@lucide/svelte';
  import { api } from '$lib/api';

  type StoredFile = { id: string; filename: string; size: number; mimetype: string; kind: string; created_at: string };
  type Storage = { used: number; quota: number; files: StoredFile[] };

  let data = $state<Storage | null>(null);
  let loading = $state(true);
  let busyId = $state<string | null>(null);

  async function load() {
    loading = true;
    try {
      data = await api<Storage>('/api/me/storage');
    } catch {
      data = null;
    } finally {
      loading = false;
    }
  }
  onMount(load);

  function fmt(bytes: number): string {
    if (bytes < 1024) return `${bytes} o`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} Ko`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} Mo`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} Go`;
  }
  const pct = $derived(data ? Math.min(100, Math.round((data.used / data.quota) * 100)) : 0);

  function iconFor(mime: string) {
    if (mime.startsWith('image/')) return Image;
    if (mime.startsWith('video/')) return Film;
    if (mime.startsWith('audio/')) return Music;
    return FileText;
  }

  async function del(f: StoredFile) {
    if (!window.confirm(`Supprimer "${f.filename}" ?`)) return;
    busyId = f.id;
    try {
      await api(`/api/files/${f.id}`, { method: 'DELETE' });
      if (data) data = { ...data, used: data.used - f.size, files: data.files.filter((x) => x.id !== f.id) };
    } catch {
    } finally {
      busyId = null;
    }
  }
</script>

<div class="mx-auto max-w-3xl px-6 py-8">
  <h1 class="flex items-center gap-2 text-title font-bold"><HardDrive size={26} /> Stockage</h1>

  {#if loading}
    <p class="mt-6 text-body text-muted">Chargement…</p>
  {:else if !data}
    <p class="mt-6 text-body text-danger">Impossible de charger le stockage.</p>
  {:else}
    <section class="mt-6 rounded-lg border border-border p-5">
      <div class="flex items-baseline justify-between">
        <span class="text-body font-medium text-content">{fmt(data.used)} utilisés</span>
        <span class="text-label text-muted">sur {fmt(data.quota)}</span>
      </div>
      <div class="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-elevated">
        <div class="h-full rounded-full transition-all {pct > 90 ? 'bg-danger' : 'bg-primary'}" style="width:{pct}%"></div>
      </div>
      <p class="mt-1.5 text-label text-muted">{pct}% de ton espace est utilisé.</p>
    </section>

    <section class="mt-6">
      <h2 class="mb-3 text-subtitle font-semibold text-content">Tes fichiers ({data.files.length})</h2>
      {#if data.files.length === 0}
        <p class="text-body text-muted">Aucun fichier pour le moment.</p>
      {:else}
        <div class="divide-y divide-border rounded-lg border border-border">
          {#each data.files as f (f.id)}
            {@const Icon = iconFor(f.mimetype)}
            <div class="flex items-center gap-3 px-4 py-2.5">
              <Icon size={18} class="shrink-0 text-muted" />
              <div class="min-w-0 flex-1">
                <p class="truncate text-body text-content">{f.filename}</p>
                <p class="text-label text-muted">{fmt(f.size)} · {f.kind} · {new Date(f.created_at).toLocaleDateString('fr-FR')}</p>
              </div>
              <button type="button" onclick={() => del(f)} disabled={busyId === f.id}
                      class="grid size-8 shrink-0 place-items-center rounded-md text-muted transition-colors hover:bg-danger/10 hover:text-danger disabled:opacity-50">
                <Trash2 size={16} />
              </button>
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>
