<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import {
    registerThisBrowser,
    requestPermission,
    subscribe,
    type Device
  } from '$lib/push/ntfy';
  import { spaces, loadSpaces } from '$lib/stores/spaces';
  import { getDefaultJoinNotif, setDefaultJoinNotif, type NotifLevel } from '$lib/stores/inbox';
  import { t } from '$lib/i18n';
  import { Button } from '$lib/ui';

  type Pref = { space_id: string; scope: 'all' | 'mentions' | 'none' };

  let devices = $state<Device[]>([]);
  let prefs = $state<Record<string, Pref['scope']>>({});
  let loading = $state(true);
  let err = $state<string | null>(null);
  let notifPerm = $state<NotificationPermission>('default');
  let busy = $state(false);

  let defaultJoinNotif = $state<NotifLevel>(getDefaultJoinNotif());
  const JOIN_LEVELS: { v: NotifLevel; l: string }[] = [
    { v: 'all', l: 'Tous les messages' },
    { v: 'mentions', l: 'Mentions seulement' },
    { v: 'nothing', l: 'Rien' }
  ];
  function pickDefaultJoinNotif(v: NotifLevel) {
    defaultJoinNotif = v;
    setDefaultJoinNotif(v);
  }

  onMount(async () => {
    if (typeof Notification !== 'undefined') notifPerm = Notification.permission;
    await loadSpaces();
    await refresh();
  });

  async function refresh() {
    loading = true;
    err = null;
    try {
      devices = await api<Device[]>('/api/me/devices');
      const list = await api<Pref[]>('/api/me/push-prefs');
      prefs = Object.fromEntries(list.map((p) => [p.space_id, p.scope]));
      if (devices[0]) subscribe(devices[0].ntfy_topic);
    } catch (e) {
      err = e instanceof Error ? e.message : 'load failed';
    } finally {
      loading = false;
    }
  }

  async function addThisBrowser() {
    busy = true;
    err = null;
    try {
      notifPerm = await requestPermission();
      const dev = await registerThisBrowser();
      devices = [...devices, dev];
      subscribe(dev.ntfy_topic);
    } catch (e) {
      err = e instanceof Error ? e.message : 'register failed';
    } finally {
      busy = false;
    }
  }

  async function removeDevice(id: string) {
    busy = true;
    try {
      await api(`/api/me/devices/${id}`, { method: 'DELETE' });
      devices = devices.filter((d) => d.id !== id);
    } catch (e) {
      err = e instanceof Error ? e.message : 'delete failed';
    } finally {
      busy = false;
    }
  }

  async function setPref(spaceId: string, scope: Pref['scope']) {
    prefs = { ...prefs, [spaceId]: scope };
    try {
      await api(`/api/spaces/${spaceId}/push-pref`, {
        method: 'PUT',
        body: { scope }
      });
    } catch (e) {
      err = e instanceof Error ? e.message : 'save failed';
    }
  }
</script>

{#if err}
  <p class="mb-4 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-label text-danger">{err}</p>
{/if}

<section class="mb-8">
  <h2 class="text-label font-semibold uppercase tracking-wide text-muted">Nouveaux serveurs</h2>
  <p class="mt-1 text-body text-muted">
    Niveau de notification appliqué automatiquement quand tu rejoins un serveur.
    Tu peux toujours l'ajuster ensuite par serveur (clic-droit sur l'icône).
  </p>
  <div class="mt-3 inline-flex rounded-lg border border-border p-1">
    {#each JOIN_LEVELS as o (o.v)}
      <button
        type="button"
        onclick={() => pickDefaultJoinNotif(o.v)}
        class="rounded px-4 py-1.5 text-label font-medium transition-colors duration-150 ease-smooth
               {defaultJoinNotif === o.v ? 'bg-primary text-white' : 'text-muted hover:text-content'}"
      >
        {o.l}
      </button>
    {/each}
  </div>
</section>

<section>
  <h2 class="text-label font-semibold uppercase tracking-wide text-muted">{$t('devices.title')}</h2>
  <p class="mt-1 text-body text-muted">
    {$t('devices.hint')}
    {#if notifPerm !== 'granted'}
      <span class="text-warning">{$t('devices.permission', { perm: notifPerm })}</span>
    {/if}
  </p>

  <div class="mt-3 space-y-2">
    {#if loading}
      <div class="h-12 animate-pulse rounded-lg bg-surface"></div>
    {:else if devices.length === 0}
      <p class="text-body text-muted">{$t('devices.empty')}</p>
    {/if}
    {#each devices as d (d.id)}
      <div class="flex items-center justify-between rounded-lg border border-border bg-surface px-3 py-2.5">
        <div class="min-w-0">
          <p class="truncate text-body font-medium">{d.name}</p>
          <p class="truncate font-mono text-label text-muted">{d.ntfy_topic}</p>
        </div>
        <button
          type="button"
          disabled={busy}
          onclick={() => removeDevice(d.id)}
          class="shrink-0 text-label text-danger transition-colors hover:brightness-125 disabled:opacity-50"
        >
          {$t('devices.remove')}
        </button>
      </div>
    {/each}
  </div>

  <Button class="mt-3" loading={busy} onclick={addThisBrowser}>{$t('devices.addBrowser')}</Button>
</section>

<section class="mt-10">
  <h2 class="text-label font-semibold uppercase tracking-wide text-muted">{$t('devices.perSpace.title')}</h2>
  <p class="mt-1 text-body text-muted">{$t('devices.perSpace.hint')}</p>
  <ul class="mt-3 divide-y divide-border overflow-hidden rounded-lg border border-border">
    {#each $spaces.data as space (space.id)}
      <li class="flex items-center justify-between bg-surface px-3 py-2.5">
        <span class="text-body">{space.name}</span>
        <select
          value={prefs[space.id] ?? 'all'}
          onchange={(e) => setPref(space.id, (e.target as HTMLSelectElement).value as Pref['scope'])}
          class="rounded border border-border bg-base px-2 py-1.5 text-label text-content outline-none focus:border-brand"
        >
          <option value="all">{$t('devices.level.all')}</option>
          <option value="mentions">{$t('devices.level.mentions')}</option>
          <option value="none">{$t('devices.level.none')}</option>
        </select>
      </li>
    {/each}
  </ul>
</section>
