<script lang="ts">
  import { tick } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    MessagesSquare,
    Flag,
    Trash2,
    Pencil,
    SmilePlus,
    Reply,
    CornerUpRight,
    Pin,
    Bookmark,
    BookmarkCheck,
    MoreHorizontal,
    Link2,
    Forward,
    Hash,
    Copy,
    Check,
    Mail,
    History,
    ListChecks,
    Bell,
    BellOff,
    Eraser,
    ShieldAlert,
    ChevronDown,
    Image as ImageIcon,
    Paperclip
  } from '@lucide/svelte';
  import { Popover } from 'bits-ui';
  import type { ChannelMessage } from '$lib/stores/messages';
  import { memberNames, membersBySpace } from '$lib/stores/members';
  import { pillsFor } from '$lib/stores/reactions';
  import { seenBy } from '$lib/stores/reads';
  import { blockedIds } from '$lib/stores/friends';
  import { renderMarkup, relativeTime, isEmojiOnly } from '$lib/render/markup';
  import EmojiInfoPopover from '$lib/components/EmojiInfoPopover.svelte';
  import { emojisBySpace, loadEmojis, emojiUrl } from '$lib/stores/emojis';
  import { checkUrls, verdict } from '$lib/stores/linksafety';
  import { authedObjectURL } from '$lib/api';
  import { peerStatus, selfAvailability, type Availability } from '$lib/stores/status';
  import PollsPanel from '$lib/components/PollsPanel.svelte';
  import { pollsByChannel } from '$lib/stores/polls';
  import Modal from '$lib/components/Modal.svelte';
  import InviteEmbed from '$lib/components/InviteEmbed.svelte';
  import LinkEmbed from '$lib/components/LinkEmbed.svelte';
  import { extractMediaLinks } from '$lib/render/links';
  import EditHistory from '$lib/components/EditHistory.svelte';
  import { presences } from '$lib/stores/presence';
  import { timeoutMember } from '$lib/stores/timeouts';
  import { rolesBySpace } from '$lib/stores/roles';
  import AttachmentView from './AttachmentView.svelte';
  import ProfileCard from './ProfileCard.svelte';
  import { clickOutside } from '$lib/actions/clickOutside';
  import { longpress } from '$lib/actions/longpress';

  type ByEmoji = Record<string, string[]>;

  type Props = {
    messages: ChannelMessage[];
    canModerate?: boolean;
    selfId?: string;
    mentionUsernames?: Set<string>;
    mentionRoles?: Map<string, string | null>;
    selfUsername?: string;
    reactions?: Record<string, ByEmoji>;
    reads?: Record<string, string>;
    canManage?: boolean;
    pinnedIds?: Set<string>;
    savedIds?: Set<string>;
    spaceId?: string;
    unreadAfterId?: string;
    recentEmojis?: string[];
    threadsByRoot?: Record<string, { id: string; reply_count: number }>;
    onthread?: (m: ChannelMessage) => void;
    onreport?: (m: ChannelMessage) => void;
    onforward?: (m: ChannelMessage) => void;
    onreplyelsewhere?: (m: ChannelMessage) => void;
    ondelete?: (m: ChannelMessage) => void;
    onedit?: (m: ChannelMessage, body: string) => void | Promise<void>;
    onreact?: (m: ChannelMessage, emoji: string) => void;
    onreply?: (m: ChannelMessage) => void;
    onpin?: (m: ChannelMessage, pinned: boolean) => void;
    onsave?: (m: ChannelMessage, saved: boolean) => void;
    onmarkunread?: (m: ChannelMessage) => void;
    onhistory?: (m: ChannelMessage) => void;
    ontask?: (m: ChannelMessage) => void;
    onfollow?: (m: ChannelMessage, follow: boolean) => void;
    followedIds?: Set<string>;
    onbulkdelete?: (ids: string[]) => void | Promise<void>;
    onpurge?: (m: ChannelMessage) => void;
    onloadmore?: () => Promise<void>;
    hasMore?: boolean;
    loadingMore?: boolean;
  };
  let {
    messages,
    canModerate = false,
    selfId,
    mentionUsernames,
    mentionRoles,
    selfUsername,
    reactions = {},
    reads = {},
    canManage = false,
    pinnedIds,
    savedIds,
    spaceId,
    unreadAfterId = '',
    recentEmojis = [],
    threadsByRoot = {},
    onthread,
    onreport,
    onforward,
    onreplyelsewhere,
    ondelete,
    onedit,
    onreact,
    onreply,
    onpin,
    onsave,
    onmarkunread,
    onhistory,
    ontask,
    onfollow,
    followedIds,
    onbulkdelete,
    onpurge,
    onloadmore,
    hasMore = false,
    loadingMore = false
  }: Props = $props();

  let emojiMap = $state<Map<string, string>>(new Map());
  $effect(() => {
    const sid = spaceId;
    if (!sid) {
      emojiMap = new Map();
      return;
    }
    const list = $emojisBySpace[sid];
    if (list === undefined) {
      void loadEmojis(sid).catch(() => {});
      return;
    }
    let cancelled = false;
    void Promise.all(
      list.map(async (e) => [e.name, await emojiUrl(e.file_key)] as const)
    ).then((entries) => {
      if (!cancelled) emojiMap = new Map(entries);
    });
    return () => {
      cancelled = true;
    };
  });

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

  const DEFAULT_EMOJIS = ['👍', '❤️', '😂', '🎉', '😮', '😢', '🔥', '✅'];
  const pickerEmojis = $derived.by(() => {
    const merged = [...recentEmojis.filter(Boolean)];
    for (const e of DEFAULT_EMOJIS) if (!merged.includes(e)) merged.push(e);
    return merged.slice(0, 12);
  });
  let pickerForId = $state<string | null>(null);

  const dividerIndex = $derived.by(() => {
    if (!unreadAfterId) return -1;
    const idx = messages.findIndex((m) => m.id === unreadAfterId);
    if (idx < 0 || idx >= messages.length - 1) return -1;
    return idx + 1;
  });

  function reactedLabel(users: string[]): string {
    const names = users.map((u) => $memberNames[u] ?? u.slice(0, 6));
    if (names.length === 0) return '';
    if (names.length === 1) return `${names[0]} a réagi`;
    if (names.length <= 3)
      return `${names.slice(0, -1).join(', ')} et ${names[names.length - 1]} ont réagi`;
    const rest = names.length - 3;
    return `${names.slice(0, 3).join(', ')} et ${rest} autre${rest > 1 ? 's' : ''} ont réagi`;
  }

  let menuForId = $state<string | null>(null);
  let menuDown = $state(false);
  let menuMaxH = $state(420);
  function decideMenuDir(anchorY: number) {
    const vh = window.innerHeight;
    const above = anchorY - 12;
    const below = vh - anchorY - 12;
    menuDown = below > above;
    menuMaxH = Math.max(180, Math.min(vh * 0.6, menuDown ? below : above));
  }
  let copied = $state<string | null>(null);

  let menuSection = $state<'copy' | 'perso' | null>(null);
  function toggleSection(s: 'copy' | 'perso') {
    menuSection = menuSection === s ? null : s;
  }

  let shiftHeld = $state(false);
  function onKeyChange(e: KeyboardEvent) {
    shiftHeld = e.shiftKey;
  }

  let selectionMode = $state(false);
  const selected = new SvelteSet<string>();
  let bulkBusy = $state(false);
  let bulkError = $state<string | null>(null);

  let selectionCancelEl = $state<HTMLButtonElement | null>(null);
  function enterSelection(m: ChannelMessage) {
    menuForId = null;
    selected.clear();
    selected.add(m.id);
    bulkError = null;
    selectionMode = true;
    void tick().then(() => selectionCancelEl?.focus());
  }
  function exitSelection() {
    selectionMode = false;
    selected.clear();
    bulkError = null;
  }
  function toggleSelect(id: string) {
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    bulkError = null;
  }
  async function confirmBulkDelete() {
    if (!selected.size || bulkBusy) return;
    bulkBusy = true;
    bulkError = null;
    try {
      await onbulkdelete?.([...selected]);
      exitSelection();
    } catch (e) {
      const status = (e as { status?: number })?.status;
      bulkError =
        status === 403
          ? 'Vous ne pouvez supprimer que vos propres messages.'
          : 'Suppression impossible. Réessaie.';
      console.error('bulk delete', e);
    } finally {
      bulkBusy = false;
    }
  }

  function messageLink(m: ChannelMessage): string {
    const base = typeof location !== 'undefined' ? location.origin : '';
    return `${base}/app/spaces/${spaceId ?? ''}/channels/${m.channelId}?m=${encodeURIComponent(m.id)}`;
  }
  function appLink(m: ChannelMessage): string {
    return `krovara://m/${spaceId ?? ''}/${m.channelId}/${m.id}`;
  }
  async function copyText(key: string, text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = key;
      setTimeout(() => {
        if (copied === key) copied = null;
        menuForId = null;
      }, 900);
    } catch {
      menuForId = null;
    }
  }
  function toggleMenu(id: string, e?: MouseEvent) {
    const opening = menuForId !== id;
    if (opening && e) {
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
      decideMenuDir(r.top);
    }
    menuForId = opening ? id : null;
    pickerForId = null;
    copied = null;
    menuSection = null;
  }
  function openContextMenu(e: MouseEvent, id: string) {
    if (selectionMode || editingId === id) return;
    if ((e.target as HTMLElement)?.closest('a')) return;
    e.preventDefault();
    decideMenuDir((e.currentTarget as HTMLElement).getBoundingClientRect().top);
    menuForId = id;
    pickerForId = null;
    copied = null;
    menuSection = null;
  }
  function forward(m: ChannelMessage) {
    menuForId = null;
    onforward?.(m);
  }
  function replyElsewhere(m: ChannelMessage) {
    menuForId = null;
    onreplyelsewhere?.(m);
  }

  function react(m: ChannelMessage, emoji: string) {
    pickerForId = null;
    onreact?.(m, emoji);
  }

  function repliedTo(m: ChannelMessage): ChannelMessage | undefined {
    if (!m.replyToId) return undefined;
    return messages.find((x) => x.id === m.replyToId);
  }

  type QuoteInfo = { kind: 'text' | 'image' | 'file' | 'missing'; author: string; text: string };
  const IMG_RE = /\.(png|jpe?g|gif|webp|avif|bmp|svg)(\?|#|$)/i;
  function quoteInfo(m: ChannelMessage): QuoteInfo {
    const parent = repliedTo(m);
    if (!parent) return { kind: 'missing', author: '', text: '' };
    const author = authorName(parent);
    const body = (parent.body ?? '').trim();
    if (body && body !== parent.oobUrl) {
      return { kind: 'text', author, text: body.replace(/\s+/g, ' ').slice(0, 100) };
    }
    const first = parent.oobs?.[0] ?? (parent.oobUrl ? { url: parent.oobUrl, name: parent.oobName } : undefined);
    if (first) {
      const count = parent.oobs?.length ?? 1;
      const isImg = IMG_RE.test(first.name ?? first.url);
      const text = count > 1 ? `${count} pièces jointes` : isImg ? 'Image' : first.name || 'Pièce jointe';
      return { kind: isImg ? 'image' : 'file', author, text };
    }
    return { kind: 'text', author, text: '' };
  }

  function jumpTo(id?: string) {
    if (!id || !scrollEl) return;
    const el = scrollEl.querySelector<HTMLElement>(`[data-mid="${CSS.escape(id)}"]`);
    if (!el) return;
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    el.classList.remove('msg-flash');
    void el.offsetWidth;
    el.classList.add('msg-flash');
    setTimeout(() => el.classList.remove('msg-flash'), 1300);
  }

  const lastId = $derived(messages.length ? messages[messages.length - 1].id : null);
  function seenNames(m: ChannelMessage): string {
    const author = m.fromResource || m.from;
    const names = seenBy(reads, m.id, selfId ?? '', author).map(
      (uid) => $memberNames[uid] ?? uid.slice(0, 6)
    );
    if (names.length === 0) return '';
    if (names.length <= 3) return names.join(', ');
    return `${names.slice(0, 3).join(', ')} +${names.length - 3}`;
  }

  const isMine = (m: ChannelMessage) => !!selfId && (m.fromResource || m.from) === selfId;

  let editingId = $state<string | null>(null);
  let draft = $state('');
  let editBusy = $state(false);

  function startEdit(m: ChannelMessage) {
    editingId = m.id;
    draft = m.body;
  }
  function cancelEdit() {
    editingId = null;
    draft = '';
  }
  async function commitEdit(m: ChannelMessage) {
    const body = draft.trim();
    if (!body || body === m.body) {
      cancelEdit();
      return;
    }
    editBusy = true;
    try {
      await onedit?.(m, body);
      cancelEdit();
    } finally {
      editBusy = false;
    }
  }
  function focusEnd(node: HTMLTextAreaElement) {
    node.focus();
    const n = node.value.length;
    node.setSelectionRange(n, n);
  }
  function onEditKeydown(e: KeyboardEvent, m: ChannelMessage) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void commitEdit(m);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      cancelEdit();
    }
  }

  function authorName(m: ChannelMessage): string {
    const id = m.fromResource || m.from;
    return $memberNames[id] ?? id;
  }
  const authorId = (m: ChannelMessage) => m.fromResource || m.from;

  const memberList = $derived($membersBySpace[spaceId ?? ''] ?? []);
  const memberByUid = $derived(new Map(memberList.map((m) => [m.user_id, m])));

  const mentionableRoleNames = $derived(
    ($rolesBySpace[spaceId ?? ''] ?? []).filter((r) => r.mentionable && !r.is_everyone).map((r) => r.name)
  );

  const mentionUserColors = $derived(
    new Map(
      memberList.filter(m => m.role_color).map(m => [m.username.toLowerCase(), m.role_color as string])
    )
  );

  let clickedMentionUser = $state<string | null>(null);

  function extractInvites(text: string | null | undefined): string[] {
    if (!text) return [];
    const matches = [...text.matchAll(/(?:krovara\.app\/join\/|\/join\/)([a-zA-Z0-9]+)/g)];
    return Array.from(new Set(matches.map(m => m[1])));
  }

  let avatarUrls = $state<Record<string, string>>({});
  $effect(() => {
    let cancelled = false;
    for (const m of messages) {
      const key = memberByUid.get(authorId(m))?.avatar_key;
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
    if (userId === selfId) return $selfAvailability === 'invisible' ? 'offline' : $selfAvailability;
    return $peerStatus[userId]?.availability ?? 'offline';
  }
  function presenceForAuthor(userId: string) {
    return $presences[`${userId}@krovara.local`] ?? null;
  }
  function callAuthor(userId: string) {
    (window as unknown as { krovaraCall?: (jid: string) => void }).krovaraCall?.(`${userId}@krovara.local`);
  }
  async function timeoutAuthor(userId: string, minutes: number) {
    if (!spaceId) return;
    try {
      await timeoutMember(spaceId, userId, minutes);
    } catch (e) {
      console.error('timeout', e);
    }
  }

  let scrollEl: HTMLDivElement | null = $state(null);
  let stickToBottom = $state(true);

  function onScroll() {
    if (!scrollEl) return;
    const distFromBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
    stickToBottom = distFromBottom < 80;
  }

  function onCopy(e: ClipboardEvent) {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || !scrollEl) return;
    const rowsEls = Array.from(scrollEl.querySelectorAll<HTMLElement>('[data-mid][data-author]'));
    const lines: string[] = [];
    for (const el of rowsEls) {
      if (!sel.containsNode(el, true)) continue;
      const body = el.querySelector<HTMLElement>('[data-msg-body]');
      const text = (body?.innerText ?? '').trim();
      if (text) lines.push(`${el.dataset.author}: ${text}`);
    }
    if (lines.length > 1) {
      e.clipboardData?.setData('text/plain', lines.join('\n'));
      e.preventDefault();
    }
  }

  let emojiPop = $state<{ src: string; name: string; fileKey?: string; isSticker?: boolean; x: number; y: number } | null>(null);
  let hoverTimer: ReturnType<typeof setTimeout> | null = null;
  function onMediaOver(e: MouseEvent) {
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
  function onMediaOut(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (t?.closest('img.inline-emoji') || t?.closest('img[data-sticker-key]')) {
      if (hoverTimer) clearTimeout(hoverTimer);
      emojiPop = null;
    }
  }

  async function loadOlder() {
    if (!scrollEl || !onloadmore || loadingMore) return;
    const prevH = scrollEl.scrollHeight;
    const prevTop = scrollEl.scrollTop;
    await onloadmore();
    await tick();
    if (scrollEl) scrollEl.scrollTop = prevTop + (scrollEl.scrollHeight - prevH);
  }

  let contentEl: HTMLDivElement | null = $state(null);
  let lastSeenId: string | null = null;

  function pinToBottom() {
    if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
  }

  $effect(() => {
    void messages.length;
    const last = messages[messages.length - 1];
    if (last && last.id !== lastSeenId) {
      lastSeenId = last.id;
      if (isMine(last)) stickToBottom = true;
    }
    if (!stickToBottom) return;
    tick().then(pinToBottom);
  });

  $effect(() => {
    if (!contentEl) return;
    const ro = new ResizeObserver(() => {
      if (stickToBottom) pinToBottom();
    });
    ro.observe(contentEl);
    return () => ro.disconnect();
  });

  const GROUP_GAP_MS = 5 * 60 * 1000;

  const visibleMessages = $derived(messages.filter((m) => !$blockedIds.has(m.fromResource || m.from)));

  const rows = $derived(
    visibleMessages.map((m, i) => {
      const prev = visibleMessages[i - 1];
      const author = m.fromResource || m.from;
      const prevAuthor = prev ? prev.fromResource || prev.from : null;
      const dayStart = !prev || prev.at.toDateString() !== m.at.toDateString();
      const grouped =
        !dayStart &&
        !!prev &&
        prevAuthor === author &&
        m.at.getTime() - prev.at.getTime() < GROUP_GAP_MS;
      return { m, name: authorName(m), head: !grouped, dayStart, dayLabel: fmtDay(m.at) };
    })
  );

  function fmtDay(d: Date): string {
    const today = new Date();
    const y = new Date(today);
    y.setDate(today.getDate() - 1);
    if (d.toDateString() === today.toDateString()) return "Aujourd'hui";
    if (d.toDateString() === y.toDateString()) return 'Hier';
    return d.toLocaleDateString('fr-FR', {
      weekday: 'long',
      day: 'numeric',
      month: 'long',
      year: d.getFullYear() === today.getFullYear() ? undefined : 'numeric'
    });
  }

  async function onBodyClick(e: MouseEvent) {
    const target = e.target as HTMLElement;
    const copyBtn = target.closest<HTMLElement>('[data-copy]');
    if (copyBtn) {
      const code = copyBtn.closest<HTMLElement>('[data-code]')?.dataset.code ?? '';
      try {
        await navigator.clipboard.writeText(code);
      } catch {
        return;
      }
      const label = copyBtn.querySelector('.copy-label');
      if (label) {
        const prev = label.textContent;
        label.textContent = 'Copié';
        setTimeout(() => {
          label.textContent = prev;
        }, 1500);
      }
      return;
    }
    const foldBtn = target.closest<HTMLElement>('[data-fold]');
    if (foldBtn) {
      const block = foldBtn.closest('.code-block');
      const open = block?.classList.toggle('code-open');
      foldBtn.textContent = open ? 'Replier' : 'Déplier';
      return;
    }
    const mentionSpan = target.closest<HTMLElement>('[data-mention-user]');
    if (mentionSpan) {
      clickedMentionUser = mentionSpan.dataset.mentionUser?.toLowerCase() ?? null;
      return;
    }
    const bad = target.closest<HTMLAnchorElement>('a[data-bad-link]');
    if (bad) {
      e.preventDefault();
      flaggedLink = { url: bad.getAttribute('href') ?? '', threat: bad.dataset.badLink ?? '' };
      return;
    }
    const link = target.closest<HTMLAnchorElement>('a[href^="http"]');
    if (link) {
      const href = link.getAttribute('href') ?? '';
      try {
        const u = new URL(href, location.href);
        if (u.host !== location.host) {
          e.preventDefault();
          flaggedLink = { url: href, threat: '' };
          return;
        }
      } catch {
      }
    }
    const spoiler = target.closest('.spoiler, .spoiler-block');
    if (spoiler) spoiler.classList.add('revealed');
  }

  let flaggedLink = $state<{ url: string; threat: string } | null>(null);

  async function scanLinks(root: HTMLElement) {
    const anchors = Array.from(root.querySelectorAll<HTMLAnchorElement>('a[href^="http"]'));
    const urls = anchors.map((a) => a.getAttribute('href') ?? '').filter(Boolean);
    if (!urls.length) return;
    await checkUrls(urls);
    for (const a of anchors) {
      const href = a.getAttribute('href') ?? '';
      const v = verdict(href);
      if (v) {
        a.dataset.badLink = v;
        a.classList.add('link-flagged');
      }
    }
  }

  function linkGuard(node: HTMLElement) {
    let pending: ReturnType<typeof setTimeout> | undefined;
    const rescan = () => {
      clearTimeout(pending);
      pending = setTimeout(() => void scanLinks(node), 200);
    };
    rescan();
    const obs = new MutationObserver(rescan);
    obs.observe(node, { childList: true, subtree: true });
    return {
      destroy() {
        clearTimeout(pending);
        obs.disconnect();
      }
    };
  }

  function openFlagged() {
    if (flaggedLink?.url) window.open(flaggedLink.url, '_blank', 'noopener,noreferrer');
    flaggedLink = null;
  }

  $effect(() => {
    const refresh = () => {
      scrollEl?.querySelectorAll<HTMLElement>('[data-ts]').forEach((el) => {
        const u = Number(el.dataset.ts);
        if (Number.isFinite(u)) el.textContent = relativeTime(u);
      });
    };
    const id = setInterval(refresh, 30_000);
    return () => clearInterval(id);
  });
  function codeActions(node: HTMLElement) {
    node.addEventListener('click', onBodyClick);
    return {
      destroy() {
        node.removeEventListener('click', onBodyClick);
      }
    };
  }

  function fmtTime(d: Date): string {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  function initials(name: string): string {
    return name.trim().slice(0, 2).toUpperCase() || '?';
  }
</script>

<svelte:window onkeydown={onKeyChange} onkeyup={onKeyChange} onblur={() => (shiftHeld = false)} />

<div class="flex min-h-0 flex-1 flex-col">
<div
  bind:this={scrollEl}
  onscroll={onScroll}
  onmouseover={onMediaOver}
  onmouseout={onMediaOut}
  oncopy={onCopy}
  use:codeActions
  use:linkGuard
  class="msg-scroll flex-1 overflow-y-auto px-4 py-4"
>
  <div bind:this={contentEl} class="flex min-h-full flex-col justify-end">
  {#if hasMore && onloadmore}
    <div class="mb-2 flex justify-center">
      <button
        type="button"
        onclick={loadOlder}
        disabled={loadingMore}
        class="rounded-full border border-border px-3 py-1 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content disabled:opacity-50"
      >
        {loadingMore ? 'Chargement…' : 'Charger les messages plus anciens'}
      </button>
    </div>
  {/if}
  {#snippet authorProfile(m: ChannelMessage)}
    {@const uid = authorId(m)}
    {@const mem = memberByUid.get(uid)}
    <ProfileCard
      name={authorName(m)}
      username={mem?.username ?? authorName(m)}
      avatarKey={mem?.avatar_key}
      availability={availabilityFor(uid)}
      game={presenceForAuthor(uid)}
      isSelf={uid === selfId}
      canModerate={canModerate}
      userId={uid}
      spaceId={spaceId}
      oncall={() => callAuthor(uid)}
      ontimeout={(min) => timeoutAuthor(uid, min)}
    />
  {/snippet}
  {#each rows as row, i (row.m.id)}
    {@const pills = pillsFor(reactions[row.m.id], selfId ?? '')}
    {@const rowMedia = extractMediaLinks(row.m.body)}
    {#if row.dayStart}
      <div class="relative my-3 flex items-center gap-2" role="separator" aria-label={row.dayLabel}>
        <span class="h-px flex-1 bg-border"></span>
        <span class="rounded-full bg-elevated px-2.5 py-0.5 text-[0.625rem] font-semibold uppercase tracking-wide text-muted">
          {row.dayLabel}
        </span>
        <span class="h-px flex-1 bg-border"></span>
      </div>
    {/if}
    {#if i === dividerIndex}
      <div
        class="relative my-3 flex items-center gap-2"
        role="separator"
        aria-label="Nouveaux messages"
      >
        <span class="h-px flex-1 bg-danger/40"></span>
        <span
          class="rounded-full bg-danger/15 px-2 py-0.5 text-[0.625rem] font-semibold uppercase tracking-wide text-danger"
        >
          Nouveaux messages
        </span>
      </div>
    {/if}
    <div
      data-mid={row.m.id}
      data-author={row.name}
      use:longpress
      oncontextmenu={(e) => openContextMenu(e, row.m.id)}
      onclick={selectionMode ? () => toggleSelect(row.m.id) : undefined}
      class="msg-row group relative flex gap-3 rounded px-2 transition-colors duration-150
             {row.head ? 'msg-head' : ''}
             {selectionMode ? 'cursor-pointer hover:bg-primary/5' : 'hover:bg-surface/50'}
             {selectionMode && selected.has(row.m.id) ? 'bg-primary/10' : ''}
             {menuForId === row.m.id || pickerForId === row.m.id ? 'z-40' : ''}
             {row.head ? 'mt-4 pt-0.5 first:mt-0' : 'mt-0.5'}"
    >
      {#if selectionMode}
        <div class="pointer-events-none flex shrink-0 items-center">
          <input
            type="checkbox"
            checked={selected.has(row.m.id)}
            tabindex="-1"
            aria-label="Sélectionner le message de {row.name}"
            class="size-4 accent-primary"
          />
        </div>
      {/if}
      {#if row.head}
        {@const headKey = memberByUid.get(authorId(row.m))?.avatar_key}
        <Popover.Root>
          <Popover.Trigger>
            {#snippet child({ props })}
              <button
                {...props}
                type="button"
                title={row.name}
                class="mt-0.5 grid size-9 shrink-0 place-items-center overflow-hidden rounded-full bg-elevated text-label font-semibold text-muted transition-opacity hover:opacity-90"
              >
                {#if headKey && avatarUrls[headKey]}
                  <img src={avatarUrls[headKey]} alt="" class="size-full object-cover" />
                {:else}
                  {initials(row.name)}
                {/if}
              </button>
            {/snippet}
          </Popover.Trigger>
          <Popover.Portal>
            <Popover.Content
              side="right"
              align="start"
              sideOffset={8}
              class="z-50 rounded-lg border border-border bg-surface shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
            >
              {@render authorProfile(row.m)}
            </Popover.Content>
          </Popover.Portal>
        </Popover.Root>
      {:else}
        <div class="w-9 shrink-0 select-none pt-0.5 text-right">
          <span
            class="text-[0.625rem] leading-5 text-muted opacity-0 transition-opacity duration-150 group-hover:opacity-100"
          >
            {fmtTime(row.m.at)}
          </span>
        </div>
      {/if}

      <div class="min-w-0 flex-1">
        {#if row.head}
          <div class="flex items-baseline gap-2">
            <Popover.Root>
              <Popover.Trigger>
                {#snippet child({ props })}
                  <button {...props} type="button" class="text-body font-semibold text-content transition-colors hover:underline">
                    {row.name}
                  </button>
                {/snippet}
              </Popover.Trigger>
              <Popover.Portal>
                <Popover.Content
                  side="right"
                  align="start"
                  sideOffset={8}
                  class="z-50 rounded-lg border border-border bg-surface shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
                >
                  {@render authorProfile(row.m)}
                </Popover.Content>
              </Popover.Portal>
            </Popover.Root>
            <span class="text-label text-muted">{fmtTime(row.m.at)}</span>
          </div>
        {/if}
        {#if row.m.replyToId}
          {@const q = quoteInfo(row.m)}
          <button
            type="button"
            onclick={() => jumpTo(row.m.replyToId)}
            class="mb-0.5 flex max-w-full items-center gap-1.5 truncate text-label text-muted transition-colors duration-150 hover:text-content"
          >
            <CornerUpRight size={13} class="shrink-0" />
            {#if q.kind === 'missing'}
              <span class="italic">message d'origine</span>
            {:else}
              <span class="shrink-0 font-medium text-content/70">{q.author}</span>
              {#if q.kind === 'image'}
                <ImageIcon size={13} class="shrink-0 text-muted" />
              {:else if q.kind === 'file'}
                <Paperclip size={13} class="shrink-0 text-muted" />
              {/if}
              <span class="truncate">{q.text}</span>
            {/if}
          </button>
        {/if}
        {#if editingId === row.m.id}
          <div class="mt-0.5">
            <textarea
              bind:value={draft}
              onkeydown={(e) => onEditKeydown(e, row.m)}
              rows="1"
              use:focusEnd
              class="w-full resize-y rounded border border-border bg-base/60 px-3 py-2 text-body text-content
                     outline-none focus:border-brand"
            ></textarea>
            <p class="mt-1 text-[0.6875rem] text-muted/70">
              <button type="button" class="text-accent hover:underline" onclick={() => commitEdit(row.m)} disabled={editBusy}>Enregistrer</button>
              ·
              <button type="button" class="hover:underline" onclick={cancelEdit}>annuler</button>
              · <kbd class="font-sans">Entrée</kbd> pour sauver, <kbd class="font-sans">Échap</kbd> pour annuler
            </p>
          </div>
        {:else}
          {#if row.m.body && row.m.body !== row.m.oobUrl}
            <div data-msg-body class="break-words text-body text-content/90 [&_a]:break-all {isEmojiOnly(row.m.body) ? 'msg-jumbo' : ''}">
              {@html renderMarkup(row.m.body, { usernames: mentionUsernames, selfUsername, roles: mentionRoles, userColors: mentionUserColors, emojis: emojiMap, emojiByKey: emojiByKeyMap, stripUrls: new Set(rowMedia.map((x) => x.url)) })}{#if row.m.edited}<span class="ml-1 select-none text-[0.6875rem] text-muted" title="Message modifié">(modifié)</span>{/if}
            </div>
          {/if}
          {#if row.m.oobs?.length}
            {#if row.m.oobs.length === 1}
              <AttachmentView url={row.m.oobs[0].url} name={row.m.oobs[0].name} sticker={row.m.oobs[0].name === 'sticker'} spoiler={row.m.oobs[0].spoiler} />
            {:else}
              <div class="mt-1 grid max-w-md grid-cols-2 gap-1.5">
                {#each row.m.oobs as o (o.url)}
                  <AttachmentView url={o.url} name={o.name} sticker={o.name === 'sticker'} spoiler={o.spoiler} />
                {/each}
              </div>
            {/if}
          {:else if row.m.oobUrl}
            <AttachmentView url={row.m.oobUrl} name={row.m.oobName} sticker={row.m.oobName === 'sticker'} />
          {/if}

          {@const invites = extractInvites(row.m.body)}
          {#if invites.length > 0}
            <div class="mt-2 flex flex-col gap-2">
              {#each invites as code}
                <InviteEmbed {code} />
              {/each}
            </div>
          {/if}
          {#if rowMedia.length > 0}
            <div class="mt-2 flex flex-col gap-2">
              {#each rowMedia as ml (ml.url)}
                <LinkEmbed link={ml} />
              {/each}
            </div>
          {/if}
        {/if}

        {#if pills.length}
          <div class="mt-1 flex flex-wrap gap-1">
            {#each pills as p (p.emoji)}
              <div class="group/react relative">
                <button
                  type="button"
                  onclick={() => onreact?.(row.m, p.emoji)}
                  title={reactedLabel(p.users)}
                  aria-label={reactedLabel(p.users)}
                  class="flex items-center gap-1 rounded-full border px-2 py-0.5 text-label transition-colors duration-150
                         {p.mine
                    ? 'border-primary bg-primary/15 text-content'
                    : 'border-border bg-elevated/50 text-muted hover:border-border-strong'}"
                >
                  <span>{p.emoji}</span><span class="tabular-nums">{p.count}</span>
                </button>
                <div
                  class="pointer-events-none absolute bottom-full left-0 z-20 mb-1 hidden w-max max-w-[15rem] group-hover/react:block group-focus-within/react:block"
                >
                  <div
                    class="rounded-md border border-border bg-overlay px-2.5 py-1.5 text-label text-content shadow-xl"
                  >
                    <span class="mr-1">{p.emoji}</span>{reactedLabel(p.users)}
                  </div>
                </div>
              </div>
            {/each}
          </div>
        {/if}

        {#if onthread && threadsByRoot[row.m.id]}
          {@const th = threadsByRoot[row.m.id]}
          <button
            type="button"
            onclick={() => onthread?.(row.m)}
            class="mt-1 flex items-center gap-1.5 rounded-md border border-border bg-elevated/40 px-2 py-1 text-label text-accent transition-colors duration-150 hover:border-border-strong hover:bg-elevated"
          >
            <MessagesSquare size={14} class="shrink-0" />
            <span class="font-medium"
              >{th.reply_count} réponse{th.reply_count > 1 ? 's' : ''}</span
            >
          </button>
        {/if}

        {#if row.m.id === lastId}
          {@const names = seenNames(row.m)}
          {#if names}
            <p class="mt-1 text-[0.6875rem] text-muted/80">Vu par {names}</p>
          {/if}
        {/if}
      </div>

      {#if editingId !== row.m.id && !selectionMode}
        {@const mine = isMine(row.m)}
        <div
          class="absolute right-2 top-0 hidden -translate-y-1/2 items-center gap-0.5 rounded-md
                 border border-border bg-overlay p-0.5 shadow-lg group-hover:flex
                 {pickerForId === row.m.id || menuForId === row.m.id ? '!flex' : ''}"
        >
          {#if onreact}
            <div class="relative">
              <button
                type="button"
                title="Réagir"
                onclick={() => (pickerForId = pickerForId === row.m.id ? null : row.m.id)}
                class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
              >
                <SmilePlus size={15} />
              </button>
              {#if pickerForId === row.m.id}
                <div
                  use:clickOutside={() => (pickerForId = null)}
                  class="absolute bottom-full right-0 z-30 mb-1 flex max-w-[13rem] flex-wrap gap-0.5 rounded-lg border border-border bg-overlay p-1 shadow-xl"
                >
                  {#each pickerEmojis as e (e)}
                    <button
                      type="button"
                      onclick={() => react(row.m, e)}
                      class="grid size-8 place-items-center rounded text-lg transition-colors hover:bg-elevated"
                    >
                      {e}
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
          {#if onreply}
            <button
              type="button"
              title="Répondre"
              onclick={() => onreply?.(row.m)}
              class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
            >
              <Reply size={15} />
            </button>
          {/if}
          {#if shiftHeld}
            {#if onthread}
              {@const inThread = !!threadsByRoot[row.m.id]}
              <button
                type="button"
                title={inThread ? 'Ouvrir le fil' : 'Créer un fil'}
                onclick={() => onthread?.(row.m)}
                class="grid size-7 place-items-center rounded transition-colors hover:bg-elevated
                       {inThread ? 'text-accent' : 'text-muted hover:text-content'}"
              >
                <MessagesSquare size={15} />
              </button>
            {/if}
            {#if onsave}
              {@const isSaved = savedIds?.has(row.m.id) ?? false}
              <button
                type="button"
                title={isSaved ? 'Retirer des favoris' : 'Enregistrer'}
                onclick={() => onsave?.(row.m, isSaved)}
                class="grid size-7 place-items-center rounded transition-colors hover:bg-elevated
                       {isSaved ? 'text-accent' : 'text-muted hover:text-content'}"
              >
                {#if isSaved}<BookmarkCheck size={15} />{:else}<Bookmark size={15} />{/if}
              </button>
            {/if}
            {#if canManage && onpin}
              {@const isPinned = pinnedIds?.has(row.m.id) ?? false}
              <button
                type="button"
                title={isPinned ? 'Désépingler' : 'Épingler'}
                onclick={() => onpin?.(row.m, isPinned)}
                class="grid size-7 place-items-center rounded transition-colors hover:bg-elevated
                       {isPinned ? 'text-accent' : 'text-muted hover:text-content'}"
              >
                <Pin size={15} class={isPinned ? 'fill-current' : ''} />
              </button>
            {/if}
            {#if mine && onedit}
              <button
                type="button"
                title="Modifier"
                onclick={() => startEdit(row.m)}
                class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
              >
                <Pencil size={15} />
              </button>
            {/if}
            {#if !mine && onreport}
              <button
                type="button"
                title="Signaler"
                onclick={() => onreport?.(row.m)}
                class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-warning"
              >
                <Flag size={15} />
              </button>
            {/if}
            {#if (mine || canModerate) && ondelete}
              <button
                type="button"
                title="Supprimer"
                onclick={() => ondelete?.(row.m)}
                class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-danger"
              >
                <Trash2 size={15} />
              </button>
            {/if}
          {/if}
            <div class="relative">
              <button
                type="button"
                title="Plus d'actions"
                onclick={(e) => toggleMenu(row.m.id, e)}
                class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
              >
                <MoreHorizontal size={15} />
              </button>
              {#if menuForId === row.m.id}
                <div
                  use:clickOutside={() => (menuForId = null)}
                  style="max-height: {menuMaxH}px"
                  class="absolute right-0 z-30 w-60 overflow-y-auto rounded-lg border border-border bg-overlay py-1 shadow-xl
                         {menuDown ? 'top-full mt-1' : 'bottom-full mb-1'}"
                >

                  {#if onreply}
                    <button type="button" onclick={() => { menuForId = null; onreply?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <Reply size={14} class="text-muted" /> Répondre
                    </button>
                  {/if}
                  {#if onthread}
                    {@const inThread = !!threadsByRoot[row.m.id]}
                    <button type="button" onclick={() => { menuForId = null; onthread?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <MessagesSquare size={14} class="text-muted" /> {inThread ? 'Ouvrir le fil' : 'Créer un fil'}
                    </button>
                  {/if}
                  {#if onforward}
                    <button type="button" onclick={() => forward(row.m)} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <Forward size={14} class="text-muted" /> Transférer…
                    </button>
                  {/if}
                  {#if onreplyelsewhere}
                    <button type="button" onclick={() => replyElsewhere(row.m)} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <CornerUpRight size={14} class="text-muted" /> Répondre ailleurs…
                    </button>
                  {/if}

                  {#if onsave || onfollow || onmarkunread || ontask || (canManage && onpin)}
                    <div class="my-1 h-px bg-border/60"></div>
                    <button type="button" onclick={() => toggleSection('perso')} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <Bookmark size={14} class="text-muted" /> Actions
                      <ChevronDown size={14} class="ml-auto text-muted transition-transform {menuSection === 'perso' ? 'rotate-180' : ''}" />
                    </button>
                    {#if menuSection === 'perso'}
                      {#if onsave}
                        {@const isSaved = savedIds?.has(row.m.id) ?? false}
                        <button type="button" onclick={() => { menuForId = null; onsave?.(row.m, isSaved); }} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                          <Bookmark size={14} class="text-muted" /> {isSaved ? 'Retirer des favoris' : 'Enregistrer'}
                        </button>
                      {/if}
                      {#if onfollow}
                        {@const isFollowed = followedIds?.has(row.m.id) ?? false}
                        <button type="button" onclick={() => { menuForId = null; onfollow?.(row.m, !isFollowed); }} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                          {#if isFollowed}<BellOff size={14} class="text-muted" /> Ne plus suivre{:else}<Bell size={14} class="text-muted" /> Suivre les réponses{/if}
                        </button>
                      {/if}
                      {#if onmarkunread}
                        <button type="button" onclick={() => { menuForId = null; onmarkunread?.(row.m); }} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                          <Mail size={14} class="text-muted" /> Marquer comme non-lu
                        </button>
                      {/if}
                      {#if ontask}
                        <button type="button" onclick={() => { menuForId = null; ontask?.(row.m); }} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                          <ListChecks size={14} class="text-muted" /> Créer une tâche
                        </button>
                      {/if}
                      {#if canManage && onpin}
                        {@const isPinned = pinnedIds?.has(row.m.id) ?? false}
                        <button type="button" onclick={() => { menuForId = null; onpin?.(row.m, isPinned); }} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                          <Pin size={14} class="text-muted" /> {isPinned ? 'Désépingler' : 'Épingler'}
                        </button>
                      {/if}
                    {/if}
                  {/if}

                  {#if (mine && onedit) || (row.m.edited && onhistory)}
                    <div class="my-1 h-px bg-border/60"></div>
                    {#if mine && onedit}
                      <button type="button" onclick={() => { menuForId = null; startEdit(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                        <Pencil size={14} class="text-muted" /> Modifier
                      </button>
                    {/if}
                    {#if row.m.edited && onhistory}
                      <button type="button" onclick={() => { menuForId = null; onhistory?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                        <History size={14} class="text-muted" /> Historique des modifications
                      </button>
                    {/if}
                  {/if}

                  <div class="my-1 h-px bg-border/60"></div>
                  <button type="button" onclick={() => toggleSection('copy')} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                    <Copy size={14} class="text-muted" /> Copier
                    <ChevronDown size={14} class="ml-auto text-muted transition-transform {menuSection === 'copy' ? 'rotate-180' : ''}" />
                  </button>
                  {#if menuSection === 'copy'}
                    <button type="button" onclick={() => copyText(`${row.m.id}:text`, row.m.body)} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                      {#if copied === `${row.m.id}:text`}<Check size={14} class="text-success" /> Texte copié{:else}Le texte{/if}
                    </button>
                    <button type="button" onclick={() => copyText(`${row.m.id}:link`, messageLink(row.m))} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                      {#if copied === `${row.m.id}:link`}<Check size={14} class="text-success" /> Lien copié{:else}Le lien{/if}
                    </button>
                    <button type="button" onclick={() => copyText(`${row.m.id}:applink`, appLink(row.m))} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                      {#if copied === `${row.m.id}:applink`}<Check size={14} class="text-success" /> Lien copié{:else}Le lien (appli){/if}
                    </button>
                    <button type="button" onclick={() => copyText(`${row.m.id}:id`, row.m.id)} class="flex w-full items-center gap-2 py-1.5 pl-9 pr-3 text-left text-label text-content transition-colors hover:bg-elevated">
                      {#if copied === `${row.m.id}:id`}<Check size={14} class="text-success" /> Identifiant copié{:else}L'identifiant{/if}
                    </button>
                  {/if}

                  {#if onbulkdelete}
                    <div class="my-1 h-px bg-border/60"></div>
                    <button type="button" onclick={() => enterSelection(row.m)} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-content transition-colors hover:bg-elevated">
                      <ListChecks size={14} class="text-muted" /> Sélectionner des messages
                    </button>
                  {/if}

                  {#if (!mine && onreport) || ((mine || canModerate) && ondelete) || (onpurge && canModerate && !mine)}
                    <div class="my-1 h-px bg-border/60"></div>
                    {#if !mine && onreport}
                      <button type="button" onclick={() => { menuForId = null; onreport?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-warning transition-colors hover:bg-warning/10">
                        <Flag size={14} /> Signaler
                      </button>
                    {/if}
                    {#if (mine || canModerate) && ondelete}
                      <button type="button" onclick={() => { menuForId = null; ondelete?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-danger transition-colors hover:bg-danger/10">
                        <Trash2 size={14} /> Supprimer
                      </button>
                    {/if}
                    {#if onpurge && canModerate && !mine}
                      <button type="button" onclick={() => { menuForId = null; onpurge?.(row.m); }} class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-label text-danger transition-colors hover:bg-danger/10">
                        <Eraser size={14} /> Purger les messages de {authorName(row.m)}
                      </button>
                    {/if}
                  {/if}
                </div>
              {/if}
            </div>
          </div>
      {/if}
    </div>
  {:else}
    <div class="grid h-full place-items-center">
      <div class="text-center text-muted">
        <div class="mx-auto mb-3 grid size-12 place-items-center rounded-2xl bg-surface">
          <MessagesSquare size={22} />
        </div>
        <p class="text-body">Aucun message pour l'instant.</p>
        <p class="text-label">Sois le premier à écrire ici.</p>
      </div>
    </div>
  {/each}
  </div>
</div>
  {#if selectionMode}
    <div class="flex items-center gap-3 border-t border-border bg-surface px-4 py-2.5">
      <span class="text-label text-muted" aria-live="polite">
        {selected.size} message{selected.size > 1 ? 's' : ''} sélectionné{selected.size > 1 ? 's' : ''}
      </span>
      {#if bulkError}
        <span class="text-label text-danger" role="alert">{bulkError}</span>
      {/if}
      <div class="ml-auto flex items-center gap-1.5">
        <button
          type="button"
          bind:this={selectionCancelEl}
          onclick={exitSelection}
          class="rounded-md px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          Annuler
        </button>
        <button
          type="button"
          disabled={!selected.size || bulkBusy}
          onclick={confirmBulkDelete}
          class="flex items-center gap-1.5 rounded-md bg-danger px-3 py-1.5 text-label font-medium text-white transition-[filter] duration-150 hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Trash2 size={14} /> Supprimer{selected.size ? ` (${selected.size})` : ''}
        </button>
      </div>
    </div>
  {/if}
</div>

<Modal open={!!flaggedLink} title={flaggedLink?.threat ? 'Lien signalé' : 'Tu quittes Krovara'} onclose={() => (flaggedLink = null)}>
  {#if flaggedLink}
    <div class="flex items-start gap-3">
      <ShieldAlert size={20} class="mt-0.5 shrink-0 {flaggedLink.threat ? 'text-danger' : 'text-warning'}" />
      <div class="min-w-0">
        {#if flaggedLink.threat}
          <p class="text-body text-content">
            Ce lien a été signalé comme malveillant ({flaggedLink.threat}) par URLhaus. Ne l'ouvre que si tu sais ce que tu fais.
          </p>
        {:else}
          <p class="text-body text-content">
            Ce lien mène en dehors de Krovara. Fais attention à ne pas te rendre sur des sites inconnus, et ne saisis jamais tes identifiants ailleurs que sur Krovara.
          </p>
        {/if}
        <p class="mt-2 break-all rounded-md border border-border bg-base/60 px-2.5 py-1.5 text-label text-muted">
          {flaggedLink.url}
        </p>
      </div>
    </div>
    <div class="mt-4 flex justify-end gap-2">
      <button
        type="button"
        onclick={() => (flaggedLink = null)}
        class="rounded-md px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        Annuler
      </button>
      <button
        type="button"
        onclick={openFlagged}
        class="rounded-md px-3 py-1.5 text-label font-medium text-white transition-[filter] duration-150 hover:brightness-110 {flaggedLink?.threat ? 'bg-danger' : 'bg-primary'}"
      >
        {flaggedLink?.threat ? 'Ouvrir quand même' : 'Continuer'}
      </button>
    </div>
  {/if}
</Modal>

{#if clickedMentionUser}
  {@const m = memberList.find(m => m.username.toLowerCase() === clickedMentionUser)}
  {#if m}
    <Modal open={true} title="Profil de {m.nickname ?? m.username}" onclose={() => clickedMentionUser = null}>
      <ProfileCard
        name={m.nickname ?? m.username}
        username={m.username}
        avatarKey={m.avatar_key}
        availability={availabilityFor(m.user_id)}
        game={presenceForAuthor(m.user_id)}
        isSelf={m.user_id === selfId}
        canModerate={canModerate}
        userId={m.user_id}
        spaceId={spaceId}
        oncall={() => { callAuthor(m.user_id); clickedMentionUser = null; }}
        ontimeout={(min) => { timeoutAuthor(m.user_id, min); clickedMentionUser = null; }}
      />
    </Modal>
  {/if}
{/if}

{#if emojiPop}
  <EmojiInfoPopover {...emojiPop} onclose={() => (emojiPop = null)} />
{/if}
