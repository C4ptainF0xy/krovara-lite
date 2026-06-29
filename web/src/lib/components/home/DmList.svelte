<script lang="ts">
  import { onMount } from 'svelte';
  import { MessageSquare, UserPlus, Copy, Link as LinkIcon, Ban, User, Users, Plus } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { authedObjectURL } from '$lib/api';
  import { friends, loadFriends, sendRequest, blockHandle } from '$lib/stores/friends';
  import { dmUnread, dmByPeer } from '$lib/stores/dm';
  import { myGroups, groupUnread, loadMyGroups, createGroup } from '$lib/stores/groups';
  import { memberNames } from '$lib/stores/members';
  import { peerStatus } from '$lib/stores/status';
  import { Popover, ContextMenu } from 'bits-ui';
  import ProfileCard from '$lib/components/ProfileCard.svelte';
  import FullProfile from '$lib/components/FullProfile.svelte';
  import Modal from '$lib/components/Modal.svelte';

  let { selected = null, onpick, onpickgroup }: { selected?: string | null; onpick: (id: string) => void; onpickgroup?: (id: string) => void } =
    $props();

  onMount(() => {
    void loadFriends();
    void loadMyGroups();
  });

  let createOpen = $state(false);
  let createSel = $state<Set<string>>(new Set());
  let creating = $state(false);
  function toggleSel(id: string) {
    const n = new Set(createSel);
    if (n.has(id)) n.delete(id); else n.add(id);
    createSel = n;
  }
  async function doCreateGroup() {
    if (createSel.size === 0 || creating) return;
    creating = true;
    try {
      const g = await createGroup([...createSel]);
      createOpen = false;
      createSel = new Set();
      onpickgroup?.(g.id);
    } finally {
      creating = false;
    }
  }
  function groupTitle(g: { name: string | null; member_count?: number }) {
    return g.name || `Groupe de ${g.member_count ?? 0}`;
  }

  function initials(name: string) {
    return name.slice(0, 2).toUpperCase();
  }

  const convos = $derived.by(() => {
    const friendIds = new Set($friends.map((f) => f.id));
    const items = $friends.map((f) => ({ id: f.id, name: f.username, avatar_key: f.avatar_key, since: f.since }));
    for (const id of Object.keys($dmByPeer)) {
      if (friendIds.has(id)) continue;
      items.push({ id, name: $memberNames[id] ?? 'Utilisateur', avatar_key: null, since: undefined });
    }
    const keyOf = (c: { id: string; since?: string }) => {
      const msgs = $dmByPeer[c.id];
      const last = msgs && msgs.length ? new Date(msgs[msgs.length - 1].at).getTime() : 0;
      const since = c.since ? new Date(c.since).getTime() : 0;
      return Math.max(last, since);
    };
    return items.sort((a, b) => keyOf(b) - keyOf(a));
  });

  const friendIdSet = $derived(new Set($friends.map((f) => f.id)));
  let profileTarget = $state<{ id: string; name: string; avatar_key: string | null } | null>(null);
  function copy(text: string) {
    void navigator.clipboard?.writeText(text);
  }
</script>

<div class="flex h-full min-h-0 flex-col">
  <header class="flex items-center gap-2 border-b border-border px-4 py-3">
    <MessageSquare size={18} class="text-muted" />
    <h2 class="flex-1 text-body font-semibold text-content">{$t('dm.title')}</h2>
    <button type="button" onclick={() => (createOpen = true)} title="Créer un groupe" class="grid size-7 place-items-center rounded text-muted hover:bg-elevated hover:text-content"><Plus size={18} /></button>
  </header>
  <div class="min-h-0 flex-1 overflow-y-auto p-2">
    {#if $myGroups.length > 0}
      <p class="px-2 pb-1 pt-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Groupes</p>
      {#each $myGroups as g (g.id)}
        {@const gu = $groupUnread[g.id] ?? 0}
        <button type="button" onclick={() => onpickgroup?.(g.id)}
                class="group flex w-full items-center gap-3 rounded px-2 py-1.5 text-left transition-colors {selected === g.id ? 'bg-elevated text-content' : 'text-muted hover:bg-elevated/50 hover:text-content'}">
          <span class="relative grid size-8 shrink-0 place-items-center rounded-full bg-elevated text-muted">
            {#if g.icon_key}
              {#await authedObjectURL(`/api/files/${g.icon_key}`) then src}<img {src} alt="" class="size-8 rounded-full object-cover" />{/await}
            {:else}<Users size={16} />{/if}
            {#if gu > 0}<span class="absolute -right-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full border-2 border-overlay bg-brand px-1 text-[9px] font-bold text-white">{gu > 9 ? '9+' : gu}</span>{/if}
          </span>
          <span class="min-w-0 flex-1 truncate text-body font-medium">{groupTitle(g)}</span>
        </button>
      {/each}
      <p class="px-2 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Messages privés</p>
    {/if}
    {#if convos.length === 0 && $myGroups.length === 0}
      <p class="px-3 py-6 text-label text-muted">{$t('dm.empty.list')}</p>
    {:else}
      {#each convos as c (c.id)}
        {@const unread = $dmUnread[c.id] ?? 0}
        <ContextMenu.Root>
          <ContextMenu.Trigger>
            {#snippet child({ props })}
              <button
                {...props}
                type="button"
                onclick={() => onpick(c.id)}
                class="group flex w-full items-center gap-3 rounded px-2 py-1.5 text-left transition-colors
                       {selected === c.id ? 'bg-elevated text-content' : 'text-muted hover:bg-elevated/50 hover:text-content'}"
              >
                <div class="relative flex shrink-0 items-center justify-center" onclick={(e) => e.stopPropagation()}>
                  <Popover.Root>
                    <Popover.Trigger class="flex shrink-0 items-center justify-center rounded-full hover:opacity-80 transition-opacity">
                      {#if c.avatar_key}
                        {#await authedObjectURL(`/api/files/${c.avatar_key}`) then src}
                          <img {src} alt={c.name} class="size-8 rounded-full object-cover" />
                        {/await}
                      {:else}
                        <span class="grid size-8 place-items-center rounded-full bg-elevated text-xs font-semibold text-content">{initials(c.name)}</span>
                      {/if}
                      {#if unread > 0}
                        <span class="absolute -right-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full border-2 border-overlay bg-brand px-1 text-[9px] font-bold text-white pointer-events-none">{unread > 9 ? '9+' : unread}</span>
                      {/if}
                    </Popover.Trigger>
                    <Popover.Content align="start" sideOffset={4} class="p-0 border-none">
                      <ProfileCard
                        userId={c.id}
                        name={c.name}
                        username={c.name}
                        avatarKey={c.avatar_key}
                        availability={$peerStatus[c.id]?.availability || 'offline'}
                      />
                    </Popover.Content>
                  </Popover.Root>
                </div>
                <span class="min-w-0 flex-1 truncate text-body font-medium">{c.name}</span>
              </button>
            {/snippet}
          </ContextMenu.Trigger>
          <ContextMenu.Portal>
            <ContextMenu.Content class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in">
              <ContextMenu.Item
                onSelect={() => (profileTarget = { id: c.id, name: c.name, avatar_key: c.avatar_key })}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                <User size={14} class="text-muted" /> Voir le profil
              </ContextMenu.Item>
              <ContextMenu.Item
                onSelect={() => onpick(c.id)}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                <MessageSquare size={14} class="text-muted" /> Envoyer un message
              </ContextMenu.Item>
              {#if !friendIdSet.has(c.id)}
                <ContextMenu.Item
                  onSelect={() => void sendRequest(c.name).catch(() => {})}
                  class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
                >
                  <UserPlus size={14} class="text-muted" /> Ajouter en ami
                </ContextMenu.Item>
              {/if}
              <ContextMenu.Item
                onSelect={() => copy(c.id)}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                <Copy size={14} class="text-muted" /> Copier l'identifiant
              </ContextMenu.Item>
              <ContextMenu.Item
                onSelect={() => copy(`https://krovara.com/users/${c.id}`)}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated"
              >
                <LinkIcon size={14} class="text-muted" /> Copier le lien
              </ContextMenu.Item>
              <div class="my-1 h-px bg-border/60"></div>
              <ContextMenu.Item
                onSelect={() => void blockHandle(c.name).catch(() => {})}
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-danger transition-colors data-[highlighted]:bg-danger/10"
              >
                <Ban size={14} class="text-danger" /> Bloquer
              </ContextMenu.Item>
            </ContextMenu.Content>
          </ContextMenu.Portal>
        </ContextMenu.Root>
      {/each}
    {/if}
  </div>
</div>

{#if profileTarget}
  <Modal open={true} title={profileTarget.name} onclose={() => (profileTarget = null)} wide flush>
    <FullProfile
      userId={profileTarget.id}
      name={profileTarget.name}
      username={profileTarget.name}
      avatarKey={profileTarget.avatar_key}
      availability={$peerStatus[profileTarget.id]?.availability || 'offline'}
    />
  </Modal>
{/if}

{#if createOpen}
  <Modal open={true} title="Nouveau groupe" onclose={() => (createOpen = false)}>
    <p class="mb-3 text-label text-muted">Sélectionne des amis (max 9 + toi).</p>
    <div class="max-h-72 space-y-0.5 overflow-y-auto">
      {#each $friends as f (f.id)}
        {@const on = createSel.has(f.id)}
        <button type="button" onclick={() => toggleSel(f.id)} disabled={!on && createSel.size >= 9}
                class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left hover:bg-elevated/60 disabled:opacity-40">
          {#if f.avatar_key}
            {#await authedObjectURL(`/api/files/${f.avatar_key}`) then src}<img {src} alt="" class="size-8 rounded-full object-cover" />{/await}
          {:else}
            <span class="grid size-8 place-items-center rounded-full bg-elevated text-xs font-semibold text-content">{initials(f.username)}</span>
          {/if}
          <span class="min-w-0 flex-1 truncate text-body text-content">{f.username}</span>
          <span class="grid size-4 place-items-center rounded border {on ? 'border-primary bg-primary text-white' : 'border-border'}">{#if on}✓{/if}</span>
        </button>
      {:else}
        <p class="px-2 py-4 text-label text-muted">Ajoute des amis pour créer un groupe.</p>
      {/each}
    </div>
    <div class="mt-4 flex justify-end gap-2">
      <button type="button" onclick={() => (createOpen = false)} class="rounded-md border border-border px-3 py-1.5 text-label text-muted hover:text-content">Annuler</button>
      <button type="button" onclick={doCreateGroup} disabled={createSel.size === 0 || creating} class="rounded-md bg-primary px-3 py-1.5 text-label font-medium text-white hover:bg-primary-hover disabled:opacity-50">Créer ({createSel.size})</button>
    </div>
  </Modal>
{/if}
