<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { Shield, Users, Flag, Ban, RotateCcw, Trash2, Award, Gavel } from '@lucide/svelte';
  import { Popover } from 'bits-ui';
  import { api, ApiError } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import { Button } from '$lib/ui';
  import { BADGE_LIST } from '$lib/badges';

  type AdminUser = {
    id: string;
    username: string;
    display_name: string | null;
    status: string | null;
    is_admin: boolean;
    disabled: boolean;
    badges?: string[] | null;
  };

  const BADGES = BADGE_LIST;

  async function toggleBadge(u: AdminUser, key: string) {
    const cur = new Set(u.badges ?? []);
    if (cur.has(key)) cur.delete(key);
    else cur.add(key);
    const next = [...cur];
    try {
      const res = await api<{ badges: string[] }>(`/api/admin/users/${u.id}/badges`, {
        method: 'PUT',
        body: { badges: next }
      });
      users = users.map((x) => (x.id === u.id ? { ...x, badges: res.badges } : x));
    } catch (e) {
      err = e instanceof ApiError ? e.message : 'Maj badges impossible';
    }
  }
  type AdminReport = {
    id: string;
    reporter_id: string;
    target_type: string;
    target_id: string;
    reason: string;
    status: string | null;
    created_at: string;
  };

  let tab = $state<'users' | 'reports'>('users');
  let users = $state<AdminUser[]>([]);
  let reports = $state<AdminReport[]>([]);
  let loading = $state(true);
  let err = $state<string | null>(null);
  let busyId = $state<string | null>(null);
  let confirmDelete = $state<string | null>(null);

  const meId = $derived($auth.user?.id);

  onMount(async () => {
    if ($auth.user && !$auth.user.is_admin) {
      void goto('/app');
      return;
    }
    await load();
  });

  async function load() {
    loading = true;
    err = null;
    try {
      const [allUsers, rep] = await Promise.all([
        api<AdminUser[]>('/api/admin/users'),
        api<AdminReport[]>('/api/admin/reports')
      ]);
      users = allUsers.filter((u) => !u.username.startsWith('deleted-'));
      reports = rep;
    } catch (e) {
      if (e instanceof ApiError && e.status === 403) void goto('/app');
      err = e instanceof Error ? e.message : 'Chargement impossible';
    } finally {
      loading = false;
    }
  }

  async function setDisabled(u: AdminUser, disabled: boolean) {
    busyId = u.id;
    try {
      const updated = await api<AdminUser>(`/api/admin/users/${u.id}`, {
        method: 'PATCH',
        body: { disabled }
      });
      users = users.map((x) => (x.id === u.id ? updated : x));
    } catch (e) {
      err = e instanceof Error ? e.message : 'Action impossible';
    } finally {
      busyId = null;
    }
  }

  async function setAdmin(u: AdminUser, isAdmin: boolean) {
    busyId = u.id;
    try {
      await api(`/api/admin/users/${u.id}/admin`, {
        method: 'PUT',
        body: { is_admin: isAdmin }
      });
      users = users.map((x) => (x.id === u.id ? { ...x, is_admin: isAdmin } : x));
    } catch (e) {
      err = e instanceof Error ? e.message : 'Action impossible';
    } finally {
      busyId = null;
    }
  }

  async function remove(u: AdminUser) {
    busyId = u.id;
    try {
      await api(`/api/admin/users/${u.id}`, { method: 'DELETE' });
      users = users.filter((x) => x.id !== u.id);
      confirmDelete = null;
    } catch (e) {
      err = e instanceof Error ? e.message : 'Suppression impossible';
    } finally {
      busyId = null;
    }
  }

  async function globalBan(u: AdminUser) {
    if (!window.confirm(`Bannir ${u.username} ? Son pseudo et son email seront bloqués et le compte supprimé.`)) return;
    busyId = u.id;
    try {
      await api(`/api/admin/users/${u.id}/global-ban`, { method: 'POST' });
      users = users.filter((x) => x.id !== u.id);
    } catch (e) {
      err = e instanceof Error ? e.message : 'Bannissement impossible';
    } finally {
      busyId = null;
    }
  }

  function initials(u: AdminUser): string {
    return (u.display_name || u.username).slice(0, 2).toUpperCase();
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

<div class="mx-auto max-w-4xl px-6 py-8">
  <div class="flex items-center gap-3">
    <div class="grid size-10 place-items-center rounded-lg bg-surface text-brand">
      <Shield size={22} />
    </div>
    <div>
      <h1 class="text-title font-bold leading-tight">Administration</h1>
      <p class="text-body text-muted">Gestion de la plateforme</p>
    </div>
  </div>

  <div class="mt-6 flex gap-1 border-b border-border">
    <button
      type="button"
      onclick={() => (tab = 'users')}
      class="-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-body transition-colors duration-150
             {tab === 'users' ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
    >
      <Users size={16} /> Utilisateurs
      <span class="rounded-full bg-elevated px-1.5 text-label text-muted">{users.length}</span>
    </button>
    <button
      type="button"
      onclick={() => (tab = 'reports')}
      class="-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-body transition-colors duration-150
             {tab === 'reports' ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
    >
      <Flag size={16} /> Signalements
      <span class="rounded-full bg-elevated px-1.5 text-label text-muted">{reports.length}</span>
    </button>
  </div>

  {#if err}
    <p class="mt-4 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-label text-danger">{err}</p>
  {/if}

  <div class="mt-6">
    {#if loading}
      {#each [0, 1, 2] as i (i)}
        <div class="mb-2 h-16 animate-pulse rounded-lg bg-surface"></div>
      {/each}
    {:else if tab === 'users'}
      <ul class="space-y-2">
        {#each users as u (u.id)}
          <li class="flex items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3">
            <div class="grid size-9 shrink-0 place-items-center rounded-full bg-elevated text-label font-semibold text-muted">
              {initials(u)}
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="truncate text-body font-medium {u.disabled ? 'text-muted line-through' : ''}">
                  {u.display_name || u.username}
                </span>
                {#if u.is_admin}
                  <span class="rounded-full bg-brand/15 px-2 py-0.5 text-[0.6875rem] font-medium text-accent">Admin</span>
                {/if}
                {#if u.disabled}
                  <span class="rounded-full bg-danger/10 px-2 py-0.5 text-[0.6875rem] font-medium text-danger">Désactivé</span>
                {/if}
              </div>
              <p class="truncate text-label text-muted">@{u.username}</p>
            </div>

            <div class="flex shrink-0 items-center gap-1.5">
              <Popover.Root>
                <Popover.Trigger
                  title="Badges"
                  class="grid size-8 place-items-center rounded-md text-muted transition-colors hover:bg-elevated hover:text-content"
                >
                  <Award size={15} />
                  {#if u.badges?.length}
                    <span class="ml-0.5 text-[0.625rem] tabular-nums">{u.badges.length}</span>
                  {/if}
                </Popover.Trigger>
                <Popover.Portal>
                  <Popover.Content
                    align="end"
                    sideOffset={4}
                    class="z-50 w-56 rounded-lg border border-border bg-overlay p-1.5 shadow-lg animate-fade-in"
                  >
                    <p class="px-1.5 py-1 text-label font-semibold uppercase tracking-wide text-muted">Badges de {u.username}</p>
                    {#each BADGES as b (b.key)}
                      {@const on = (u.badges ?? []).includes(b.key)}
                      <button
                        type="button"
                        onclick={() => toggleBadge(u, b.key)}
                        class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-body transition-colors hover:bg-elevated
                               {on ? 'text-content' : 'text-muted'}"
                      >
                        <img src={b.img} alt={b.label} class="size-4 object-contain" aria-hidden="true" />
                        <span class="flex-1">{b.label}</span>
                        {#if on}<span class="text-accent">✓</span>{/if}
                      </button>
                    {/each}
                  </Popover.Content>
                </Popover.Portal>
              </Popover.Root>

              {#if u.id !== meId}
                {#if u.disabled}
                  <Button size="sm" variant="secondary" loading={busyId === u.id} onclick={() => setDisabled(u, false)}>
                    <RotateCcw size={14} /> Réactiver
                  </Button>
                {:else}
                  <Button size="sm" variant="secondary" loading={busyId === u.id} onclick={() => setDisabled(u, true)}>
                    <Ban size={14} /> Désactiver
                  </Button>
                {/if}
                {#if u.is_admin}
                  <Button size="sm" variant="secondary" loading={busyId === u.id} onclick={() => setAdmin(u, false)}>
                    Retirer Admin
                  </Button>
                {:else}
                  <Button size="sm" variant="secondary" loading={busyId === u.id} onclick={() => setAdmin(u, true)}>
                    Promouvoir Admin
                  </Button>
                {/if}
                {#if confirmDelete === u.id}
                  <Button size="sm" variant="danger" loading={busyId === u.id} onclick={() => remove(u)}>
                    Confirmer
                  </Button>
                  <button type="button" class="px-1 text-label text-muted hover:text-content" onclick={() => (confirmDelete = null)}>Annuler</button>
                {:else}
                  <button
                    type="button"
                    title="Bannir (bloque pseudo + email)"
                    onclick={() => globalBan(u)}
                    class="grid size-8 place-items-center rounded-md text-danger transition-colors hover:bg-danger/10"
                  >
                    <Gavel size={15} />
                  </button>
                  <button
                    type="button"
                    title="Supprimer le compte"
                    onclick={() => (confirmDelete = u.id)}
                    class="grid size-8 place-items-center rounded-md text-danger transition-colors hover:bg-danger/10"
                  >
                    <Trash2 size={15} />
                  </button>
                {/if}
              {:else}
                <span class="shrink-0 text-label text-muted ml-2">toi</span>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <ul class="space-y-2">
        {#each reports as r (r.id)}
          <li class="rounded-lg border border-border bg-surface px-4 py-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-label text-muted">{r.target_type} · {fmtDate(r.created_at)}</span>
              <span
                class="rounded-full px-2 py-0.5 text-[0.6875rem] font-medium
                       {r.status === 'resolved'
                  ? 'bg-success/10 text-success'
                  : r.status === 'dismissed'
                    ? 'bg-elevated text-muted'
                    : 'bg-warning/10 text-warning'}"
              >
                {r.status ?? 'pending'}
              </span>
            </div>
            <p class="mt-1 whitespace-pre-wrap break-words text-body text-content/90">{r.reason}</p>
          </li>
        {:else}
          <div class="grid place-items-center rounded-lg border border-border py-16 text-muted">
            <Flag size={28} class="mb-3 opacity-60" />
            <p class="text-body">Aucun signalement.</p>
          </div>
        {/each}
      </ul>
    {/if}
  </div>
</div>
