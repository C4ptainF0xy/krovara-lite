<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { api, authedObjectURL } from '$lib/api';
  import { t } from '$lib/i18n';
  import { auth, clearSession } from '$lib/stores/auth';
  import {
    spaces,
    channelsBySpace,
    categoriesBySpace,
    loadSpaces,
    loadChannels,
    loadCategories,
    createSpace,
    createChannel,
    createCategory,
    leaveSpace
  } from '$lib/stores/spaces';
  import { startMessageIngest } from '$lib/stores/messages';
  import { startDmIngest, dmUnread } from '$lib/stores/dm';
  import { incoming } from '$lib/stores/friends';
  import { startPresenceIngest } from '$lib/stores/presence';
  import { startStatusIngest } from '$lib/stores/status';
  import { startReactionIngest } from '$lib/stores/reactions';
  import { startReadIngest } from '$lib/stores/reads';
  import { inboxUnread, refreshInboxUnread, loadInbox, mentionsByChannel, setNotifSetting, type NotifLevel } from '$lib/stores/inbox';
  import { pendingReports, refreshPendingReports } from '$lib/stores/reports';
  import { unreadByChannel, loadReadState } from '$lib/stores/readstate';
  import { startTypingIngest } from '$lib/stores/typing';
  import '$lib/xmpp/presence';
  import { start as startXMPP, stop as stopXMPP, xmppState } from '$lib/xmpp/client';
  import { joinSpacePresence } from '$lib/xmpp/muc';
  import { subscribe as subscribePush, type Device } from '$lib/push/ntfy';
  import { joinVoice } from '$lib/voip/room';
  import { Popover, ContextMenu } from 'bits-ui';
  import SpaceIcon from '$lib/components/SpaceIcon.svelte';
  import ChannelList from '$lib/components/ChannelList.svelte';
  import BgContextMenu from '$lib/components/BgContextMenu.svelte';
  import ChannelSettingsModal from '$lib/components/ChannelSettingsModal.svelte';
  import QuickSwitcher from '$lib/components/QuickSwitcher.svelte';
  import ShortcutsHelp from '$lib/components/ShortcutsHelp.svelte';
  import LiveRegion from '$lib/components/LiveRegion.svelte';
  import type { Channel } from '$lib/stores/spaces';
  import { ResizeHandle } from '$lib/ui';
  import { longpress } from '$lib/actions/longpress';
  import {
    layout,
    focusMode,
    setChannelSidebarWidth,
    setMembersPanelWidth,
    toggleMembersPanel,
    toggleFocusMode,
    CHANNEL_SIDEBAR_MIN,
    CHANNEL_SIDEBAR_MAX,
    MEMBERS_PANEL_MIN,
    MEMBERS_PANEL_MAX
  } from '$lib/stores/layout';
  import MembersPanel from '$lib/components/MembersPanel.svelte';
  import CallPanel from '$lib/components/CallPanel.svelte';
  import VerifyEmailGate from '$lib/components/VerifyEmailGate.svelte';
  import Modal from '$lib/components/Modal.svelte';
  import { Button, Input } from '$lib/ui';
  import {
    Plus,
    Search,
    UserPlus,
    ShieldAlert,
    ListChecks,
    CalendarDays,
    Settings,
    LogOut,
    Bell,
    Menu,
    Check,
    Copy,
    Trash2,
    Users,
    Home,
    Compass,
    Shield,
    Maximize2,
    Minimize2,
    MoreHorizontal,
    Hash,
    FolderPlus,
    Columns2,
    X
  } from '@lucide/svelte';
  import { onDestroy } from 'svelte';

  let { children } = $props();

  let drawerOpen = $state(false);
  let membersDrawerOpen = $state(false);

  let touchStartX = 0;
  let touchStartY = 0;
  let touchTracking = false;
  const EDGE_ZONE = 28;
  const SWIPE_MIN = 60;

  function onTouchStart(e: TouchEvent) {
    const t = e.touches[0];
    touchStartX = t.clientX;
    touchStartY = t.clientY;
    const fromRightEdge = t.clientX >= window.innerWidth - EDGE_ZONE;
    touchTracking = drawerOpen || membersDrawerOpen || t.clientX <= EDGE_ZONE || fromRightEdge;
  }
  function onTouchEnd(e: TouchEvent) {
    if (!touchTracking) return;
    touchTracking = false;
    const t = e.changedTouches[0];
    const dx = t.clientX - touchStartX;
    const dy = t.clientY - touchStartY;
    if (Math.abs(dx) < SWIPE_MIN || Math.abs(dx) <= Math.abs(dy)) return;
    if (dx > 0) {
      if (membersDrawerOpen) membersDrawerOpen = false;
      else if (!drawerOpen) drawerOpen = true;
    } else {
      if (drawerOpen) drawerOpen = false;
      else if (!membersDrawerOpen && activeSpaceId) membersDrawerOpen = true;
    }
  }

  let spaceModal = $state(false);
  let spaceModalTab = $state<'create' | 'join'>('create');
  let joinCode = $state('');
  let channelModal = $state(false);
  let inviteModal = $state(false);
  let newSpaceName = $state('');
  let newChannelName = $state('');
  let newChannelType = $state<'text' | 'voice'>('text');
  let newChannelCategory = $state<string | null>(null);
  let modalBusy = $state(false);
  let modalErr = $state<string | null>(null);

  let creatingCategory = $state(false);
  let newCategoryName = $state('');

  let settingsChannel = $state<Channel | null>(null);

  function openChannelModal(categoryId: string | null) {
    modalErr = null;
    newChannelName = '';
    newChannelType = 'text';
    newChannelCategory = categoryId;
    channelModal = true;
  }

  async function submitCategory() {
    if (!activeSpaceId) return;
    const name = newCategoryName.trim();
    if (!name) {
      creatingCategory = false;
      return;
    }
    try {
      await createCategory(activeSpaceId, name);
      newCategoryName = '';
      creatingCategory = false;
    } catch {
    }
  }

  let inviteCode = $state<string | null>(null);
  let inviteCopied = $state(false);

  let inviteMaxUses = $state(50);
  let inviteTtl = $state(604800);
  const INVITE_USES = [
    { v: 0, label: 'Illimité' },
    { v: 1, label: '1' },
    { v: 10, label: '10' },
    { v: 50, label: '50' },
    { v: 100, label: '100' }
  ];
  const INVITE_TTL = [
    { v: 0, label: 'Jamais' },
    { v: 1800, label: '30 min' },
    { v: 86400, label: '1 jour' },
    { v: 604800, label: '7 jours' },
    { v: 2592000, label: '30 jours' }
  ];

  type ActiveInvite = {
    code: string;
    max_uses: number | null;
    uses: number | null;
    expires_at: string | null;
  };
  let activeInvites = $state<ActiveInvite[]>([]);

  function openInvite() {
    modalErr = null;
    inviteCode = null;
    inviteCopied = false;
    inviteModal = true;
    void loadInvites();
  }

  let chanBg = $state({ open: false, x: 0, y: 0 });
  const chanBgItems = $derived([
    { label: $t('nav.newChannel'), icon: Hash, onclick: () => openChannelModal(null) },
    {
      label: 'Créer une catégorie',
      icon: FolderPlus,
      onclick: () => {
        creatingCategory = true;
        newCategoryName = '';
      }
    },
    { label: $t('nav.invite'), icon: UserPlus, onclick: openInvite }
  ]);
  function onChanBg(e: MouseEvent) {
    if (e.defaultPrevented) return;
    e.preventDefault();
    chanBg = { open: true, x: e.clientX, y: e.clientY };
  }

  let railBg = $state({ open: false, x: 0, y: 0 });
  const railBgItems = $derived([
    { label: 'Accueil', icon: Home, onclick: () => goto('/app') },
    { label: $t('nav.newSpace'), icon: Plus, onclick: () => { modalErr = null; spaceModal = true; } },
    { label: $t('nav.explore'), icon: Compass, onclick: () => goto('/app/discover') },
    { label: $t('nav.settings'), icon: Settings, onclick: () => goto('/app/settings/profile') }
  ]);
  function onRailBg(e: MouseEvent) {
    if (e.defaultPrevented) return;
    e.preventDefault();
    railBg = { open: true, x: e.clientX, y: e.clientY };
  }

  async function loadInvites() {
    if (!activeSpaceId) return;
    try {
      activeInvites = await api<ActiveInvite[]>(`/api/spaces/${activeSpaceId}/invites`);
    } catch {
      activeInvites = [];
    }
  }

  async function revokeInvite(code: string) {
    try {
      await api(`/api/invites/${code}`, { method: 'DELETE' });
      activeInvites = activeInvites.filter((i) => i.code !== code);
    } catch (e) {
      modalErr = e instanceof Error ? e.message : 'Révocation impossible';
    }
  }

  function inviteStat(i: ActiveInvite): string {
    const used = i.max_uses && i.max_uses > 0 ? `${i.uses ?? 0}/${i.max_uses}` : `${i.uses ?? 0}`;
    const exp = i.expires_at ? ` · expire le ${new Date(i.expires_at).toLocaleDateString()}` : '';
    return `${used} utilisation${(i.uses ?? 0) > 1 ? 's' : ''}${exp}`;
  }

  async function generateInvite() {
    if (!activeSpaceId) return;
    modalErr = null;
    modalBusy = true;
    inviteCode = null;
    inviteCopied = false;
    try {
      const body: Record<string, number> = {};
      if (inviteMaxUses > 0) body.max_uses = inviteMaxUses;
      if (inviteTtl > 0) body.ttl_seconds = inviteTtl;
      const inv = await api<{ code: string }>(
        `/api/spaces/${activeSpaceId}/invites`,
        { method: 'POST', body }
      );
      inviteCode = inv.code;
    } catch (err) {
      modalErr = err instanceof Error ? err.message : 'failed';
    } finally {
      modalBusy = false;
    }
  }

  function inviteLink(): string {
    if (!inviteCode) return '';
    return `${window.location.origin}/join/${inviteCode}`;
  }

  async function copyInvite() {
    if (!inviteCode) return;
    try {
      await navigator.clipboard.writeText(inviteLink());
      inviteCopied = true;
      setTimeout(() => (inviteCopied = false), 1500);
    } catch {
      const el = document.getElementById('invite-link') as HTMLInputElement | null;
      el?.select();
    }
  }

  const dmTotalUnread = $derived(Object.values($dmUnread).reduce((n, v) => n + v, 0));

  const activeSpaceId = $derived<string | null>(page.params.id ?? null);
  const activeChannelId = $derived<string | null>(page.params.cid ?? null);

  let splitChannelId = $state<string | null>(null);
  let splitWidth = $state(480);
  const splitSrc = $derived(
    splitChannelId && activeSpaceId
      ? `/app/spaces/${activeSpaceId}/channels/${splitChannelId}?popout=1`
      : ''
  );
  function openSplit(channelId: string) {
    splitChannelId = channelId;
  }
  function closeSplit() {
    splitChannelId = null;
  }
  function setSplitWidth(px: number) {
    splitWidth = Math.max(320, Math.min(px, 900));
  }
  let splitSpace: string | null = null;
  $effect(() => {
    if (activeSpaceId !== splitSpace) {
      splitSpace = activeSpaceId;
      splitChannelId = null;
    }
  });
  const canModerate = $derived(
    !!activeSpaceId &&
      $spaces.data.find((s) => s.id === activeSpaceId)?.owner_id === $auth.user?.id
  );
  const channelSlice = $derived(
    activeSpaceId ? ($channelsBySpace[activeSpaceId] ?? { data: [], loading: false, error: null }) : null
  );
  const categorySlice = $derived(
    activeSpaceId ? ($categoriesBySpace[activeSpaceId] ?? { data: [], loading: false, error: null }) : null
  );

  let ws: WebSocket | null = null;
  onMount(async () => {
    if (page.url.searchParams.get('popout') === '1') focusMode.set(true);
    await loadSpaces();
    void loadReadState().catch(() => {});
    void loadInbox().catch(() => void refreshInboxUnread());
    startMessageIngest();
    startDmIngest();
    startPresenceIngest();
    startStatusIngest();
    startReactionIngest();
    startReadIngest();
    startTypingIngest();
    void import('$lib/stores/members').then((m) => m.startMemberPresenceSync());
    void import('$lib/stores/friends').then((f) => { f.loadFriends(); f.loadBlocks(); f.loadRequests(); });
    void startXMPP();
    try {
      const devs = await api<Device[]>('/api/me/devices');
      if (devs[0]) subscribePush(devs[0].ntfy_topic);
    } catch {}

    const token = $auth.accessToken;
    const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProto}//${window.location.host}/api/me/events?token=${token}`;
    ws = new WebSocket(wsUrl);

    ws.addEventListener('message', (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'friend_request') {
          import('$lib/stores/friends').then(f => f.loadRequests());
        } else if (msg.type === 'friend_accept') {
          import('$lib/stores/friends').then(f => {
            f.loadFriends();
            f.loadRequests();
          });
        } else if (msg.type === 'friend_remove') {
          import('$lib/stores/friends').then(f => f.loadFriends());
        } else if (msg.type === 'profile_update') {
          import('$lib/stores/friends').then(f => f.loadFriends());
          import('$lib/stores/members').then(m => m.reloadLoadedSpaces());
        } else if (msg.type === 'inbox_update') {
          void refreshInboxUnread();
        } else if (msg.type === 'member_join') {
          const sid = msg.data?.space_id;
          if (sid) import('$lib/stores/members').then(m => m.loadMembers(sid).catch(() => {}));
        } else if (msg.type === 'group_message') {
          import('$lib/stores/groups').then(g => g.onGroupMessage($auth.user?.id ?? '', msg.data));
        } else if (msg.type === 'group_update') {
          import('$lib/stores/groups').then(g => g.onGroupUpdate(msg.data));
        } else if (msg.type === 'space_update') {
          const sid = msg.data?.space_id;
          const what = msg.data?.what;
          if (sid && (what === 'channels' || !what)) { void loadChannels(sid); void loadCategories(sid); }
          if (sid && (what === 'members' || !what)) import('$lib/stores/members').then(m => m.loadMembers(sid).catch(() => {}));
          if (sid && (what === 'roles' || !what)) import('$lib/stores/roles').then(m => m.loadRoles(sid).catch(() => {}));
        }
      } catch (e) {
        console.error('ws parse error', e);
      }
    });

  });

  onDestroy(() => {
    stopXMPP();
    ws?.close();
  });

  $effect(() => {
    if (
      activeSpaceId &&
      !$spaces.loading &&
      $spaces.data.length > 0 &&
      !$spaces.data.some((s) => s.id === activeSpaceId)
    ) {
      void goto('/app');
    }
  });

  $effect(() => {
    if (activeSpaceId && !$channelsBySpace[activeSpaceId]) {
      void loadChannels(activeSpaceId);
    }
  });

  $effect(() => {
    if (activeSpaceId && !$categoriesBySpace[activeSpaceId]) {
      void loadCategories(activeSpaceId);
    }
  });

  $effect(() => {
    if (activeSpaceId && canModerate) void refreshPendingReports(activeSpaceId);
  });

  const joinedSpacePresence = new Set<string>();
  $effect(() => {
    if ($xmppState !== 'online') {
      joinedSpacePresence.clear();
      return;
    }
    for (const s of $spaces.data) {
      if (joinedSpacePresence.has(s.id)) continue;
      joinedSpacePresence.add(s.id);
      void joinSpacePresence(s.id).catch(() => {});
    }
  });

  async function submitSpace(e: Event) {
    e.preventDefault();
    modalErr = null;
    modalBusy = true;
    try {
      const sp = await createSpace(newSpaceName.trim());
      spaceModal = false;
      newSpaceName = '';
      await goto(`/app/spaces/${sp.id}`);
    } catch (err) {
      modalErr = err instanceof Error ? err.message : 'failed';
    } finally {
      modalBusy = false;
    }
  }

  async function setSpaceNotif(spaceId: string, level: NotifLevel) {
    try {
      await setNotifSetting('space', spaceId, { level });
    } catch (e) {
      console.error('space notif', e);
    }
  }
  async function leaveSpaceConfirm(spaceId: string, name: string) {
    if (!confirm(`Quitter l'espace « ${name} » ?`)) return;
    const wasActive = spaceId === activeSpaceId;
    try {
      await leaveSpace(spaceId);
      if (wasActive) await goto('/app');
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Impossible de quitter');
    }
  }

  async function submitChannel(e: Event) {
    e.preventDefault();
    if (!activeSpaceId) return;
    modalErr = null;
    modalBusy = true;
    try {
      const ch = await createChannel(
        activeSpaceId,
        newChannelName.trim(),
        newChannelType,
        newChannelCategory
      );
      channelModal = false;
      newChannelName = '';
      newChannelType = 'text';
      newChannelCategory = null;
      await goto(`/app/spaces/${activeSpaceId}/channels/${ch.id}`);
    } catch (err) {
      modalErr = err instanceof Error ? err.message : 'failed';
    } finally {
      modalBusy = false;
    }
  }

  function onWindowKeydown(e: KeyboardEvent) {
    const el = e.target as HTMLElement | null;
    const typing =
      !!el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName));
    if ((e.ctrlKey || e.metaKey) && e.key === '.') {
      e.preventDefault();
      toggleFocusMode();
    } else if (e.key === 'Escape' && $focusMode && !typing) {
      toggleFocusMode();
    }
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

{#if $auth.user && $auth.user.email_verified === false}
  <VerifyEmailGate />
{:else}
<div class="flex h-[100dvh] flex-col overflow-hidden">
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="flex min-h-0 flex-1 overflow-hidden"
  ontouchstart={onTouchStart}
  ontouchend={onTouchEnd}
>
  {#if drawerOpen}
    <button
      type="button"
      aria-label="Fermer le menu"
      onclick={() => (drawerOpen = false)}
      class="fixed inset-0 z-30 bg-black/50 backdrop-blur-sm md:hidden animate-fade-in"
    ></button>
  {/if}

  <div
    class="flex shrink-0 max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:shadow-2xl transition-transform duration-200 ease-smooth {drawerOpen
      ? 'max-md:translate-x-0'
      : 'max-md:-translate-x-full'}{$focusMode ? ' md:hidden' : ''}"
  >
    <aside
      oncontextmenu={onRailBg}
      class="flex w-[4.5rem] shrink-0 flex-col items-center gap-2 border-r border-border bg-base pb-[calc(0.75rem+var(--safe-bottom))] pt-[calc(0.75rem+var(--safe-top))]"
    >
    <a href="/app" title="Accueil" class="relative mb-1 transition-transform duration-150 hover:scale-105">
      <img src="/krovara.png" alt="Krovara" width="44" height="44" class="size-11 rounded-xl" />
      {#if dmTotalUnread + $inboxUnread + $incoming.length > 0}
        <span class="absolute -right-1 -top-1 size-3.5 rounded-full bg-brand ring-2 ring-base"></span>
      {/if}
    </a>
    <span class="mb-1 h-px w-8 bg-border" aria-hidden="true"></span>

    {#if $spaces.loading}
      <div class="size-12 animate-pulse rounded-2xl bg-elevated"></div>
    {/if}
    {#each $spaces.data as space (space.id)}
      {@const isOwner = space.owner_id === $auth.user?.id}
      <ContextMenu.Root>
        <ContextMenu.Trigger>
          {#snippet child({ props })}
            <div {...props} use:longpress>
              <SpaceIcon
                {space}
                active={space.id === activeSpaceId}
                onclick={() => goto(`/app/spaces/${space.id}`)}
              />
            </div>
          {/snippet}
        </ContextMenu.Trigger>
        <ContextMenu.Portal>
          <ContextMenu.Content class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in">
            <p class="px-2 py-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Notifications</p>
            {#each [['all', 'Tous les messages'], ['mentions', 'Mentions seulement'], ['nothing', 'Rien']] as [lvl, label] (lvl)}
              <ContextMenu.Item onSelect={() => setSpaceNotif(space.id, lvl as NotifLevel)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <Bell size={14} class="text-muted" /> {label}
              </ContextMenu.Item>
            {/each}
            <div class="my-1 h-px bg-border/60"></div>
            {#if isOwner}
              <ContextMenu.Item onSelect={() => goto(`/app/spaces/${space.id}/settings`)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <Settings size={14} class="text-muted" /> Paramètres de l'espace
              </ContextMenu.Item>
            {:else}
              <ContextMenu.Item onSelect={() => leaveSpaceConfirm(space.id, space.name)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-danger transition-colors data-[highlighted]:bg-danger/10">
                <LogOut size={14} /> Quitter l'espace
              </ContextMenu.Item>
            {/if}
          </ContextMenu.Content>
        </ContextMenu.Portal>
      </ContextMenu.Root>
    {/each}

    <button
      type="button"
      title={$t("nav.newSpace")}
      onclick={() => {
        modalErr = null;
        spaceModal = true;
      }}
      class="grid size-12 place-items-center rounded-2xl bg-elevated text-success
             transition-[border-radius,background-color,color] duration-150 ease-smooth
             hover:rounded-xl hover:bg-success hover:text-white"
    >
      <Plus size={22} />
    </button>

    <div class="mt-auto flex flex-col items-center gap-1.5">
      {#if $auth.user?.is_admin}
        <a
          href="/app/admin"
          title="Administration"
          class="grid size-10 place-items-center rounded-full text-muted
                 transition-colors duration-150 hover:bg-elevated hover:text-brand"
        >
          <Shield size={18} />
        </a>
      {/if}
      <a
        href="/app/discover"
        title={$t("nav.explore")}
        class="grid size-10 place-items-center rounded-full text-muted
               transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        <Compass size={18} />
      </a>
      <a
        href="/app/settings/profile"
        title={$t("nav.settings")}
        class="grid size-10 place-items-center rounded-full text-muted
               transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        <Settings size={18} />
      </a>
    </div>
  </aside>

  {#if activeSpaceId}
    {@const activeSpace = $spaces.data.find((s) => s.id === activeSpaceId)}
    <aside
      style="width: {$layout.channelSidebarWidth}px"
      class="flex shrink-0 flex-col border-r border-border bg-surface pt-[var(--safe-top)]"
    >
      {#if activeSpace?.banner_key}
        <div class="relative w-full overflow-hidden bg-surface-active h-[88px] shrink-0 border-b border-border shadow-sm">
          {#await authedObjectURL(`/api/files/${activeSpace.banner_key}`) then src}
            <img {src} alt="Banner" class="absolute inset-0 h-full w-full object-cover transition-opacity duration-300" />
          {/await}
          <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-black/40 to-black/10"></div>
          <div class="absolute bottom-0 left-0 right-0 flex items-center justify-between px-4 pb-2.5 pt-4">
            <h2 class="truncate text-body font-semibold text-white drop-shadow-md">
              {activeSpace.name}
            </h2>
            <div class="flex items-center gap-1 text-white/90 drop-shadow-sm">
              <a
                href={`/app/spaces/${activeSpaceId}/search`}
                title={$t('nav.search')}
                class="grid size-7 place-items-center rounded transition-colors duration-150 hover:bg-white/20 hover:text-white"
              >
                <Search size={16} />
              </a>
              <Popover.Root>
                <Popover.Trigger
                  title="Plus"
                  class="grid size-7 place-items-center rounded transition-colors duration-150 hover:bg-white/20 hover:text-white"
                >
                  <MoreHorizontal size={16} />
                </Popover.Trigger>
            <Popover.Portal>
              <Popover.Content
                align="end"
                sideOffset={4}
                class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-lg animate-fade-in"
              >
                <Popover.Close
                  onclick={() => openChannelModal(null)}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <Hash size={16} class="text-muted" /> {$t('nav.newChannel')}
                </Popover.Close>
                <Popover.Close
                  onclick={() => {
                    creatingCategory = true;
                    newCategoryName = '';
                  }}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <FolderPlus size={16} class="text-muted" /> Créer une catégorie
                </Popover.Close>
                <Popover.Close
                  onclick={openInvite}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <UserPlus size={16} class="text-muted" /> {$t('nav.invite')}
                </Popover.Close>
                <div class="my-1 h-px bg-border"></div>
                <a href={`/app/spaces/${activeSpaceId}/tasks`} class="flex items-center gap-2.5 rounded px-2 py-1.5 text-body text-content transition-colors hover:bg-elevated">
                  <ListChecks size={16} class="text-muted" /> {$t('nav.tasks')}
                </a>
                <a href={`/app/spaces/${activeSpaceId}/events`} class="flex items-center gap-2.5 rounded px-2 py-1.5 text-body text-content transition-colors hover:bg-elevated">
                  <CalendarDays size={16} class="text-muted" /> {$t('nav.events')}
                </a>
                {#if canModerate}
                  <a href={`/app/spaces/${activeSpaceId}/reports`} class="flex items-center gap-2.5 rounded px-2 py-1.5 text-body text-content transition-colors hover:bg-elevated">
                    <ShieldAlert size={16} class="text-muted" /> {$t('nav.moderation')}
                    {#if ($pendingReports[activeSpaceId] ?? 0) > 0}
                      <span class="ml-auto grid h-4 min-w-4 place-items-center rounded-full bg-danger px-1 text-[0.625rem] font-semibold text-white">
                        {$pendingReports[activeSpaceId] > 9 ? '9+' : $pendingReports[activeSpaceId]}
                      </span>
                    {/if}
                  </a>
                  <a href={`/app/spaces/${activeSpaceId}/settings`} class="flex items-center gap-2.5 rounded px-2 py-1.5 text-body text-content transition-colors hover:bg-elevated">
                    <Settings size={16} class="text-muted" /> {$t('nav.spaceSettings')}
                  </a>
                {/if}
                <div class="my-1 h-px bg-border"></div>
                <Popover.Close
                  onclick={toggleMembersPanel}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <Users size={16} class="text-muted" />
                  {$layout.membersPanelOpen ? 'Masquer les membres' : 'Afficher les membres'}
                </Popover.Close>
                <Popover.Close
                  onclick={toggleFocusMode}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <Maximize2 size={16} class="text-muted" /> Mode focus
                  <span class="ml-auto text-label text-muted">Ctrl+.</span>
                </Popover.Close>

                  </Popover.Content>
                </Popover.Portal>
              </Popover.Root>
            </div>
          </div>
        </div>
      {:else}
        <div class="flex items-center justify-between border-b border-border px-4 py-3.5 shrink-0">
          <h2 class="truncate text-body font-semibold">
            {activeSpace?.name ?? 'Espace'}
          </h2>
          <div class="flex items-center gap-1 text-muted">
            <a
              href={`/app/spaces/${activeSpaceId}/search`}
              title={$t('nav.search')}
              class="grid size-7 place-items-center rounded transition-colors duration-150 hover:bg-elevated hover:text-content"
            >
              <Search size={16} />
            </a>
            <Popover.Root>
              <Popover.Trigger
                title="Plus"
                class="grid size-7 place-items-center rounded text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
              >
                <MoreHorizontal size={16} />
              </Popover.Trigger>
              <Popover.Portal>
                <Popover.Content
                  align="end"
                  sideOffset={4}
                  class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-lg animate-fade-in"
                >
                  <Popover.Close
                    onclick={() => openChannelModal(null)}
                    class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                  >
                    <Hash size={16} class="text-muted" /> {$t('nav.newChannel')}
                  </Popover.Close>
                  <Popover.Close
                    onclick={() => {
                      creatingCategory = true;
                      newCategoryName = '';
                    }}
                    class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                  >
                    <FolderPlus size={16} class="text-muted" /> Créer une catégorie
                  </Popover.Close>
                  <div class="my-1 h-px bg-border/60"></div>
                  <Popover.Close
                    onclick={openInvite}
                    class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                  >
                    <UserPlus size={16} class="text-muted" /> {$t('nav.invite')}
                  </Popover.Close>
                </Popover.Content>
              </Popover.Portal>
            </Popover.Root>
          </div>
        </div>
      {/if}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="flex-1 overflow-y-auto p-2" oncontextmenu={onChanBg}>
        {#if channelSlice?.loading}
          {#each [0, 1, 2] as i (i)}
            <div class="mx-2 my-1.5 h-4 animate-pulse rounded bg-elevated/60"></div>
          {/each}
        {/if}
        {#if channelSlice?.error}
          <p class="px-2 py-1 text-label text-danger">{channelSlice.error}</p>
        {/if}
        {#if !channelSlice?.loading && !channelSlice?.error}
          <ChannelList
            spaceId={activeSpaceId}
            channels={channelSlice?.data ?? []}
            categories={categorySlice?.data ?? []}
            {activeChannelId}
            unread={$unreadByChannel}
            mentions={$mentionsByChannel}
            canManage={canModerate}
            onSelect={(channel) => {
              drawerOpen = false;
              void goto(`/app/spaces/${activeSpaceId}/channels/${channel.id}`);
            }}
            onActivate={(channel) => {
              drawerOpen = false;
              if (channel.type === 'voice') void joinVoice(channel.id);
              void goto(`/app/spaces/${activeSpaceId}/channels/${channel.id}`);
            }}
            onAddChannel={openChannelModal}
            onAddCategory={() => {
              creatingCategory = true;
              newCategoryName = '';
            }}
            onSettings={(channel) => (settingsChannel = channel)}
            oninvite={openInvite}
            onsplit={(channel) => openSplit(channel.id)}
          />
          {#if (channelSlice?.data?.length ?? 0) === 0}
            <p class="px-2 py-3 text-center text-label text-muted">Aucun salon. Crée-en un avec +</p>
          {/if}
        {/if}
      </div>
      {#if canModerate && creatingCategory}
        <div class="border-t border-border p-2">
          <!-- svelte-ignore a11y_autofocus -->
          <input
            autofocus
            bind:value={newCategoryName}
            onkeydown={(e) => {
              if (e.key === 'Enter') void submitCategory();
              if (e.key === 'Escape') {
                creatingCategory = false;
                newCategoryName = '';
              }
            }}
            onblur={() => void submitCategory()}
            maxlength={64}
            placeholder="Nom de la catégorie"
            class="w-full rounded border border-border-strong bg-base px-2 py-1.5 text-body
                   text-content outline-none placeholder:text-muted"
          />
        </div>
      {/if}
    </aside>
    <div class="hidden md:block">
      <ResizeHandle
        width={$layout.channelSidebarWidth}
        min={CHANNEL_SIDEBAR_MIN}
        max={CHANNEL_SIDEBAR_MAX}
        edge="right"
        onresize={setChannelSidebarWidth}
        label="Redimensionner la liste des salons"
      />
    </div>
  {/if}
  </div>

  <main class="flex flex-1 flex-col overflow-hidden bg-base">
    <header class="flex items-center justify-between border-b border-border px-4 pb-3 pt-[calc(0.75rem+var(--safe-top))] md:hidden">
      <button
        type="button"
        onclick={() => (drawerOpen = !drawerOpen)}
        class="grid size-9 place-items-center rounded border border-border text-muted transition-colors hover:bg-elevated hover:text-content"
        aria-label="Menu"
      >
        <Menu size={18} />
      </button>
      <div class="font-bold tracking-tight text-content">Krovara</div>
      {#if activeSpaceId}
        <button
          type="button"
          onclick={() => (membersDrawerOpen = !membersDrawerOpen)}
          class="grid size-9 place-items-center rounded border border-border text-muted transition-colors hover:bg-elevated hover:text-content"
          aria-label="Membres"
        >
          <Users size={18} />
        </button>
      {:else}
        <div class="size-9"></div>
      {/if}
    </header>
    <div class="flex-1 overflow-y-auto">
      {@render children()}
    </div>
  </main>

  {#if activeSpaceId && $layout.membersPanelOpen && !$focusMode}
    <div class="hidden lg:block">
      <ResizeHandle
        width={$layout.membersPanelWidth}
        min={MEMBERS_PANEL_MIN}
        max={MEMBERS_PANEL_MAX}
        edge="left"
        onresize={setMembersPanelWidth}
        label="Redimensionner le panneau des membres"
      />
    </div>
    <MembersPanel spaceId={activeSpaceId} width={$layout.membersPanelWidth} canModerate={canModerate} oninvite={openInvite} />
  {/if}

  {#if activeSpaceId && membersDrawerOpen}
    <button
      type="button"
      aria-label="Fermer les membres"
      onclick={() => (membersDrawerOpen = false)}
      class="fixed inset-0 z-30 bg-black/50 backdrop-blur-sm md:hidden animate-fade-in"
    ></button>
    <aside
      class="fixed inset-y-0 right-0 z-40 flex w-72 max-w-[80vw] flex-col border-l border-border bg-surface shadow-2xl md:hidden pt-[var(--safe-top)] pb-[var(--safe-bottom)]"
    >
      <MembersPanel spaceId={activeSpaceId} canModerate={canModerate} oninvite={openInvite} overlay />
    </aside>
  {/if}

  {#if splitChannelId && activeSpaceId && !$focusMode}
    <div class="hidden lg:block">
      <ResizeHandle
        width={splitWidth}
        min={320}
        max={900}
        edge="left"
        onresize={setSplitWidth}
        label="Redimensionner la vue divisée"
      />
    </div>
    <aside
      style="width: {splitWidth}px"
      class="hidden shrink-0 flex-col border-l border-border bg-base lg:flex"
    >
      <div class="flex items-center justify-between border-b border-border px-3 py-2">
        <span class="flex items-center gap-1.5 text-label font-medium text-muted">
          <Columns2 size={14} /> Vue divisée
        </span>
        <button
          type="button"
          onclick={closeSplit}
          title="Fermer la vue divisée"
          aria-label="Fermer la vue divisée"
          class="grid size-6 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
        >
          <X size={15} />
        </button>
      </div>
      <iframe src={splitSrc} title="Vue divisée" class="min-h-0 flex-1 border-0"></iframe>
    </aside>
  {/if}
</div>
</div>
{/if}

<BgContextMenu bind:open={chanBg.open} x={chanBg.x} y={chanBg.y} items={chanBgItems} />
<BgContextMenu bind:open={railBg.open} x={railBg.x} y={railBg.y} items={railBgItems} />

{#if $focusMode}
  <button
    type="button"
    onclick={toggleFocusMode}
    title="Quitter le mode focus (Échap)"
    class="fixed right-4 top-[calc(1rem+var(--safe-top))] z-50 flex items-center gap-1.5 rounded-full border border-border
           bg-surface/90 px-3 py-1.5 text-label text-muted shadow-lg backdrop-blur
           transition-colors duration-150 hover:text-content"
  >
    <Minimize2 size={15} /> Quitter le focus
  </button>
{/if}

<QuickSwitcher />
<ShortcutsHelp />
<LiveRegion />

<CallPanel />

{#if activeSpaceId}
  <ChannelSettingsModal
    open={!!settingsChannel}
    channel={settingsChannel}
    spaceId={activeSpaceId}
    onclose={() => (settingsChannel = null)}
  />
{/if}

<Modal open={spaceModal} title={spaceModalTab === 'create' ? "Créer un espace" : "Rejoindre un espace"} onclose={() => (spaceModal = false)}>
  <div class="mb-4 flex gap-1 border-b border-border">
    <button
      type="button"
      onclick={() => (spaceModalTab = 'create')}
      class="-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-body transition-colors duration-150 {spaceModalTab === 'create' ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
    >
      <Plus size={16} /> Créer
    </button>
    <button
      type="button"
      onclick={() => (spaceModalTab = 'join')}
      class="-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-body transition-colors duration-150 {spaceModalTab === 'join' ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
    >
      <Compass size={16} /> Rejoindre
    </button>
  </div>

  {#if spaceModalTab === 'create'}
    <form onsubmit={submitSpace} class="space-y-4">
      <Input
        label="Nom de l'espace"
        required
        minlength={1}
        maxlength={64}
        placeholder="Ma communauté"
        bind:value={newSpaceName}
        error={modalErr}
      />
      <Button type="submit" full loading={modalBusy}>Créer l'espace</Button>
    </form>
  {:else}
    <form onsubmit={(e) => {
      e.preventDefault();
      if (!joinCode.trim()) return;
      let code = joinCode.trim();
      const match = code.match(/\/join\/([a-zA-Z0-9_-]+)/);
      if (match) code = match[1];
      spaceModal = false;
      joinCode = '';
      goto('/join/' + code);
    }} class="space-y-4">
      <Input
        label="Code d'invitation ou lien"
        required
        placeholder="x7ganda6 ou https://krovara.com/join/..."
        bind:value={joinCode}
        error={modalErr}
      />
      <Button type="submit" full>Rejoindre</Button>
    </form>
  {/if}
</Modal>

<Modal open={channelModal} title="Créer un salon" onclose={() => (channelModal = false)}>
  <form onsubmit={submitChannel} class="space-y-4">
    <Input
      label="Nom du salon"
      required
      minlength={1}
      maxlength={64}
      placeholder="off-topic"
      bind:value={newChannelName}
    />
    <div class="space-y-1.5">
      <span class="block text-label font-medium text-muted">Type</span>
      <div class="grid grid-cols-2 gap-2">
        <button
          type="button"
          onclick={() => (newChannelType = 'text')}
          class="rounded border px-3 py-2 text-body transition-colors duration-150 {newChannelType ===
          'text'
            ? 'border-primary bg-primary/10 text-content'
            : 'border-border text-muted hover:border-border-strong'}"
        >
          # Texte
        </button>
        <button
          type="button"
          onclick={() => (newChannelType = 'voice')}
          class="rounded border px-3 py-2 text-body transition-colors duration-150 {newChannelType ===
          'voice'
            ? 'border-primary bg-primary/10 text-content'
            : 'border-border text-muted hover:border-border-strong'}"
        >
          🔊 Vocal
        </button>
      </div>
    </div>
    {#if modalErr}
      <p class="text-label text-danger">{modalErr}</p>
    {/if}
    <Button type="submit" full loading={modalBusy}>Créer le salon</Button>
  </form>
</Modal>

<Modal open={inviteModal} title="Inviter dans cet espace" onclose={() => (inviteModal = false)}>
  {#if !inviteCode}
    <div class="space-y-4">
      <div class="space-y-1.5">
        <span class="block text-label font-medium text-muted">Nombre d'utilisations max</span>
        <div class="flex flex-wrap gap-1">
          {#each INVITE_USES as o (o.v)}
            <button
              type="button"
              onclick={() => (inviteMaxUses = o.v)}
              aria-pressed={inviteMaxUses === o.v}
              class="rounded border px-2.5 py-1 text-label transition-colors duration-150
                     {inviteMaxUses === o.v
                ? 'border-primary bg-primary/10 text-content'
                : 'border-border text-muted hover:border-border-strong'}"
            >
              {o.label}
            </button>
          {/each}
        </div>
      </div>
      <div class="space-y-1.5">
        <span class="block text-label font-medium text-muted">Expire après</span>
        <div class="flex flex-wrap gap-1">
          {#each INVITE_TTL as o (o.v)}
            <button
              type="button"
              onclick={() => (inviteTtl = o.v)}
              aria-pressed={inviteTtl === o.v}
              class="rounded border px-2.5 py-1 text-label transition-colors duration-150
                     {inviteTtl === o.v
                ? 'border-primary bg-primary/10 text-content'
                : 'border-border text-muted hover:border-border-strong'}"
            >
              {o.label}
            </button>
          {/each}
        </div>
      </div>
      {#if modalErr}<p class="text-label text-danger">{modalErr}</p>{/if}
      <Button type="button" full loading={modalBusy} onclick={generateInvite}>
        Générer le lien d'invitation
      </Button>

      {#if activeInvites.length}
        <div class="border-t border-border pt-3">
          <p class="mb-2 text-label font-medium text-muted">Invitations actives</p>
          <ul class="space-y-1.5">
            {#each activeInvites as inv (inv.code)}
              <li class="flex items-center gap-2 rounded-lg border border-border px-3 py-2">
                <div class="min-w-0 flex-1">
                  <code class="block truncate font-mono text-label text-accent">{inv.code}</code>
                  <span class="text-[0.625rem] text-muted">{inviteStat(inv)}</span>
                </div>
                <button
                  type="button"
                  title="Copier le lien"
                  onclick={() => navigator.clipboard?.writeText(`${window.location.origin}/join/${inv.code}`)}
                  class="grid size-7 shrink-0 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
                >
                  <Copy size={14} />
                </button>
                <button
                  type="button"
                  title="Révoquer"
                  onclick={() => revokeInvite(inv.code)}
                  class="grid size-7 shrink-0 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-danger"
                >
                  <Trash2 size={14} />
                </button>
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    </div>
  {:else}
    <p class="text-body text-muted">
      Partage ce lien.{inviteMaxUses > 0 ? ` Jusqu'à ${inviteMaxUses} utilisation${inviteMaxUses > 1 ? 's' : ''}.` : ' Utilisations illimitées.'}
    </p>
    <div class="mt-3 flex gap-2">
      <input
        id="invite-link"
        readonly
        value={inviteLink()}
        class="h-10 flex-1 rounded border border-border bg-base/50 px-3 text-body text-content outline-none"
      />
      <Button type="button" onclick={copyInvite}>
        {#if inviteCopied}<Check size={16} /> Copié{:else}<Copy size={16} /> Copier{/if}
      </Button>
    </div>
    <p class="mt-3 text-label text-muted">
      Ou juste le code : <code class="rounded bg-base px-1.5 py-0.5 font-mono text-accent">{inviteCode}</code>
    </p>
  {/if}
</Modal>
