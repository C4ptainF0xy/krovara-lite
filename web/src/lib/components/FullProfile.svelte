<script lang="ts">
  import { onDestroy } from 'svelte';
  import { MessageSquare, UserPlus, Check, Phone, Link as LinkIcon, Users } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { api, authedObjectURL, ApiError } from '$lib/api';
  import { STATUS_META, type Availability } from '$lib/stores/status';
  import { sendRequest, friends } from '$lib/stores/friends';
  import { BADGES, sortBadges } from '$lib/badges';

  type ProfileLink = { label: string; url: string };
  type Props = {
    userId: string;
    name: string;
    username: string;
    avatarKey?: string | null;
    availability: Availability;
    isSelf?: boolean;
    oncall?: () => void;
  };
  let { userId, name, username, avatarKey, availability, isSelf = false, oncall }: Props = $props();

  type Public = {
    bio: string | null;
    pronouns: string | null;
    links: ProfileLink[];
    banner_key: string | null;
    badges?: string[] | null;
    created_at?: string;
  };
  type MutualUser = { id: string; username: string; display_name: string; avatar_key: string | null };
  type MutualSpace = { id: string; name: string; icon_key: string | null };
  type MutualGroup = { id: string; name: string | null; icon_key: string | null };

  let profile = $state<Public | null>(null);
  let mutualFriends = $state<MutualUser[]>([]);
  let mutualSpaces = $state<MutualSpace[]>([]);
  let mutualGroups = $state<MutualGroup[]>([]);
  let tab = $state<'activity' | 'friends' | 'servers' | 'groups'>('activity');

  const isFriend = $derived($friends.some((f) => f.id === userId));
  let friendState = $state<'idle' | 'sending' | 'sent' | 'error'>('idle');

  $effect(() => {
    let alive = true;
    api<Public>(`/api/users/${userId}/profile`).then((p) => alive && (profile = p)).catch(() => {});
    if (!isSelf) {
      api<{ friends: MutualUser[]; spaces: MutualSpace[]; groups: MutualGroup[] }>(`/api/users/${userId}/mutuals`)
        .then((m) => {
          if (!alive) return;
          mutualFriends = m.friends;
          mutualSpaces = m.spaces;
          mutualGroups = m.groups ?? [];
        })
        .catch(() => {});
    }
    return () => (alive = false);
  });

  async function addFriend() {
    if (friendState === 'sending' || friendState === 'sent') return;
    friendState = 'sending';
    try {
      await sendRequest(username);
      friendState = 'sent';
    } catch (e) {
      friendState = e instanceof ApiError && e.status === 409 ? 'sent' : 'error';
    }
  }

  let avatarUrl = $state<string | null>(null);
  let bannerUrl = $state<string | null>(null);
  const revokers: string[] = [];
  $effect(() => {
    if (!avatarKey) return;
    authedObjectURL(`/api/files/${avatarKey}`).then((u) => { avatarUrl = u; revokers.push(u); }).catch(() => {});
  });
  $effect(() => {
    const k = profile?.banner_key;
    if (!k) return;
    authedObjectURL(`/api/files/${k}`).then((u) => { bannerUrl = u; revokers.push(u); }).catch(() => {});
  });
  onDestroy(() => revokers.forEach((u) => URL.revokeObjectURL(u)));

  const initials = $derived((name || username).slice(0, 2).toUpperCase());
  const since = $derived(
    profile?.created_at
      ? new Date(profile.created_at).toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' })
      : null
  );

  function openDm() {
    void goto(`/app?tab=messages&dm=${userId}`);
  }
</script>

<div class="flex max-h-[80vh] w-full flex-col overflow-y-auto sm:h-[28rem] sm:flex-row sm:overflow-hidden">
  <div class="flex min-w-0 shrink-0 flex-col border-b border-border sm:w-2/5 sm:overflow-y-auto sm:border-b-0 sm:border-r">
    <div class="h-20 shrink-0 bg-gradient-to-r from-primary/40 to-brand/30 bg-cover bg-center"
         style={bannerUrl ? `background-image:url(${bannerUrl})` : ''}></div>
    <div class="px-4 pb-4">
      <div class="-mt-9 mb-2">
        <div class="relative inline-block">
          <div class="grid size-[4.5rem] place-items-center overflow-hidden rounded-full bg-elevated text-title font-semibold text-muted ring-4 ring-surface">
            {#if avatarUrl}<img src={avatarUrl} alt="" class="size-full object-cover" />{:else}{initials}{/if}
          </div>
          <span title={STATUS_META[availability].label}
                class="absolute -bottom-0.5 -right-0.5 size-4 rounded-full ring-4 ring-surface {STATUS_META[availability].dot}"></span>
        </div>
      </div>

      <p class="text-subtitle font-bold text-content">{name}</p>
      <p class="flex items-center gap-1.5 text-label text-muted">
        @{username}
        {#if profile?.pronouns}<span>· {profile.pronouns}</span>{/if}
      </p>

      {#if profile?.badges?.length}
        <div class="mt-2 flex flex-wrap gap-1.5">
          {#each sortBadges(profile.badges) as b (b)}
            <img src={BADGES[b].img} alt={BADGES[b].label} title={BADGES[b].label} class="size-6 object-contain" />
          {/each}
        </div>
      {/if}

      {#if !isSelf}
        <div class="mt-3 flex gap-2">
          <button type="button" onclick={openDm}
                  class="flex flex-1 items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-2 text-label font-medium text-white transition-colors hover:bg-primary-hover">
            <MessageSquare size={15} /> Message
          </button>
          {#if isFriend}
            <span class="grid size-9 place-items-center rounded-md border border-border text-success" title="Vous êtes amis"><Check size={16} /></span>
          {:else}
            <button type="button" onclick={addFriend} disabled={friendState === 'sending' || friendState === 'sent'}
                    title={friendState === 'sent' ? 'Demande envoyée' : 'Ajouter en ami'}
                    class="grid size-9 place-items-center rounded-md border border-border text-muted transition-colors hover:bg-elevated hover:text-accent {friendState === 'sent' ? 'text-success' : ''}">
              {#if friendState === 'sent'}<Check size={16} />{:else}<UserPlus size={16} />{/if}
            </button>
          {/if}
          {#if oncall}
            <button type="button" onclick={oncall} title="Appeler"
                    class="grid size-9 place-items-center rounded-md border border-border text-muted transition-colors hover:bg-elevated hover:text-success"><Phone size={16} /></button>
          {/if}
        </div>
      {/if}

      {#if profile?.bio}
        <div class="mt-4">
          <p class="mb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">À propos</p>
          <p class="whitespace-pre-wrap text-label text-content/85">{profile.bio}</p>
        </div>
      {/if}

      {#if profile?.links?.length}
        <div class="mt-3 space-y-1">
          {#each profile.links as link (link.url)}
            <a href={link.url} target="_blank" rel="noopener noreferrer"
               class="flex items-center gap-1.5 truncate text-label text-accent hover:underline">
              <LinkIcon size={13} class="shrink-0" /><span class="truncate">{link.label || link.url}</span>
            </a>
          {/each}
        </div>
      {/if}

      {#if since}
        <div class="mt-4">
          <p class="mb-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Membre depuis</p>
          <p class="text-label text-content/85">{since}</p>
        </div>
      {/if}
    </div>
  </div>

  <div class="flex min-w-0 flex-1 flex-col">
    <nav class="no-scrollbar flex shrink-0 gap-1 overflow-x-auto border-b border-border px-3">
      {#each [
        { k: 'activity', l: 'Activité' },
        { k: 'friends', l: `${mutualFriends.length} amis` },
        { k: 'servers', l: `${mutualSpaces.length} serveurs` },
        { k: 'groups', l: `${mutualGroups.length} groupes` }
      ] as t (t.k)}
        {#if t.k !== 'activity' && isSelf}{:else}
          <button type="button" onclick={() => (tab = t.k as typeof tab)}
                  class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-2 py-2.5 text-label transition-colors
                         {tab === t.k ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}">
            {t.l}
          </button>
        {/if}
      {/each}
    </nav>

    <div class="min-h-0 flex-1 overflow-y-auto p-3">
      {#if tab === 'activity'}
        <p class="grid h-full place-items-center text-center text-label text-muted">Aucune activité pour le moment.</p>
      {:else if tab === 'friends'}
        {#if mutualFriends.length === 0}
          <p class="grid h-full place-items-center text-label text-muted">Aucun ami en commun.</p>
        {:else}
          <div class="space-y-0.5">
            {#each mutualFriends as f (f.id)}
              <div class="flex items-center gap-2.5 rounded px-2 py-1.5 hover:bg-elevated/60">
                {#if f.avatar_key}
                  {#await authedObjectURL(`/api/files/${f.avatar_key}`) then src}
                    <img {src} alt="" class="size-8 rounded-full object-cover" />
                  {/await}
                {:else}
                  <span class="grid size-8 place-items-center rounded-full bg-elevated text-xs font-semibold text-content">{f.username.slice(0, 2).toUpperCase()}</span>
                {/if}
                <span class="truncate text-body text-content">{f.display_name}</span>
              </div>
            {/each}
          </div>
        {/if}
      {:else if tab === 'servers'}
        {#if mutualSpaces.length === 0}
          <p class="grid h-full place-items-center text-label text-muted">Aucun serveur en commun.</p>
        {:else}
          <div class="space-y-0.5">
            {#each mutualSpaces as sp (sp.id)}
              <div class="flex items-center gap-2.5 rounded px-2 py-1.5 hover:bg-elevated/60">
                {#if sp.icon_key && !sp.icon_key.startsWith(':')}
                  {#await authedObjectURL(`/api/files/${sp.icon_key}`) then src}
                    <img {src} alt="" class="size-8 rounded-lg object-cover" />
                  {/await}
                {:else}
                  <span class="grid size-8 place-items-center rounded-lg bg-elevated text-xs font-semibold text-content">{sp.name.slice(0, 2).toUpperCase()}</span>
                {/if}
                <span class="truncate text-body text-content">{sp.name}</span>
              </div>
            {/each}
          </div>
        {/if}
      {:else if tab === 'groups'}
        {#if mutualGroups.length === 0}
          <p class="grid h-full place-items-center text-label text-muted">Aucun groupe en commun.</p>
        {:else}
          <div class="space-y-0.5">
            {#each mutualGroups as g (g.id)}
              <div class="flex items-center gap-2.5 rounded px-2 py-1.5 hover:bg-elevated/60">
                {#if g.icon_key}
                  {#await authedObjectURL(`/api/files/${g.icon_key}`) then src}
                    <img {src} alt="" class="size-8 rounded-full object-cover" />
                  {/await}
                {:else}
                  <span class="grid size-8 place-items-center rounded-full bg-elevated text-xs font-semibold text-content"><Users size={15} /></span>
                {/if}
                <span class="truncate text-body text-content">{g.name || 'Groupe'}</span>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>
