<script lang="ts">
  import { Phone, Check, UserPlus, Shield, AtSign, MessageSquare, Ban, EyeOff, StickyNote, User, Link as LinkIcon, Copy, UserX, Gavel } from '@lucide/svelte';
  import { Popover, ContextMenu } from 'bits-ui';
  import { goto } from '$app/navigation';
  import ProfileCard from './ProfileCard.svelte';
  import FullProfile from './FullProfile.svelte';
  import BgContextMenu from './BgContextMenu.svelte';
  import Modal from './Modal.svelte';
  import { Button } from '$lib/ui';
  import { friends, loadFriends, sendRequest, blockHandle } from '$lib/stores/friends';
  import { BADGES, sortBadges } from '$lib/badges';
  import { api, ApiError, authedObjectURL } from '$lib/api';
  import { presences } from '$lib/stores/presence';
  import { peerStatus, selfAvailability, customStatus, STATUS_META, type Availability } from '$lib/stores/status';
  import { openConversation } from '$lib/stores/dm';
  import { auth } from '$lib/stores/auth';
  import { membersBySpace, loadMembers, type Member } from '$lib/stores/members';
  import {
    rolesBySpace,
    loadRoles,
    memberRoleIds,
    assignRole,
    removeRole
  } from '$lib/stores/roles';
  import { placeCall } from '$lib/voip/signaling';
  import { activeRoom } from '$lib/voip/room';
  import { voiceMode } from '$lib/voip/mode';

  type Props = { spaceId: string; width?: number; canModerate?: boolean; oninvite?: () => void; overlay?: boolean };
  let { spaceId, width = 240, canModerate = false, oninvite, overlay = false }: Props = $props();

  let bg = $state({ open: false, x: 0, y: 0 });
  const bgItems = $derived([
    ...(oninvite ? [{ label: 'Inviter des membres', icon: UserPlus, onclick: oninvite }] : []),
    ...(canModerate
      ? [{ label: 'Gérer les rôles', icon: Shield, onclick: () => void goto(`/app/spaces/${spaceId}/roles`) }]
      : [])
  ]);
  function onBg(e: MouseEvent) {
    if (e.defaultPrevented || bgItems.length === 0) return;
    e.preventDefault();
    bg = { open: true, x: e.clientX, y: e.clientY };
  }

  const spaceRoles = $derived(($rolesBySpace[spaceId] ?? []).filter((r) => !r.is_everyone));
  let menuMemberRoleIds = $state<Set<string>>(new Set());
  let menuMemberId = $state<string | null>(null);
  async function openMemberMenu(m: Member) {
    menuMemberId = m.id;
    menuMemberRoleIds = new Set();
    if (!$rolesBySpace[spaceId]) void loadRoles(spaceId).catch(() => {});
    try {
      menuMemberRoleIds = new Set(await memberRoleIds(m.id));
    } catch {
    }
  }
  async function toggleRole(m: Member, roleId: string) {
    const had = menuMemberRoleIds.has(roleId);
    try {
      if (had) await removeRole(m.id, roleId);
      else await assignRole(m.id, roleId);
      const next = new Set(menuMemberRoleIds);
      if (had) next.delete(roleId);
      else next.add(roleId);
      menuMemberRoleIds = next;
      await loadMembers(spaceId);
    } catch (e) {
      console.error('toggle role', e);
    }
  }
  function copyId(id: string) {
    void navigator.clipboard?.writeText(id);
  }

  import { timeoutMember } from '$lib/stores/timeouts';
  async function applyTimeout(userId: string, minutes: number) {
    try {
      await timeoutMember(spaceId, userId, minutes);
    } catch (e) {
      console.error('timeout', e);
    }
  }

  let loading = $state(false);
  let err = $state<string | null>(null);

  const selfId = $derived($auth.user?.id ?? '');

  let avatarUrls = $state<Record<string, string>>({});
  $effect(() => {
    const list = $membersBySpace[spaceId] ?? [];
    let cancelled = false;
    for (const m of list) {
      const key = m.avatar_key;
      if (!key || avatarUrls[key]) continue;
      void authedObjectURL(`/api/files/${key}`)
        .then((u) => {
          if (!cancelled) avatarUrls = { ...avatarUrls, [key]: u };
        })
        .catch(() => {});
    }
    return () => {
      cancelled = true;
    };
  });

  function availabilityFor(userId: string): Availability {
    if (userId === selfId) {
      return $selfAvailability === 'invisible' ? 'offline' : $selfAvailability;
    }
    return $peerStatus[userId]?.availability ?? 'offline';
  }

  const RANK: Record<Availability, number> = { online: 0, idle: 1, dnd: 2, offline: 3 };
  const members = $derived(
    [...($membersBySpace[spaceId] ?? [])].sort((a, b) => {
      const d = RANK[availabilityFor(a.user_id)] - RANK[availabilityFor(b.user_id)];
      return d !== 0 ? d : (a.nickname || a.username).localeCompare(b.nickname || b.username);
    })
  );

  type Section = { key: string; label: string; color: string | null; members: typeof members };
  const sections = $derived.by<Section[]>(() => {
    const online = members.filter((m) => availabilityFor(m.user_id) !== 'offline');
    const offline = members.filter((m) => availabilityFor(m.user_id) === 'offline');
    const hoisted = new Map<string, Section>();
    const noRole: typeof members = [];
    for (const m of online) {
      if (m.hoist_role) {
        const key = m.hoist_role + ':' + (m.hoist_position ?? 0);
        let sec = hoisted.get(key);
        if (!sec) {
          sec = { key, label: m.hoist_role, color: m.role_color ?? null, members: [] };
          hoisted.set(key, sec);
        }
        sec.members.push(m);
      } else {
        noRole.push(m);
      }
    }
    const out = [...hoisted.values()].sort((a, b) => {
      const pa = Number(a.key.split(':').pop()), pb = Number(b.key.split(':').pop());
      return pb - pa;
    });
    if (noRole.length) out.push({ key: 'online', label: 'En ligne', color: null, members: noRole });
    if (offline.length) out.push({ key: 'offline', label: 'Hors ligne', color: null, members: offline });
    return out;
  });

  const onlineCount = $derived(
    (($membersBySpace[spaceId] ?? []).filter((m) => availabilityFor(m.user_id) !== 'offline')).length
  );

  function statusText(userId: string): string | undefined {
    if (userId === selfId) return $customStatus.trim() || undefined;
    return $peerStatus[userId]?.text;
  }

  $effect(() => {
    void load(spaceId);
  });

  $effect(() => {
    if ($friends.length === 0) void loadFriends().catch(() => {});
  });

  const friendIds = $derived(new Set($friends.map((f) => f.id)));

  async function load(id: string) {
    if (!id) return;
    loading = true;
    err = null;
    try {
      await loadMembers(id);
    } catch (e) {
      err = e instanceof Error ? e.message : 'load failed';
    } finally {
      loading = false;
    }
  }

  function presenceFor(userId: string) {
    return $presences[`${userId}@krovara.local`] ?? null;
  }

  function call(userId: string) {
    const jid = `${userId}@krovara.local`;
    if (voiceMode === 'mesh' && $activeRoom) {
      void placeCall(jid, $activeRoom);
    } else {
      (window as unknown as { krovaraCall?: (jid: string) => void }).krovaraCall?.(jid);
    }
  }

  let friendMsg = $state<{ ok: boolean; text: string } | null>(null);
  let friendTimer: ReturnType<typeof setTimeout> | null = null;
  function flashFriend(ok: boolean, text: string) {
    friendMsg = { ok, text };
    if (friendTimer) clearTimeout(friendTimer);
    friendTimer = setTimeout(() => (friendMsg = null), 3500);
  }
  async function addFriend(m: Member) {
    try {
      await sendRequest(m.username);
      flashFriend(true, `Demande d'ami envoyée à ${m.username}.`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) flashFriend(false, 'Demande déjà envoyée ou déjà amis.');
      else if (e instanceof ApiError && e.status === 403) flashFriend(false, `${m.username} n'accepte pas les demandes.`);
      else flashFriend(false, "Impossible d'envoyer la demande.");
    }
  }

  async function kickMember(m: Member) {
    if (!window.confirm(`Expulser ${m.username} du serveur ?`)) return;
    try {
      await api(`/api/spaces/${spaceId}/members/${m.user_id}`, { method: 'DELETE' });
      await loadMembers(spaceId);
      flashFriend(true, `${m.username} a été expulsé.`);
    } catch {
      flashFriend(false, "Impossible d'expulser ce membre.");
    }
  }

  let profileTarget = $state<Member | null>(null);

  let banTarget = $state<Member | null>(null);
  let banReason = $state('');
  let banWipe = $state(false);
  let banBusy = $state(false);
  function openBan(m: Member) {
    banTarget = m;
    banReason = '';
    banWipe = false;
  }
  async function confirmBan() {
    if (!banTarget || banBusy) return;
    banBusy = true;
    try {
      await api(`/api/spaces/${spaceId}/bans`, {
        method: 'POST',
        body: { user_id: banTarget.user_id, reason: banReason.trim() || undefined, wipe_messages: banWipe }
      });
      await loadMembers(spaceId);
      flashFriend(true, `${banTarget.username} a été banni.`);
      banTarget = null;
    } catch {
      flashFriend(false, 'Impossible de bannir ce membre.');
    } finally {
      banBusy = false;
    }
  }
</script>

<aside
  style={overlay ? '' : `width: ${width}px`}
  class={overlay
    ? 'flex h-full w-full flex-col bg-surface'
    : 'hidden shrink-0 border-l border-border bg-surface/60 lg:flex lg:flex-col'}
>
  <div class="border-b border-border px-4 py-3.5">
    <h2 class="text-label font-semibold uppercase tracking-wide text-muted">
      Membres{#if onlineCount > 0} · {onlineCount} en ligne{/if}
    </h2>
  </div>
  {#if friendMsg}
    <p class="mx-2 mt-2 rounded-md border px-2.5 py-1.5 text-label {friendMsg.ok ? 'border-success/30 bg-success/10 text-success' : 'border-danger/30 bg-danger/10 text-danger'}">
      {friendMsg.text}
    </p>
  {/if}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="flex-1 space-y-0.5 overflow-y-auto p-2" oncontextmenu={onBg}>
    {#if loading && members.length === 0}
      <p class="px-2 py-1 text-label text-muted">Chargement…</p>
    {/if}
    {#if err}
      <p class="px-2 py-1 text-label text-danger">{err}</p>
    {/if}
    {#each sections as section (section.key)}
      <p class="px-2 pb-0.5 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">
        <span style={section.color ? `color:${section.color}` : ''}>{section.label}</span>
        <span class="text-muted/70"> · {section.members.length}</span>
      </p>
      {#each section.members as m (m.id)}
      {@const pres = presenceFor(m.user_id)}
      {@const isSelf = m.user_id === selfId}
      {@const avail = availabilityFor(m.user_id)}
      {@const note = statusText(m.user_id)}
      <ContextMenu.Root onOpenChange={(o) => { if (o) void openMemberMenu(m); }}>
      <ContextMenu.Trigger>
      {#snippet child({ props })}
      <div
        {...props}
        class="group flex items-start gap-2.5 rounded px-2 py-1.5 transition-colors duration-150 hover:bg-elevated/60
               {avail === 'offline' ? 'opacity-50' : ''}"
      >
        <Popover.Root>
          <Popover.Trigger class="flex min-w-0 flex-1 items-start gap-2.5 text-left">
            <div class="relative mt-0.5 size-8 shrink-0">
              <div class="grid size-full place-items-center overflow-hidden rounded-full bg-elevated text-label font-medium text-muted">
                {#if m.avatar_key && avatarUrls[m.avatar_key]}
                  <img src={avatarUrls[m.avatar_key]} alt="" class="size-full object-cover" />
                {:else}
                  {(m.nickname || m.username).slice(0, 2).toUpperCase()}
                {/if}
              </div>
              <span
                title={STATUS_META[avail].label}
                class="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full ring-2 ring-surface {STATUS_META[avail].dot}"
              ></span>
            </div>
            <div class="min-w-0 flex-1">
              <p class="flex items-center gap-1.5 truncate text-body text-content">
                {#if m.role_icon}<span class="shrink-0">{m.role_icon}</span>{/if}
                <span class="truncate font-medium" style={m.role_color ? `color:${m.role_color}` : ''}>
                  {m.nickname || m.username}
                </span>
                {#if isSelf}
                  <span class="shrink-0 rounded bg-elevated px-1 py-px text-[0.625rem] font-medium uppercase tracking-wide text-muted">vous</span>
                {/if}
                {#if m.badges?.length}
                  <span class="flex items-center gap-0.5 shrink-0 ml-1">
                    {#each sortBadges(m.badges) as b (b)}
                      <img src={BADGES[b].img} alt={BADGES[b].label} title={BADGES[b].label} class="size-4 object-contain" />
                    {/each}
                  </span>
                {/if}
              </p>
              {#if note}
                <p class="truncate text-label text-muted">{note}</p>
              {/if}
              {#if pres}
                <p class="truncate text-label text-accent">
                  <span class="font-medium">{pres.game}</span>
                  {#if pres.state}
                    <span class="text-muted"> · {pres.state}</span>
                  {/if}
                </p>
                {#if pres.details}
                  <p class="truncate text-label text-muted">{pres.details}</p>
                {/if}
              {/if}
            </div>
          </Popover.Trigger>
          <Popover.Portal>
            <Popover.Content
              side="left"
              align="start"
              sideOffset={8}
              class="z-50 rounded-lg border border-border bg-surface shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
            >
              <ProfileCard
                name={m.nickname || m.username}
                username={m.username}
                avatarKey={m.avatar_key}
                availability={avail}
                statusText={note}
                game={pres}
                isSelf={isSelf}
                canModerate={canModerate}
                userId={m.user_id}
                spaceId={spaceId}
                oncall={() => call(m.user_id)}
                ontimeout={(min) => applyTimeout(m.user_id, min)}
              />
            </Popover.Content>
          </Popover.Portal>
        </Popover.Root>
        {#if !isSelf}
          <button
            type="button"
            title="Appeler"
            onclick={() => call(m.user_id)}
            class="mt-0.5 text-muted opacity-0 transition duration-150 group-hover:opacity-100 hover:text-success"
          >
            <Phone size={16} />
          </button>
        {/if}
      </div>
      {/snippet}
      </ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content
          class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in"
        >
          <ContextMenu.Item
            onSelect={() => (profileTarget = m)}
            class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
          >
            <User size={14} class="text-muted" /> Voir le profil
          </ContextMenu.Item>
          <div class="my-1 h-px bg-border/60"></div>
          {#if !isSelf}
            <ContextMenu.Item
              onSelect={() => { document.dispatchEvent(new CustomEvent('krovara:mention', { detail: m.username })); }}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
            >
              <AtSign size={14} class="text-muted" /> Mentionner
            </ContextMenu.Item>
            <ContextMenu.Item
              onSelect={() => {
                openConversation(m.user_id);
                void goto('/app');
              }}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
            >
              <MessageSquare size={14} class="text-muted" /> Envoyer un message
            </ContextMenu.Item>
            <div class="my-1 h-px bg-border/60"></div>
            {#if friendIds.has(m.user_id)}
              <div class="flex items-center gap-2 rounded px-2 py-1.5 text-label text-success">
                <Check size={14} /> Déjà ami
              </div>
            {:else}
              <ContextMenu.Item
                onSelect={() => addFriend(m)}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                <UserPlus size={14} class="text-muted" /> Ajouter en ami
              </ContextMenu.Item>
            {/if}
            <ContextMenu.Item
              onSelect={() => call(m.user_id)}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
            >
              <Phone size={14} class="text-muted" /> Appeler
            </ContextMenu.Item>
            <ContextMenu.Item
              onSelect={() => alert('À venir (Ajouter une note)')}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
            >
              <StickyNote size={14} class="text-muted" /> Ajouter une note
            </ContextMenu.Item>
            <div class="my-1 h-px bg-border/60"></div>
          {/if}
          {#if canModerate && spaceRoles.length}
            <ContextMenu.Sub>
              <ContextMenu.SubTrigger
                class="flex cursor-pointer items-center justify-between gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                Rôles <span class="text-muted">›</span>
              </ContextMenu.SubTrigger>
              <ContextMenu.SubContent
                class="z-50 max-h-72 w-52 overflow-y-auto rounded-lg border border-border bg-overlay p-1 shadow-xl"
              >
                {#each spaceRoles as role (role.id)}
                  <ContextMenu.Item
                    closeOnSelect={false}
                    onSelect={() => toggleRole(m, role.id)}
                    class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label transition-colors data-[highlighted]:bg-elevated"
                  >
                    <span class="grid size-3.5 shrink-0 place-items-center">
                      {#if menuMemberRoleIds.has(role.id)}<Check size={13} class="text-accent" />{/if}
                    </span>
                    <span class="truncate" style={role.color ? `color:${role.color}` : ''}>
                      {role.icon_emoji ? role.icon_emoji + ' ' : ''}{role.name}
                    </span>
                  </ContextMenu.Item>
                {/each}
              </ContextMenu.SubContent>
            </ContextMenu.Sub>
          {/if}
          {#if canModerate && !isSelf}
            <ContextMenu.Item
              onSelect={() => kickMember(m)}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-warning transition-colors data-[highlighted]:bg-warning/10"
            >
              <UserX size={14} class="text-warning" /> Expulser
            </ContextMenu.Item>
            <ContextMenu.Item
              onSelect={() => openBan(m)}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-danger transition-colors data-[highlighted]:bg-danger/10"
            >
              <Gavel size={14} class="text-danger" /> Bannir
            </ContextMenu.Item>
            <div class="my-1 h-px bg-border/60"></div>
          {/if}
          <ContextMenu.Item
            onSelect={() => copyId(m.user_id)}
            class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
          >
            <Copy size={14} class="text-muted" /> Copier l'identifiant
          </ContextMenu.Item>
          <ContextMenu.Item
            onSelect={() => { void navigator.clipboard?.writeText(`https://krovara.com/users/${m.user_id}`); }}
            class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
          >
            <LinkIcon size={14} class="text-muted" /> Copier le lien
          </ContextMenu.Item>
          {#if !isSelf}
            <div class="my-1 h-px bg-border/60"></div>
            <ContextMenu.Item
              onSelect={() => alert('À venir (Ignorer)')}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
            >
              <EyeOff size={14} class="text-muted" /> Ignorer
            </ContextMenu.Item>
            <ContextMenu.Item
              onSelect={() => blockHandle(m.username)}
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-danger transition-colors data-[highlighted]:bg-danger/10"
            >
              <Ban size={14} class="text-danger" /> Bloquer
            </ContextMenu.Item>
          {/if}
        </ContextMenu.Content>
      </ContextMenu.Portal>
      </ContextMenu.Root>
      {/each}
    {/each}
    {#if !loading && members.length === 0 && !err}
      <p class="px-2 py-1 text-label text-muted">Aucun membre.</p>
    {/if}
  </div>
</aside>

<BgContextMenu bind:open={bg.open} x={bg.x} y={bg.y} items={bgItems} />

{#if banTarget}
  <Modal open={true} title={`Bannir ${banTarget.username}`} onclose={() => (banTarget = null)}>
    <div class="space-y-4">
      <p class="text-body text-muted">
        {banTarget.username} sera retiré du serveur et ne pourra plus le rejoindre.
      </p>
      <label class="block">
        <span class="text-label font-medium text-content">Raison (optionnel)</span>
        <input
          bind:value={banReason}
          maxlength="512"
          placeholder="ex. spam, comportement toxique…"
          class="mt-1 h-9 w-full rounded-md border border-border bg-base/50 px-2.5 text-body text-content outline-none focus:border-primary"
        />
      </label>
      <label class="flex cursor-pointer items-center gap-2.5">
        <input type="checkbox" bind:checked={banWipe} class="size-4 accent-danger" />
        <span class="text-body text-content">Supprimer tous ses messages dans le serveur</span>
      </label>
      <div class="flex justify-end gap-2 pt-1">
        <Button type="button" variant="ghost" onclick={() => (banTarget = null)}>Annuler</Button>
        <Button type="button" variant="danger" loading={banBusy} onclick={confirmBan}>Bannir</Button>
      </div>
    </div>
  </Modal>
{/if}

{#if profileTarget}
  <Modal open={true} title={profileTarget.nickname || profileTarget.username} onclose={() => (profileTarget = null)} wide flush>
    <FullProfile
      userId={profileTarget.user_id}
      name={profileTarget.nickname || profileTarget.username}
      username={profileTarget.username}
      avatarKey={profileTarget.avatar_key}
      availability={availabilityFor(profileTarget.user_id)}
      isSelf={profileTarget.user_id === selfId}
      oncall={() => call(profileTarget!.user_id)}
    />
  </Modal>
{/if}
