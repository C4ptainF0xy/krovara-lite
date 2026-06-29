<script lang="ts">
  import { tick } from 'svelte';
  import { ArrowLeft, Pencil, Trash2, Copy } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { auth } from '$lib/stores/auth';
  import { friends, blockedIds } from '$lib/stores/friends';
  import { dmByPeer, openConversation, closeConversation, sendDm, editDm, deleteDm, type DmMessage } from '$lib/stores/dm';
  import { renderMarkup, isEmojiOnly } from '$lib/render/markup';
  import { emojiUrl } from '$lib/stores/emojis';
  import AttachmentView from '$lib/components/AttachmentView.svelte';
  import MessageInput from '$lib/components/MessageInput.svelte';
  import { Popover, ContextMenu } from 'bits-ui';
  import ProfileCard from '$lib/components/ProfileCard.svelte';
  import InviteEmbed from '$lib/components/InviteEmbed.svelte';
  import EmojiInfoPopover from '$lib/components/EmojiInfoPopover.svelte';
  import LinkEmbed from '$lib/components/LinkEmbed.svelte';
  import { extractMediaLinks } from '$lib/render/links';
  import { api, authedObjectURL } from '$lib/api';
  import { peerStatus } from '$lib/stores/status';

  let { peerId, onback }: { peerId: string; onback?: () => void } = $props();

  let sending = $state(false);
  let thread = $state<HTMLDivElement | null>(null);

  type PeerInfo = { id: string; username: string; avatar_key?: string | null };
  let peer = $state<PeerInfo | null>(null);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      const f = $friends.find((x) => x.id === peerId);
      if (f) {
        if (!cancelled) peer = { id: f.id, username: f.username, avatar_key: f.avatar_key };
        return;
      }
      try {
        const u = await api<{ id: string; username: string; avatar_key?: string | null }>(`/api/users/${peerId}/profile`);
        if (!cancelled) peer = { id: u.id, username: u.username, avatar_key: u.avatar_key };
      } catch {
        if (!cancelled) peer = { id: peerId, username: 'Utilisateur', avatar_key: null };
      }
    })();
    return () => {
      cancelled = true;
    };
  });
  const messages = $derived($blockedIds.has(peerId) ? [] : ($dmByPeer[peerId] ?? []));

  let emojiByKeyMap = $state<Map<string, string>>(new Map());
  let lastHadKeys = false;
  $effect(() => {
    const keys = new Set<string>();
    for (const m of messages) {
      if (!m.body) continue;
      for (const mt of m.body.matchAll(/<:[a-z0-9_]{2,32}:([a-f0-9-]{36})>/g)) keys.add(mt[1]);
    }
    if (keys.size === 0) {
      if (lastHadKeys) {
        emojiByKeyMap = new Map();
        lastHadKeys = false;
      }
      return;
    }
    lastHadKeys = true;
    let cancelled = false;
    void Promise.all(Array.from(keys).map(async (k) => [k, await emojiUrl(k)] as const)).then(
      (entries) => {
        if (!cancelled) emojiByKeyMap = new Map(entries);
      }
    );
    return () => {
      cancelled = true;
    };
  });

  $effect(() => {
    const id = peerId;
    openConversation(id);
    return () => closeConversation();
  });

  function timeOf(d: Date) {
    return new Date(d).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }
  function initials(name: string) {
    return name.slice(0, 2).toUpperCase();
  }

  function extractInvites(text: string | null | undefined): string[] {
    if (!text) return [];
    const matches = [...text.matchAll(/(?:\/join\/)([a-zA-Z0-9]+)/g)];
    return Array.from(new Set(matches.map((m) => m[1])));
  }

  const selfId = $derived($auth.user?.id ?? '');

  let atBottom = true;
  function onScroll() {
    if (!thread) return;
    atBottom = thread.scrollHeight - thread.scrollTop - thread.clientHeight < 120;
  }
  $effect(() => {
    messages.length;
    if (!thread) return;
    const el = thread;
    void tick().then(() => {
      el.scrollTop = el.scrollHeight;
      atBottom = true;
    });
  });
  $effect(() => {
    if (!thread) return;
    const el = thread;
    const ro = new ResizeObserver(() => {
      if (atBottom) el.scrollTop = el.scrollHeight;
    });
    if (el.firstElementChild) ro.observe(el.firstElementChild);
    return () => ro.disconnect();
  });

  async function submit(body: string) {
    if (!body || sending) return;
    sending = true;
    try {
      await sendDm(peerId, body);
    } finally {
      sending = false;
    }
  }

  let editingId = $state<string | null>(null);
  let editDraft = $state('');

  function startEdit(m: DmMessage) {
    editingId = m.id;
    editDraft = m.body;
  }
  function cancelEdit() {
    editingId = null;
    editDraft = '';
  }
  async function commitEdit(m: DmMessage) {
    const body = editDraft.trim();
    if (!body || body === m.body) {
      cancelEdit();
      return;
    }
    await editDm(peerId, m, body).catch(() => {});
    cancelEdit();
  }
  async function removeMessage(m: DmMessage) {
    await deleteDm(peerId, m).catch(() => {});
  }
  function copyText(m: DmMessage) {
    void navigator.clipboard?.writeText(m.body);
  }

  let emojiPop = $state<{ src: string; name: string; fileKey?: string; isSticker?: boolean; x: number; y: number } | null>(null);
  let hoverTimer: ReturnType<typeof setTimeout> | null = null;
  function onThreadOver(e: MouseEvent) {
    const t = e.target as HTMLElement;
    const emoji = t?.closest('img.inline-emoji') as HTMLImageElement | null;
    const sticker = t?.closest('img[data-sticker-key]') as HTMLImageElement | null;
    const el = emoji ?? sticker;
    if (!el) return;
    if (hoverTimer) clearTimeout(hoverTimer);
    const rect = el.getBoundingClientRect();
    hoverTimer = setTimeout(() => {
      emojiPop = {
        src: el.src,
        name: emoji ? (el.dataset.emojiName ?? el.alt.replace(/:/g, '')) : (el.alt ?? ''),
        fileKey: emoji ? el.dataset.emojiKey : el.dataset.stickerKey,
        isSticker: !!sticker,
        x: rect.left,
        y: rect.top
      };
    }, 350);
  }
  function onThreadOut(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (t?.closest('img.inline-emoji') || t?.closest('img[data-sticker-key]')) {
      if (hoverTimer) clearTimeout(hoverTimer);
      emojiPop = null;
    }
  }
</script>

<div class="flex h-full min-h-0">
<section class="flex h-full min-h-0 min-w-0 flex-1 flex-col">
  {#if !peer}
    <div class="grid flex-1 place-items-center text-label text-muted">{$t('dm.pick')}</div>
  {:else}
    {#snippet peerCardContent()}
      <ProfileCard
        userId={peer!.id}
        name={peer!.username}
        username={peer!.username}
        avatarKey={peer!.avatar_key}
        availability={$peerStatus[peer!.id]?.availability || 'offline'}
      />
    {/snippet}
    {#snippet msgAvatar(key: string | null, who: string)}
      {#if key}
        {#await authedObjectURL(`/api/files/${key}`) then src}
          <img {src} alt={who} class="size-10 shrink-0 rounded-full object-cover" />
        {:catch}
          <div class="grid size-10 shrink-0 place-items-center rounded-full bg-elevated text-body font-medium text-muted">{initials(who)}</div>
        {/await}
      {:else}
        <div class="grid size-10 shrink-0 place-items-center rounded-full bg-elevated text-body font-medium text-muted">{initials(who)}</div>
      {/if}
    {/snippet}
    <header class="flex items-center gap-3 border-b border-border px-4 py-3 shadow-sm z-10 shrink-0">
      {#if onback}
        <button
          type="button"
          onclick={onback}
          class="grid size-8 shrink-0 place-items-center rounded-md text-muted transition-colors hover:bg-elevated hover:text-content md:hidden"
          title={$t('dm.back')}
        >
          <ArrowLeft size={18} />
        </button>
      {/if}
      <Popover.Root>
        <Popover.Trigger class="flex items-center gap-3 text-left hover:opacity-80 transition-opacity">
          {#if peer.avatar_key}
            {#await authedObjectURL(`/api/files/${peer.avatar_key}`) then src}
              <img {src} alt={peer.username} class="size-8 shrink-0 rounded-full object-cover" />
            {/await}
          {:else}
            <span class="grid size-8 shrink-0 place-items-center rounded-full bg-elevated text-label font-medium text-muted">{initials(peer.username)}</span>
          {/if}
          <span class="min-w-0 flex-1 truncate text-body font-semibold text-content">{peer.username}</span>
        </Popover.Trigger>
        <Popover.Content align="start" sideOffset={4} class="p-0 border-none">
          {@render peerCardContent()}
        </Popover.Content>
      </Popover.Root>
    </header>

    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div bind:this={thread} onscroll={onScroll} onmouseover={onThreadOver} onmouseout={onThreadOut} class="min-h-0 flex-1 overflow-y-auto">
      {#if messages.length === 0}
        <div class="flex flex-col items-center justify-center h-full px-4 text-center">
          <div class="mb-4 grid size-16 place-items-center rounded-full bg-elevated text-muted">
            <span class="text-xl font-bold">{initials(peer.username)}</span>
          </div>
          <h2 class="text-subtitle font-bold text-content mb-2">C'est le début de l'historique !</h2>
          <p class="text-body text-muted max-w-sm">Voici le début de ta conversation directe avec {peer.username}.</p>
        </div>
      {:else}
        <div class="py-4">
          {#each messages as m (m.id)}
            {@const who = m.mine ? ($auth.user?.username ?? 'Moi') : peer!.username}
            {@const key = m.mine ? ($auth.user?.avatar_key ?? null) : (peer!.avatar_key ?? null)}
            {@const invites = extractInvites(m.body)}
            {@const media = extractMediaLinks(m.body)}
            <div class="group relative flex gap-4 px-4 py-1 hover:bg-black/5 mt-3">
              {#if editingId !== m.id}
                <div class="absolute -top-3 right-3 z-10 hidden items-center gap-0.5 rounded-lg border border-border bg-overlay p-0.5 shadow-lg group-hover:flex">
                  <button type="button" title="Copier le texte" onclick={() => copyText(m)} class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content">
                    <Copy size={15} />
                  </button>
                  {#if m.mine}
                    <button type="button" title="Modifier" onclick={() => startEdit(m)} class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content">
                      <Pencil size={15} />
                    </button>
                    <button type="button" title="Supprimer" onclick={() => removeMessage(m)} class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-danger">
                      <Trash2 size={15} />
                    </button>
                  {/if}
                </div>
              {/if}
              {#if m.mine}
                <div class="mt-0.5">{@render msgAvatar(key, who)}</div>
              {:else}
                <Popover.Root>
                  <Popover.Trigger class="mt-0.5 shrink-0 rounded-full transition-opacity hover:opacity-80">
                    {@render msgAvatar(key, who)}
                  </Popover.Trigger>
                  <Popover.Content align="start" sideOffset={4} class="p-0 border-none">
                    {@render peerCardContent()}
                  </Popover.Content>
                </Popover.Root>
              {/if}
              <div class="min-w-0 flex-1">
                <div class="flex items-baseline gap-2">
                  {#if m.mine}
                    <span class="text-body font-medium text-content">{who}</span>
                  {:else}
                    <Popover.Root>
                      <Popover.Trigger class="text-body font-medium text-content transition-colors hover:underline">{who}</Popover.Trigger>
                      <Popover.Content align="start" sideOffset={4} class="p-0 border-none">
                        {@render peerCardContent()}
                      </Popover.Content>
                    </Popover.Root>
                  {/if}
                  <span class="text-[0.6875rem] text-muted">{timeOf(m.at)}</span>
                </div>
                {#if editingId === m.id}
                  <!-- svelte-ignore a11y_autofocus -->
                  <textarea
                    bind:value={editDraft}
                    rows="1"
                    autofocus
                    onkeydown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); void commitEdit(m); }
                      else if (e.key === 'Escape') cancelEdit();
                    }}
                    class="mt-0.5 w-full resize-none rounded-md border border-border bg-elevated px-2 py-1 text-body text-content focus:border-border-strong focus:outline-none"
                  ></textarea>
                  <div class="mt-1 flex gap-2 text-[0.6875rem] text-muted">
                    <button type="button" class="hover:text-content" onclick={() => commitEdit(m)}>Enregistrer</button>
                    <button type="button" class="hover:text-content" onclick={cancelEdit}>Annuler</button>
                    <span>· Entrée pour valider · Échap pour annuler</span>
                  </div>
                {:else}
                  <ContextMenu.Root>
                    <ContextMenu.Trigger>
                      {#snippet child({ props })}
                        <div {...props} class="text-body text-content/90 break-words [&_a]:text-brand [&_a:hover]:underline mt-0.5 {isEmojiOnly(m.body) ? 'msg-jumbo' : ''}">
                          {@html renderMarkup(m.body, { emojiByKey: emojiByKeyMap, stripUrls: new Set(media.map((x) => x.url)) })}{#if m.edited}<span class="ml-1 text-[0.625rem] text-muted">(modifié)</span>{/if}
                        </div>
                      {/snippet}
                    </ContextMenu.Trigger>
                    <ContextMenu.Portal>
                      <ContextMenu.Content class="z-50 w-44 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in">
                        <ContextMenu.Item onSelect={() => copyText(m)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                          <Copy size={14} class="text-muted" /> Copier le texte
                        </ContextMenu.Item>
                        {#if m.mine}
                          <ContextMenu.Item onSelect={() => startEdit(m)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                            <Pencil size={14} class="text-muted" /> Modifier
                          </ContextMenu.Item>
                          <ContextMenu.Item onSelect={() => removeMessage(m)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-danger transition-colors data-[highlighted]:bg-danger/10">
                            <Trash2 size={14} class="text-danger" /> Supprimer
                          </ContextMenu.Item>
                        {/if}
                      </ContextMenu.Content>
                    </ContextMenu.Portal>
                  </ContextMenu.Root>
                {/if}
                {#if m.oobs && m.oobs.length > 0}
                  <div class="mt-2 flex flex-col gap-2">
                    {#each m.oobs as oob}
                      <AttachmentView url={oob.url} name={oob.name} sticker={oob.name === 'sticker'} />
                    {/each}
                  </div>
                {/if}
                {#if invites.length > 0}
                  <div class="mt-2 flex flex-col gap-2">
                    {#each invites as ic (ic)}
                      <InviteEmbed code={ic} />
                    {/each}
                  </div>
                {/if}
                {#if media.length > 0}
                  <div class="mt-2 flex flex-col gap-2">
                    {#each media as ml (ml.url)}
                      <LinkEmbed link={ml} />
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <MessageInput
      placeholder={$t('dm.placeholder', { name: peer.username })}
      onsend={submit}
      onattach={async (files, captions, spoilers) => {
        if (!files.length) return;
        try {
          const oobs: { url: string; name?: string; spoiler?: boolean }[] = [];
          for (let i = 0; i < files.length; i++) {
            const form = new FormData();
            form.append('file', files[i]);
            const dto = await api<{ id: string }>('/api/files?kind=attachment', { method: 'POST', body: form });
            const url = `/api/files/${dto.id}`;
            oobs.push({ url, name: captions?.[i]?.trim() || files[i].name, spoiler: spoilers?.[i] || undefined });
          }
          if (oobs.length) await sendDm(peerId, oobs[0].url, { oobs });
        } catch (e) {
          console.error('dm attach failed', e);
        }
      }}
      ongif={async (gif) => {
        try {
          await sendDm(peerId, gif.url, { oobs: [{ url: gif.url, name: gif.name }] });
        } catch (e) {
          console.error('gif send failed', e);
        }
      }}
      onsticker={async (sticker) => {
        const url = `/api/files/${sticker.file_key}`;
        try {
          await sendDm(peerId, url, { oobs: [{ url, name: 'sticker' }] });
        } catch (e) {
          console.error('sticker send failed', e);
        }
      }}
    />
  {/if}
</section>

{#if peer}
  <aside class="hidden w-72 shrink-0 overflow-y-auto border-l border-border bg-surface/40 p-3 xl:block">
    <ProfileCard
      userId={peer.id}
      name={peer.username}
      username={peer.username}
      avatarKey={peer.avatar_key}
      availability={$peerStatus[peer.id]?.availability || 'offline'}
    />
  </aside>
{/if}
</div>

{#if emojiPop}
  <EmojiInfoPopover {...emojiPop} onclose={() => (emojiPop = null)} />
{/if}
