<script lang="ts">
  import { onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { Phone, TimerOff, Link as LinkIcon, Award, Pencil, UserPlus, Check, MessageSquare } from '@lucide/svelte';
  import { api, authedObjectURL, ApiError } from '$lib/api';
  import { STATUS_META, type Availability } from '$lib/stores/status';
  import { getKarma, vouch } from '$lib/stores/karma';
  import { sendRequest, friends } from '$lib/stores/friends';
  import { BADGES, sortBadges } from '$lib/badges';
  import Modal from '$lib/components/Modal.svelte';
  import FullProfile from '$lib/components/FullProfile.svelte';
  import { memberTimeout, liftTimeout, timeoutMember, type Timeout } from '$lib/stores/timeouts';
  import type { RichPresence } from '$lib/xmpp/presence';

  type ProfileLink = { label: string; url: string };
  type Props = {
    name: string;
    username: string;
    avatarKey?: string | null;
    availability: Availability;
    statusText?: string;
    game?: RichPresence | null;
    isSelf?: boolean;
    canModerate?: boolean;
    userId?: string;
    spaceId?: string;
    size?: 'sm' | 'lg';
    oncall?: () => void;
    ontimeout?: (minutes: number) => void;
  };
  let {
    name,
    username,
    avatarKey,
    availability,
    statusText,
    game,
    isSelf = false,
    canModerate = false,
    userId,
    spaceId,
    size = 'sm',
    oncall,
    ontimeout
  }: Props = $props();

  const lg = $derived(size === 'lg');

  let showFull = $state(false);

  const isFriend = $derived(!!userId && $friends.some((f) => f.id === userId));

  let friendState = $state<'idle' | 'sending' | 'sent' | 'error'>('idle');
  let friendErr = $state<string | null>(null);
  async function addFriend() {
    if (friendState === 'sending' || friendState === 'sent') return;
    friendState = 'sending';
    friendErr = null;
    try {
      await sendRequest(username);
      friendState = 'sent';
    } catch (e) {
      friendState = 'error';
      if (e instanceof ApiError && e.status === 409) friendErr = 'Déjà envoyée ou déjà amis.';
      else if (e instanceof ApiError && e.status === 403) friendErr = "N'accepte pas les demandes.";
      else friendErr = 'Échec.';
    }
  }

  let karma = $state<number | null>(null);
  let vouchBusy = $state(false);
  let vouchErr = $state<string | null>(null);
  let vouched = $state(false);
  $effect(() => {
    const uid = userId;
    const sid = spaceId;
    if (!uid || !sid) return;
    vouched = false;
    vouchErr = null;
    let alive = true;
    getKarma(sid, uid)
      .then((s) => {
        if (alive) karma = s;
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  });
  async function doVouch() {
    if (!userId || !spaceId) return;
    vouchBusy = true;
    vouchErr = null;
    try {
      karma = await vouch(spaceId, userId);
      vouched = true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        vouched = true;
        vouchErr = 'Déjà soutenu';
      } else if (e instanceof ApiError && e.status === 429) {
        vouchErr = 'Limite quotidienne atteinte';
      } else if (e instanceof ApiError && e.status === 403) {
        vouchErr = 'Compte trop récent pour soutenir';
      } else {
        vouchErr = 'Échec';
      }
    } finally {
      vouchBusy = false;
    }
  }

  type PublicProfile = {
    bio: string | null;
    pronouns: string | null;
    links: ProfileLink[];
    banner_key: string | null;
    badges?: string[] | null;
  };

  let profile = $state<PublicProfile | null>(null);
  $effect(() => {
    const id = userId;
    if (!id) return;
    let alive = true;
    api<PublicProfile>(`/api/users/${id}/profile`)
      .then((p) => {
        if (alive) profile = p;
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  });

  let bannerUrl = $state<string | null>(null);
  $effect(() => {
    const key = profile?.banner_key;
    if (!key) return;
    let made: string | null = null;
    authedObjectURL(`/api/files/${key}`)
      .then((u) => {
        bannerUrl = u;
        made = u;
      })
      .catch(() => {});
    return () => {
      if (made) URL.revokeObjectURL(made);
    };
  });
  onDestroy(() => {
    if (bannerUrl) URL.revokeObjectURL(bannerUrl);
  });

  const TIMEOUTS = [
    { v: 5, label: '5 min' },
    { v: 60, label: '1 h' },
    { v: 1440, label: '1 jour' },
    { v: 10080, label: '7 jours' }
  ];
  let pickingTimeout = $state(false);

  let mute = $state<Timeout | null>(null);
  let muteBusy = $state(false);
  $effect(() => {
    const uid = userId;
    const sid = spaceId;
    if (!uid || !sid || !canModerate || isSelf) {
      mute = null;
      return;
    }
    let alive = true;
    memberTimeout(sid, uid)
      .then((t) => {
        if (alive) mute = t;
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  });

  async function applyTimeout(minutes: number) {
    if (!userId || !spaceId) {
      ontimeout?.(minutes);
      pickingTimeout = false;
      return;
    }
    muteBusy = true;
    try {
      await timeoutMember(spaceId, userId, minutes);
      ontimeout?.(minutes);
      mute = await memberTimeout(spaceId, userId);
    } catch {
    } finally {
      muteBusy = false;
      pickingTimeout = false;
    }
  }

  async function lift() {
    if (!userId || !spaceId) return;
    muteBusy = true;
    try {
      await liftTimeout(spaceId, userId);
      mute = { active: false };
    } catch {
    } finally {
      muteBusy = false;
    }
  }

  function muteUntilLabel(): string {
    if (!mute?.expires_at) return '';
    return new Date(mute.expires_at).toLocaleString([], {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  let avatarUrl = $state<string | null>(null);
  $effect(() => {
    const key = avatarKey;
    if (!key) return;
    let made: string | null = null;
    authedObjectURL(`/api/files/${key}`)
      .then((u) => {
        avatarUrl = u;
        made = u;
      })
      .catch(() => {});
    return () => {
      if (made) URL.revokeObjectURL(made);
    };
  });
  onDestroy(() => {
    if (avatarUrl) URL.revokeObjectURL(avatarUrl);
  });

  const initials = $derived((name || username).slice(0, 2).toUpperCase());
</script>

<div class="overflow-hidden rounded-lg border border-border bg-surface shadow-xl {lg ? 'w-96' : 'w-64'}">
  <div
    class="bg-gradient-to-r from-primary/40 to-brand/30 bg-cover bg-center {lg ? 'h-24' : 'h-14'}"
    style={bannerUrl ? `background-image:url(${bannerUrl})` : ''}
  ></div>
  <div class="px-4 pb-4">
    <div class="flex items-end justify-between {lg ? '-mt-10' : '-mt-7'}">
      <div class="relative">
        <div class="grid place-items-center overflow-hidden rounded-full bg-elevated font-semibold text-muted ring-4 ring-surface {lg ? 'size-20 text-title' : 'size-14 text-subtitle'}">
          {#if avatarUrl}
            <img src={avatarUrl} alt="" class="size-full object-cover" />
          {:else}
            {initials}
          {/if}
        </div>
        <span
          title={STATUS_META[availability].label}
          class="absolute -bottom-0.5 -right-0.5 size-4 rounded-full ring-4 ring-surface {STATUS_META[availability].dot}"
        ></span>
      </div>
      {#if !isSelf && userId}
        <button
          type="button"
          onclick={() => goto(`/app?tab=messages&dm=${userId}`)}
          class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors duration-150 hover:bg-elevated hover:text-accent"
          title="Envoyer un message"
        >
          <MessageSquare size={16} />
        </button>
      {/if}
      {#if !isSelf && isFriend}
        <span
          class="grid size-8 place-items-center rounded-md border border-border text-success"
          title="Vous êtes amis"
        >
          <Check size={16} />
        </span>
      {:else if !isSelf}
        <button
          type="button"
          onclick={addFriend}
          disabled={friendState === 'sending' || friendState === 'sent'}
          class="grid size-8 place-items-center rounded-md border border-border transition-colors duration-150 hover:bg-elevated
                 {friendState === 'sent' ? 'text-success' : 'text-muted hover:text-accent'}"
          title={friendState === 'sent' ? 'Demande envoyée' : friendErr ?? 'Ajouter en ami'}
        >
          {#if friendState === 'sent'}<Check size={16} />{:else}<UserPlus size={16} />{/if}
        </button>
      {/if}
      {#if !isSelf && oncall}
        <button
          type="button"
          onclick={oncall}
          class="grid size-8 place-items-center rounded-md border border-border text-muted transition-colors duration-150 hover:bg-elevated hover:text-success"
          title="Appeler"
        >
          <Phone size={16} />
        </button>
      {/if}
    </div>
    {#if friendState === 'error' && friendErr}
      <p class="mt-1 text-label text-danger">{friendErr}</p>
    {/if}

    <p class="mt-2 flex items-center gap-1.5">
      <span class="truncate text-body font-semibold text-content">{name}</span>
      {#if profile?.pronouns}
        <span class="shrink-0 text-label text-muted">· {profile.pronouns}</span>
      {/if}
    </p>
    <p class="truncate text-label text-muted">@{username}</p>

    {#if profile?.badges?.length}
      <div class="mt-2 flex flex-wrap gap-1.5">
        {#each sortBadges(profile.badges) as b (b)}
          <img
            src={BADGES[b].img}
            alt={BADGES[b].label}
            title={BADGES[b].label}
            class="object-contain {lg ? 'size-7' : 'size-5'}"
          />
        {/each}
      </div>
    {/if}

    {#if isSelf && spaceId}
      <a
        href={`/app/spaces/${spaceId}/me`}
        class="mt-2 inline-flex items-center gap-1.5 rounded border border-border px-2 py-1 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content"
      >
        <Pencil size={13} /> Éditer mon profil ici
      </a>
    {/if}

    {#if profile?.bio}
      <p class="mt-2 whitespace-pre-wrap text-label text-content/80 {lg ? '' : 'line-clamp-4'}">{profile.bio}</p>
    {/if}

    {#if profile?.links?.length}
      <div class="mt-2 space-y-1">
        {#each profile.links as link (link.url)}
          <a
            href={link.url}
            target="_blank"
            rel="noopener noreferrer"
            class="flex items-center gap-1.5 truncate text-label text-accent transition-colors duration-150 hover:underline"
          >
            <LinkIcon size={13} class="shrink-0" />
            <span class="truncate">{link.label || link.url}</span>
          </a>
        {/each}
      </div>
    {/if}

    <div class="mt-3 space-y-1.5 border-t border-border pt-3">
      <p class="flex items-center gap-1.5 text-label">
        <span class="size-2 rounded-full {STATUS_META[availability].dot}"></span>
        <span class="text-muted">{STATUS_META[availability].label}</span>
      </p>
      {#if statusText}
        <p class="text-label text-content/80">{statusText}</p>
      {/if}
      {#if game}
        <p class="text-label text-accent">
          <span class="font-medium">{game.game}</span>{#if game.state}<span class="text-muted"> · {game.state}</span>{/if}
        </p>
      {/if}
    </div>

    {#if spaceId && userId && karma !== null}
      <div class="mt-3 flex items-center justify-between gap-2 border-t border-border pt-3">
        <span class="flex items-center gap-1.5 text-label text-muted">
          <Award size={14} class="text-accent" />
          <span class="font-medium text-content">{karma}</span> karma
        </span>
        {#if !isSelf}
          <button
            type="button"
            onclick={doVouch}
            disabled={vouchBusy || vouched}
            class="rounded border border-border px-2 py-1 text-label text-muted transition-colors duration-150 enabled:hover:border-border-strong enabled:hover:text-content disabled:opacity-50"
            title={vouchErr ?? 'Donner +1 karma'}
          >
            {vouched ? 'Soutenu' : 'Soutenir'}
          </button>
        {/if}
      </div>
      {#if vouchErr && !vouched}<p class="mt-1 text-label text-danger">{vouchErr}</p>{/if}
    {/if}

    {#if canModerate && !isSelf}
      <div class="mt-3 border-t border-border pt-3">
        {#if mute?.active && !pickingTimeout}
          <p class="mb-1.5 flex items-center gap-1.5 text-label text-warning">
            <TimerOff size={14} class="shrink-0" /> En sourdine jusqu'au {muteUntilLabel()}
          </p>
          <div class="flex gap-1.5">
            <button
              type="button"
              disabled={muteBusy}
              onclick={lift}
              class="flex-1 rounded-md border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-success/50 hover:text-success disabled:opacity-50"
            >
              Lever la sourdine
            </button>
            <button
              type="button"
              disabled={muteBusy}
              onclick={() => (pickingTimeout = true)}
              class="rounded-md border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-warning/50 hover:text-warning disabled:opacity-50"
            >
              Modifier
            </button>
          </div>
        {:else if pickingTimeout}
          <p class="mb-1.5 text-label text-muted">Durée de la sourdine</p>
          <div class="flex flex-wrap gap-1">
            {#each TIMEOUTS as t (t.v)}
              <button
                type="button"
                disabled={muteBusy}
                onclick={() => applyTimeout(t.v)}
                class="rounded border border-border px-2 py-1 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content disabled:opacity-50"
              >
                {t.label}
              </button>
            {/each}
            <button type="button" onclick={() => (pickingTimeout = false)} class="px-2 py-1 text-label text-muted hover:text-content">Annuler</button>
          </div>
        {:else}
          <button
            type="button"
            onclick={() => (pickingTimeout = true)}
            class="flex w-full items-center justify-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-warning/50 hover:text-warning"
          >
            <TimerOff size={15} /> Mettre en sourdine
          </button>
        {/if}
      </div>
    {/if}

    {#if !lg && userId}
      <button
        type="button"
        onclick={() => (showFull = true)}
        class="mt-3 w-full rounded-md border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content"
      >
        Voir le profil complet
      </button>
    {/if}
  </div>
</div>

{#if showFull && userId}
  <Modal open={showFull} title={name} onclose={() => (showFull = false)} wide flush>
    <FullProfile {userId} {name} {username} {avatarKey} {availability} {isSelf} {oncall} />
  </Modal>
{/if}
