<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { ShieldAlert, Check, X, Flag, Tag } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { memberNames, loadMembers } from '$lib/stores/members';
  import {
    claimReport,
    refreshPendingReports,
    listReportComments,
    addReportComment,
    type ReportComment
  } from '$lib/stores/reports';
  import { auth } from '$lib/stores/auth';
  import { Button } from '$lib/ui';
  import { MessageSquare, Send } from '@lucide/svelte';

  const selfId = $derived($auth.user?.id ?? '');

  type ContextMsg = {
    id: string;
    author?: string;
    author_id?: string;
    body: string;
    reported?: boolean;
  };

  type Report = {
    id: string;
    reporter_id: string;
    target_type: string;
    target_id: string;
    reason: string;
    status: string | null;
    created_at: string;
    channel_id?: string;
    category?: string;
    context?: ContextMsg[];
    assigned_to?: string;
  };

  const CATEGORY_LABELS: Record<string, string> = {
    spam: 'Spam',
    harassment: 'Harcèlement',
    illegal: 'Contenu illégal',
    nsfw: 'Contenu explicite',
    other: 'Autre'
  };
  function categoryLabel(c?: string): string {
    return c ? (CATEGORY_LABELS[c] ?? c) : '';
  }

  const spaceId = $derived(page.params.id ?? '');

  let reports = $state<Report[]>([]);
  let loading = $state(true);
  let err = $state<string | null>(null);
  let busyId = $state<string | null>(null);
  let filter = $state<'pending' | 'all'>('pending');

  async function load() {
    loading = true;
    err = null;
    try {
      await loadMembers(spaceId).catch(() => {});
      const all = await api<Report[]>(`/api/spaces/${spaceId}/reports`);
      reports =
        filter === 'pending'
          ? all.filter((r) => !r.status || r.status === 'pending' || r.status === 'in_progress')
          : all;
    } catch (e) {
      err = e instanceof Error ? e.message : 'Chargement impossible';
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function resolve(r: Report, status: 'resolved' | 'dismissed') {
    busyId = r.id;
    try {
      await api(`/api/spaces/${spaceId}/reports/${r.id}`, {
        method: 'PATCH',
        body: { status }
      });
      if (filter === 'pending') {
        reports = reports.filter((x) => x.id !== r.id);
      } else {
        reports = reports.map((x) => (x.id === r.id ? { ...x, status } : x));
      }
      void refreshPendingReports(spaceId);
    } catch (e) {
      err = e instanceof Error ? e.message : 'Action impossible';
    } finally {
      busyId = null;
    }
  }

  async function claim(r: Report) {
    busyId = r.id;
    try {
      await claimReport(spaceId, r.id);
      reports = reports.map((x) =>
        x.id === r.id ? { ...x, status: 'in_progress', assigned_to: selfId } : x
      );
      void refreshPendingReports(spaceId);
    } catch (e) {
      err = e instanceof Error ? e.message : 'Action impossible';
    } finally {
      busyId = null;
    }
  }

  let selected = $state<Set<string>>(new Set());
  let bulkBusy = $state(false);
  const pendingReports = $derived(reports.filter((r) => !r.status || r.status === 'pending'));

  function toggleSelect(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }
  function selectAllPending() {
    selected = new Set(pendingReports.map((r) => r.id));
  }
  function clearSelection() {
    selected = new Set();
  }

  async function bulkResolve(status: 'resolved' | 'dismissed') {
    const ids = [...selected];
    if (ids.length === 0 || bulkBusy) return;
    bulkBusy = true;
    try {
      await api(`/api/spaces/${spaceId}/reports/bulk`, { method: 'POST', body: { ids, status } });
      if (filter === 'pending') {
        reports = reports.filter((x) => !selected.has(x.id));
      } else {
        reports = reports.map((x) => (selected.has(x.id) ? { ...x, status } : x));
      }
      clearSelection();
    } catch (e) {
      err = e instanceof Error ? e.message : 'Action groupée impossible';
    } finally {
      bulkBusy = false;
    }
  }

  let openThread = $state<string | null>(null);
  let comments = $state<Record<string, ReportComment[]>>({});
  let commentDraft = $state<Record<string, string>>({});
  let commentBusy = $state<string | null>(null);
  let commentsLoading = $state<string | null>(null);

  async function toggleThread(r: Report) {
    if (openThread === r.id) {
      openThread = null;
      return;
    }
    openThread = r.id;
    if (comments[r.id] === undefined) {
      commentsLoading = r.id;
      try {
        comments = { ...comments, [r.id]: await listReportComments(spaceId, r.id) };
      } catch (e) {
        err = e instanceof Error ? e.message : 'Discussion indisponible';
      } finally {
        commentsLoading = null;
      }
    }
  }

  async function sendComment(r: Report) {
    const body = (commentDraft[r.id] ?? '').trim();
    if (!body || commentBusy === r.id) return;
    commentBusy = r.id;
    try {
      const c = await addReportComment(spaceId, r.id, body);
      comments = { ...comments, [r.id]: [...(comments[r.id] ?? []), c] };
      commentDraft = { ...commentDraft, [r.id]: '' };
    } catch (e) {
      err = e instanceof Error ? e.message : 'Envoi impossible';
    } finally {
      commentBusy = null;
    }
  }

  function nameOf(id: string): string {
    return $memberNames[id] ?? 'Inconnu';
  }
  function fmtDate(s: string): string {
    return new Date(s).toLocaleString([], {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<div class="mx-auto max-w-3xl px-6 py-8">
  <div class="flex items-center gap-3">
    <div class="grid size-10 place-items-center rounded-lg bg-surface text-brand">
      <ShieldAlert size={22} />
    </div>
    <div>
      <h1 class="text-title font-bold leading-tight">Modération</h1>
      <p class="text-body text-muted">Signalements de cet espace</p>
    </div>
  </div>

  <div class="mt-6 flex gap-1 border-b border-border">
    {#each [['pending', 'En attente'], ['all', 'Tous']] as [val, label] (val)}
      <button
        type="button"
        onclick={() => {
          filter = val as 'pending' | 'all';
          load();
        }}
        class="-mb-px border-b-2 px-3 py-2.5 text-body transition-colors duration-150
               {filter === val ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
      >
        {label}
      </button>
    {/each}
  </div>

  {#if pendingReports.length > 0 && !loading}
    <div class="mt-4 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface px-3 py-2">
      {#if selected.size === 0}
        <button type="button" onclick={selectAllPending} class="text-label text-muted transition-colors hover:text-content">
          Tout sélectionner ({pendingReports.length})
        </button>
      {:else}
        <span class="text-label font-medium text-content">{selected.size} sélectionné{selected.size > 1 ? 's' : ''}</span>
        <button type="button" onclick={clearSelection} class="text-label text-muted transition-colors hover:text-content">Désélectionner</button>
        <div class="ml-auto flex gap-2">
          <Button size="sm" loading={bulkBusy} onclick={() => bulkResolve('resolved')}>
            <Check size={15} /> Résoudre la sélection
          </Button>
          <Button size="sm" variant="secondary" disabled={bulkBusy} onclick={() => bulkResolve('dismissed')}>
            <X size={15} /> Rejeter
          </Button>
        </div>
      {/if}
    </div>
  {/if}

  <div class="mt-6 space-y-3">
    {#if loading}
      {#each [0, 1] as i (i)}
        <div class="h-24 animate-pulse rounded-lg bg-surface"></div>
      {/each}
    {:else if err}
      <p class="rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-label text-danger">{err}</p>
    {:else if reports.length === 0}
      <div class="grid place-items-center rounded-lg border border-border py-16 text-center text-muted">
        <Flag size={28} class="mb-3 opacity-60" />
        <p class="text-body">Aucun signalement {filter === 'pending' ? 'en attente' : ''}.</p>
      </div>
    {:else}
      {#each reports as r (r.id)}
        <article class="rounded-lg border bg-surface p-4 transition-colors {selected.has(r.id) ? 'border-brand/60' : 'border-border'}">
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 gap-3">
              {#if !r.status || r.status === 'pending'}
                <input
                  type="checkbox"
                  checked={selected.has(r.id)}
                  onchange={() => toggleSelect(r.id)}
                  aria-label="Sélectionner ce signalement"
                  class="mt-1 size-4 shrink-0 accent-primary"
                />
              {/if}
              <div class="min-w-0">
              <p class="text-body">
                <span class="font-semibold text-content">{nameOf(r.reporter_id)}</span>
                <span class="text-muted"> a signalé </span>
                <span class="font-medium text-content">{nameOf(r.target_id)}</span>
              </p>
              <p class="mt-1 whitespace-pre-wrap break-words text-body text-content/90">{r.reason}</p>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                {#if r.category}
                  <span
                    class="inline-flex items-center gap-1 rounded-full bg-elevated px-2 py-0.5 text-label text-muted"
                  >
                    <Tag size={12} /> {categoryLabel(r.category)}
                  </span>
                {/if}
                <span class="text-label text-muted">{fmtDate(r.created_at)}</span>
              </div>
              {#if r.context?.length}
                <details class="mt-2">
                  <summary class="cursor-pointer text-label text-muted transition-colors hover:text-content">
                    Contexte joint ({r.context.length} messages)
                  </summary>
                  <div class="mt-2 space-y-1 rounded-md border border-border bg-base/40 p-2">
                    {#each r.context as c (c.id)}
                      <p
                        class="break-words text-label {c.reported
                          ? 'rounded bg-danger/10 px-1.5 py-0.5 text-content'
                          : 'text-muted'}"
                      >
                        <span class="font-medium text-content/80"
                          >{$memberNames[c.author_id ?? ''] ?? c.author ?? 'Inconnu'}</span
                        >
                        · {c.body}
                      </p>
                    {/each}
                  </div>
                </details>
              {/if}
              </div>
            </div>
            {#if r.status === 'in_progress'}
              <span class="shrink-0 rounded-full bg-warning/10 px-2.5 py-1 text-label text-warning">
                En cours{#if r.assigned_to} · {nameOf(r.assigned_to)}{/if}
              </span>
            {:else if r.status && r.status !== 'pending'}
              <span
                class="shrink-0 rounded-full px-2.5 py-1 text-label
                       {r.status === 'resolved' ? 'bg-success/10 text-success' : 'bg-elevated text-muted'}"
              >
                {r.status === 'resolved' ? 'Résolu' : 'Rejeté'}
              </span>
            {/if}
          </div>

          {#if !r.status || r.status === 'pending' || r.status === 'in_progress'}
            <div class="mt-3 flex gap-2">
              {#if (!r.status || r.status === 'pending')}
                <Button size="sm" variant="secondary" disabled={busyId === r.id} onclick={() => claim(r)}>
                  <Flag size={15} /> Prendre en charge
                </Button>
              {/if}
              <Button size="sm" loading={busyId === r.id} onclick={() => resolve(r, 'resolved')}>
                <Check size={15} /> Résoudre
              </Button>
              <Button
                size="sm"
                variant="secondary"
                disabled={busyId === r.id}
                onclick={() => resolve(r, 'dismissed')}
              >
                <X size={15} /> Rejeter
              </Button>
            </div>
          {/if}

          <div class="mt-3 border-t border-border/60 pt-2">
            <button
              type="button"
              onclick={() => toggleThread(r)}
              class="flex items-center gap-1.5 text-label text-muted transition-colors hover:text-content"
            >
              <MessageSquare size={14} />
              Discussion interne{#if comments[r.id]?.length}
                <span class="tabular-nums">({comments[r.id].length})</span>
              {/if}
            </button>
            {#if openThread === r.id}
              <div class="mt-2 space-y-2">
                {#if commentsLoading === r.id}
                  <div class="h-8 animate-pulse rounded bg-base/60"></div>
                {:else if (comments[r.id]?.length ?? 0) === 0}
                  <p class="text-label text-muted/70">Aucune note. Lance la discussion entre modos.</p>
                {:else}
                  {#each comments[r.id] as c (c.id)}
                    <div class="rounded-md border border-border bg-base/40 px-2.5 py-1.5">
                      <p class="flex items-baseline gap-2 text-label">
                        <span class="font-medium text-content">{nameOf(c.author_id ?? '')}</span>
                        <span class="text-[0.6875rem] text-muted">{fmtDate(c.created_at)}</span>
                      </p>
                      <p class="mt-0.5 whitespace-pre-wrap break-words text-label text-content/90">{c.body}</p>
                    </div>
                  {/each}
                {/if}
                <form
                  class="flex items-end gap-2"
                  onsubmit={(e) => {
                    e.preventDefault();
                    sendComment(r);
                  }}
                >
                  <input
                    bind:value={commentDraft[r.id]}
                    placeholder="Note interne…"
                    maxlength={4000}
                    class="h-9 flex-1 rounded-md border border-border bg-base/50 px-3 text-label text-content outline-none focus:border-primary"
                  />
                  <Button size="sm" type="submit" loading={commentBusy === r.id} disabled={!(commentDraft[r.id] ?? '').trim()}>
                    <Send size={14} />
                  </Button>
                </form>
              </div>
            {/if}
          </div>
        </article>
      {/each}
    {/if}
  </div>
</div>
