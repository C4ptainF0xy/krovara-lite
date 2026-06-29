<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import { get } from 'svelte/store';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { api, authedDataURL, ApiError } from '$lib/api';
  import {
    render as renderExport,
    mimeFor,
    downloadFile,
    authorIdOf,
    type ExportEntry,
    type ExportFormat
  } from '$lib/export/conversation';
  import { auth } from '$lib/stores/auth';
  import { channelsBySpace, spaces, setChannelLock, type Channel } from '$lib/stores/spaces';
  import { myTimeout } from '$lib/stores/timeouts';
  import { setNotifSetting, markChannelMentionsRead, type NotifLevel } from '$lib/stores/inbox';
  import { announce } from '$lib/stores/announce';
  import { Popover } from 'bits-ui';
  import { messages, removeMessage, expandChannelHistory, type ChannelMessage } from '$lib/stores/messages';
  import { xmppState } from '$lib/xmpp/client';
  import { joinMUC, leaveMUC, sendMessage, sendReactions, sendDisplayed, sendChatState, fetchHistory, fetchHistoryBefore } from '$lib/xmpp/muc';
  import { typing } from '$lib/stores/typing';
  import { publishStatus } from '$lib/stores/status';
  import { reactions, myEmojis } from '$lib/stores/reactions';
  import { recentReactions, rememberReaction } from '$lib/stores/recentReactions';
  import { readMarkers } from '$lib/stores/reads';
  import {
    readState,
    loadReadState,
    markRead as markChannelRead,
    markUnread as markChannelUnread
  } from '$lib/stores/readstate';
  import { pinsByChannel, loadPins, pinMessage, unpinMessage } from '$lib/stores/pins';
  import { savedIds, ensureSavesLoaded, saveMessage, unsaveMessage } from '$lib/stores/saves';
  import { forwardDraft, setForward, clearForward } from '$lib/stores/forward';
  import { memberNames, membersBySpace } from '$lib/stores/members';
  import { rolesBySpace, loadRoles } from '$lib/stores/roles';
  import { activeRoom, joinVoice, leaveVoice } from '$lib/voip/room';
  import {
    sfuState,
    cameraOn,
    screenOn,
    micOn,
    toggleMic,
    enableCamera,
    disableCamera,
    enableScreenShare,
    disableScreenShare
  } from '$lib/voip/sfu';
  import MessageList from '$lib/components/MessageList.svelte';
  import MessageInput from '$lib/components/MessageInput.svelte';
  import ThreadPanel from '$lib/components/ThreadPanel.svelte';
  import {
    threadsByChannel,
    loadThreads,
    createThread,
    type Thread
  } from '$lib/stores/threads';
  import VideoGrid from '$lib/components/VideoGrid.svelte';
  import PinsPanel from '$lib/components/PinsPanel.svelte';
  import PollsPanel from '$lib/components/PollsPanel.svelte';
  import { pollsByChannel } from '$lib/stores/polls';
  import Modal from '$lib/components/Modal.svelte';
  import EditHistory from '$lib/components/EditHistory.svelte';
  import QuickAccess from '$lib/components/QuickAccess.svelte';
  import BgContextMenu from '$lib/components/BgContextMenu.svelte';
  import AttachmentView from '$lib/components/AttachmentView.svelte';
  import { Button, Input } from '$lib/ui';
  import { createTask } from '$lib/stores/tasks';
  import {
    Hash,
    Volume2,
    Mic,
    MicOff,
    Video,
    VideoOff,
    MonitorUp,
    PhoneOff,
    Paperclip,
    Pin,
    CheckCheck,
    Lock,
    Unlock,
    Megaphone,
    ShieldAlert,
    TimerOff,
    Bell,
    BellOff,
    Check,
    Download,
    BarChart3,
    ExternalLink,
    MoreVertical
  } from '@lucide/svelte';

  const id = $derived(page.params.id ?? '');
  const cid = $derived(page.params.cid ?? '');
  const isPopout = $derived(page.url.searchParams.get('popout') === '1');
  const channel = $derived<Channel | undefined>(
    id ? ($channelsBySpace[id]?.data ?? []).find((c) => c.id === cid) : undefined
  );
  const isVoice = $derived(channel?.type === 'voice');
  const channelMessages = $derived(cid ? ($messages[cid] ?? []) : []);

  let loadingMore = $state(false);
  let historyExhausted = $state<Set<string>>(new Set());
  const hasMoreHistory = $derived(!!cid && channelMessages.length > 0 && !historyExhausted.has(cid));

  async function loadMoreHistory() {
    if (!cid || loadingMore) return;
    const cur = channelMessages;
    if (cur.length === 0) return;
    const oldest = cur.find((m) => m.fromHistory) ?? cur[0];
    const channel = cid;
    loadingMore = true;
    expandChannelHistory(channel);
    try {
      const { complete } = await fetchHistoryBefore(channel, oldest.id, 50);
      await tick();
      const grew = ($messages[channel] ?? []).length > cur.length;
      if (complete || !grew) historyExhausted = new Set(historyExhausted).add(channel);
    } catch (e) {
      console.warn('load more history failed', e);
    } finally {
      loadingMore = false;
    }
  }

  const memberList = $derived($membersBySpace[id] ?? []);
  const mentionUsernames = $derived(new Set(memberList.map((m) => m.username.toLowerCase())));
  const mentionables = $derived(
    memberList.map((m) => ({ username: m.username, name: m.nickname || m.username }))
  );

  let rolesLoadedFor = '';
  $effect(() => {
    if (id && rolesLoadedFor !== id) {
      rolesLoadedFor = id;
      void loadRoles(id).catch(() => {});
    }
  });
  const mentionRoles = $derived(
    new Map(
      ($rolesBySpace[id] ?? [])
        .filter((r) => r.mentionable && !r.is_everyone)
        .map((r) => [r.name.toLowerCase(), r.color] as [string, string | null])
    )
  );
  const mentionableRoleNames = $derived(
    ($rolesBySpace[id] ?? []).filter((r) => r.mentionable && !r.is_everyone).map((r) => r.name)
  );

  let joinedFor: string | null = null;

  async function joinIfReady() {
    if (!cid || $xmppState !== 'online') return;
    if (joinedFor === cid) return;
    if (joinedFor && joinedFor !== cid) {
      try {
        await leaveMUC(joinedFor);
      } catch {}
    }
    try {
      await joinMUC(cid);
      joinedFor = cid;
      publishStatus();
      void fetchHistory(cid, 50);
    } catch (e) {
      console.warn('join MUC failed', e);
    }
  }

  $effect(() => {
    void cid;
    void $xmppState;
    void isVoice;
    void joinIfReady();
  });

  onDestroy(() => {
    if (joinedFor) {
      const c = joinedFor;
      joinedFor = null;
      void leaveMUC(c);
    }
  });

  let replyTarget = $state<ChannelMessage | null>(null);
  let inputCmp = $state<ReturnType<typeof MessageInput> | null>(null);
  function startReply(m: ChannelMessage) {
    replyTarget = m;
  }
  function authorOf(m: ChannelMessage): string {
    const uid = m.fromResource || m.from;
    return $memberNames[uid] ?? uid;
  }
  function replyPreviewOf(m: ChannelMessage): string {
    const body = (m.body ?? '').trim();
    if (body && body !== m.oobUrl) return body.replace(/\s+/g, ' ').slice(0, 80);
    const first = m.oobs?.[0]?.name ?? m.oobName;
    if (first === 'sticker') return 'Sticker';
    const n = m.oobs?.length ?? (m.oobUrl ? 1 : 0);
    return n > 1 ? `${n} pièces jointes` : n === 1 ? 'Pièce jointe' : '';
  }

  function firstAttachment(m: ChannelMessage): { url: string; name?: string } | null {
    if (m.oobs?.length) return { url: m.oobs[0].url, name: m.oobs[0].name };
    if (m.oobUrl) return { url: m.oobUrl, name: m.oobName };
    return null;
  }
  function bodyText(m: ChannelMessage): string {
    const body = (m.body ?? '').trim();
    if (!body) return '';
    if (body === m.oobUrl || body === m.oobs?.[0]?.url) return '';
    return body;
  }

  let lastSentAt = $state(0);
  let slowmodeLeft = $state(0);
  let slowmodeTimer: ReturnType<typeof setInterval> | null = null;
  function startSlowmodeCountdown(seconds: number) {
    slowmodeLeft = seconds;
    if (slowmodeTimer) clearInterval(slowmodeTimer);
    slowmodeTimer = setInterval(() => {
      slowmodeLeft -= 1;
      if (slowmodeLeft <= 0 && slowmodeTimer) {
        clearInterval(slowmodeTimer);
        slowmodeTimer = null;
      }
    }, 1000);
  }
  onDestroy(() => {
    if (slowmodeTimer) clearInterval(slowmodeTimer);
  });

  async function handleSend(text: string) {
    if (!cid) return;
    const sm = channel?.slowmode_seconds ?? 0;
    if (sm > 0 && !canModerate) {
      const elapsed = (Date.now() - lastSentAt) / 1000;
      if (elapsed < sm) {
        startSlowmodeCountdown(Math.ceil(sm - elapsed));
        return;
      }
    }
    const replyToId = replyTarget?.id;
    replyTarget = null;
    clearForward();
    await sendMessage(cid, text, { replyToId });
    lastSentAt = Date.now();
  }

  async function handleEdit(m: ChannelMessage, body: string) {
    if (!cid) return;
    await sendMessage(cid, body, { replaceId: m.originId ?? m.id });
  }

  async function handleAttach(files: File[], captions?: string[], spoilers?: boolean[]) {
    if (!cid || !files.length) return;
    try {
      const oobs: { url: string; desc: string; spoiler?: boolean }[] = [];
      for (let i = 0; i < files.length; i++) {
        const file = files[i];
        const form = new FormData();
        form.append('file', file);
        const dto = await api<{ id: string }>('/api/files?kind=attachment', {
          method: 'POST',
          body: form
        });
        const url = `/api/files/${dto.id}`;
        oobs.push({ url, desc: captions?.[i]?.trim() || file.name, spoiler: spoilers?.[i] || undefined });
      }
      if (!oobs.length) return;
      await sendMessage(cid, oobs[0].url, { oobs });
    } catch (e) {
      console.error('attach failed', e);
      const msg =
        e instanceof ApiError && e.status === 413
          ? 'Fichier trop volumineux.'
          : e instanceof ApiError && e.status === 415
            ? 'Type de fichier non autorisé.'
            : "Échec de l'envoi de la pièce jointe.";
      showAttachError(msg);
    }
  }

  let attachErr = $state<string | null>(null);
  let attachErrTimer: ReturnType<typeof setTimeout> | null = null;
  function showAttachError(msg: string) {
    attachErr = msg;
    announce(msg, true);
    if (attachErrTimer) clearTimeout(attachErrTimer);
    attachErrTimer = setTimeout(() => (attachErr = null), 6000);
  }

  async function handleGif(gif: { url: string; name: string }) {
    if (!cid) return;
    try {
      await sendMessage(cid, gif.url, { oobs: [{ url: gif.url, desc: gif.name }] });
    } catch (e) {
      console.error('gif send failed', e);
    }
  }

  async function handleSticker(sticker: { file_key: string; name: string }) {
    if (!cid) return;
    const url = `/api/files/${sticker.file_key}`;
    try {
      await sendMessage(cid, url, { oobs: [{ url, desc: 'sticker' }] });
    } catch (e) {
      console.error('sticker send failed', e);
    }
  }

  function handleTyping() {
    if (cid && $xmppState === 'online') void sendChatState(cid, 'composing');
  }

  let dragActive = $state(false);
  let dragDepth = 0;
  function onDragEnter(e: DragEvent) {
    if (e.dataTransfer?.types?.includes('Files')) {
      dragDepth++;
      dragActive = true;
    }
  }
  function onDragLeave() {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) dragActive = false;
  }
  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    dragDepth = 0;
    const fs = Array.from(e.dataTransfer?.files ?? []);
    if (fs.length) inputCmp?.stageFiles(fs);
  }

  const typers = $derived(
    ($typing[cid] ?? [])
      .filter((u) => u !== $auth.user?.id)
      .map((u) => $memberNames[u] ?? u.slice(0, 6))
  );

  async function handleReact(m: ChannelMessage, emoji: string) {
    if (!cid || !$auth.user) return;
    const cur = myEmojis(m.id, $auth.user.id);
    const adding = !cur.includes(emoji);
    const next = adding ? [...cur, emoji] : cur.filter((e) => e !== emoji);
    if (adding) rememberReaction(emoji);
    await sendReactions(cid, m.id, next);
  }

  let pinsOpen = $state(false);
  let pollsOpen = $state(false);
  const pollCount = $derived(($pollsByChannel[cid] ?? []).length);
  const pinnedIds = $derived(new Set(($pinsByChannel[cid] ?? []).map((p) => p.archive_id)));

  $effect(() => {
    if (!cid || isVoice) return;
    void loadPins(cid).catch(() => {});
  });
  $effect(() => {
    void ensureSavesLoaded().catch(() => {});
  });

  async function handlePin(m: ChannelMessage, pinned: boolean) {
    if (!cid) return;
    try {
      if (pinned) await unpinMessage(cid, m.id);
      else await pinMessage(cid, m.id);
    } catch (e) {
      console.error('pin', e);
    }
  }
  async function handleSave(m: ChannelMessage, isSaved: boolean) {
    if (!cid) return;
    try {
      if (isSaved) await unsaveMessage(m.id);
      else await saveMessage(cid, m.id);
    } catch (e) {
      console.error('save', e);
    }
  }

  function jumpToMessage(archiveId: string) {
    pinsOpen = false;
    const el = document.querySelector<HTMLElement>(`[data-mid="${CSS.escape(archiveId)}"]`);
    if (!el) return;
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    el.classList.remove('msg-flash');
    void el.offsetWidth;
    el.classList.add('msg-flash');
    setTimeout(() => el.classList.remove('msg-flash'), 1300);
  }

  $effect(() => {
    if (!cid || isVoice) return;
    void loadThreads(cid).catch(() => {});
  });
  const threadList = $derived(cid ? ($threadsByChannel[cid] ?? []) : []);
  const threadsByRoot = $derived.by(() => {
    const out: Record<string, Thread> = {};
    for (const t of threadList) out[t.root_archive_id] = t;
    return out;
  });
  let activeThreadId = $state<string | null>(null);
  const activeThread = $derived(
    activeThreadId ? (threadList.find((t) => t.id === activeThreadId) ?? null) : null
  );

  async function openThread(m: ChannelMessage) {
    if (!cid) return;
    const existing = threadsByRoot[m.id];
    if (existing) {
      activeThreadId = existing.id;
      return;
    }
    const title = (m.body.trim().slice(0, 60) || 'Fil de discussion').replace(/\s+/g, ' ');
    try {
      const created = await createThread(cid, m.id, title);
      activeThreadId = created.id;
    } catch (e) {
      console.error('create thread', e);
    }
  }

  $effect(() => {
    void cid;
    activeThreadId = null;
  });

  let forwardTarget = $state<ChannelMessage | null>(null);
  let citeMode = $state<'forward' | 'reply'>('forward');
  const textChannels = $derived(
    ($channelsBySpace[id]?.data ?? []).filter((c) => c.type !== 'voice')
  );
  function openForward(m: ChannelMessage) {
    citeMode = 'forward';
    forwardTarget = m;
  }
  function openReplyElsewhere(m: ChannelMessage) {
    citeMode = 'reply';
    forwardTarget = m;
  }
  function quotedBody(m: ChannelMessage): string {
    const body = (m.body ?? '').trim();
    if (body && body !== m.oobUrl) return body.replace(/\n/g, '\n> ');
    const n = m.oobs?.length ?? (m.oobUrl ? 1 : 0);
    return n > 1 ? `${n} pièces jointes` : n === 1 ? 'Pièce jointe' : '';
  }
  function buildForwardText(m: ChannelMessage): string {
    return `> **${authorOf(m)}** (transféré)\n> ${quotedBody(m)}\n`;
  }
  function buildReplyText(m: ChannelMessage): string {
    const base = typeof location !== 'undefined' ? location.origin : '';
    const link = `${base}/app/spaces/${id}/channels/${cid}?m=${encodeURIComponent(m.id)}`;
    const here = channel?.name ? ` dans #${channel.name}` : '';
    return `> **${authorOf(m)}**${here} — ${link}\n> ${quotedBody(m)}\n`;
  }
  function chooseForward(targetCid: string) {
    if (!forwardTarget) return;
    const text = citeMode === 'reply' ? buildReplyText(forwardTarget) : buildForwardText(forwardTarget);
    setForward(targetCid, text);
    forwardTarget = null;
    if (targetCid !== cid) void goto(`/app/spaces/${id}/channels/${targetCid}`);
  }
  const pendingForward = $derived(
    $forwardDraft && $forwardDraft.channelId === cid ? $forwardDraft : null
  );

  let jumpedFor: string | null = null;
  $effect(() => {
    const target = page.url.searchParams.get('m');
    if (!target) return;
    if (jumpedFor === target) return;
    if (channelMessages.some((m) => m.id === target)) {
      jumpedFor = target;
      void tick().then(() => jumpToMessage(target));
    }
  });

  let unreadBaseline = $state('');
  let baselineFor: string | null = null;
  $effect(() => {
    const c = cid;
    if (!c || isVoice) {
      baselineFor = null;
      unreadBaseline = '';
      return;
    }
    if (baselineFor === c) return;
    baselineFor = c;
    unreadBaseline = '';
    void loadReadState()
      .catch(() => {})
      .then(() => {
        if (baselineFor !== c) return;
        unreadBaseline = get(readState)[c]?.last_read_archive_id ?? '';
      });
  });

  let lastMarked: string | null = null;
  $effect(() => {
    const list = channelMessages;
    if (isVoice || $xmppState !== 'online' || !list.length) return;
    if (typeof document !== 'undefined' && document.hidden) return;
    const last = list[list.length - 1];
    if (last.id === lastMarked) return;
    lastMarked = last.id;
    void sendDisplayed(cid, last.id);
    void markChannelRead(cid, last.id).catch(() => {});
    markChannelMentionsRead(cid);
  });

  const hasUnreadDivider = $derived.by(() => {
    if (!unreadBaseline) return false;
    const idx = channelMessages.findIndex((m) => m.id === unreadBaseline);
    return idx >= 0 && idx < channelMessages.length - 1;
  });

  let chatBg = $state({ open: false, x: 0, y: 0 });
  const chatBgItems = $derived([
    { label: 'Marquer comme lu', icon: CheckCheck, onclick: markAllRead },
    { label: 'Messages épinglés', icon: Pin, onclick: () => (pinsOpen = !pinsOpen) },
    { label: 'Sondages', icon: BarChart3, onclick: () => (pollsOpen = !pollsOpen) },
    { label: 'Exporter la conversation', icon: Download, onclick: openExport }
  ]);
  function onChatBg(e: MouseEvent) {
    if (e.defaultPrevented) return;
    e.preventDefault();
    chatBg = { open: true, x: e.clientX, y: e.clientY };
  }

  async function markAllRead() {
    const list = channelMessages;
    if (!cid || !list.length) return;
    unreadBaseline = list[list.length - 1].id;
    try {
      await markChannelRead(cid, list[list.length - 1].id);
    } catch (e) {
      console.error('mark read', e);
    }
  }

  async function handleMarkUnread(m: ChannelMessage) {
    if (!cid) return;
    const idx = channelMessages.findIndex((x) => x.id === m.id);
    unreadBaseline = idx > 0 ? channelMessages[idx - 1].id : '';
    lastMarked = channelMessages[channelMessages.length - 1]?.id ?? lastMarked;
    try {
      await markChannelUnread(cid, m.id);
    } catch (e) {
      console.error('mark unread', e);
    }
  }

  const space = $derived(($spaces.data ?? []).find((s) => s.id === id));
  const canModerate = $derived(!!space && space.owner_id === $auth.user?.id);

  let notifLevel = $state<NotifLevel>('all');
  let suppressEveryone = $state(false);
  $effect(() => {
    const cid2 = cid;
    if (!cid2) return;
    notifLevel = 'all';
    suppressEveryone = false;
    void api<Array<{ scope_type: string; scope_id: string; level: NotifLevel; suppress_everyone: boolean }>>(
      '/api/me/notif-settings'
    )
      .then((rows) => {
        const ch = rows.find((r) => r.scope_type === 'channel' && r.scope_id === cid2);
        if (ch) {
          notifLevel = ch.level;
          suppressEveryone = ch.suppress_everyone;
        }
      })
      .catch(() => {});
  });
  async function applyNotifLevel(level: NotifLevel) {
    notifLevel = level;
    try {
      await setNotifSetting('channel', cid, { level, suppressEveryone });
    } catch (e) {
      console.error('notif setting', e);
    }
  }

  let timedOutUntil = $state<string | null>(null);
  $effect(() => {
    const sid = id;
    if (!sid) return;
    void myTimeout(sid)
      .then((t) => (timedOutUntil = t.active ? (t.expires_at ?? '') : null))
      .catch(() => (timedOutUntil = null));
  });
  const isTimedOut = $derived(timedOutUntil !== null);

  const writeBlocked = $derived(
    isTimedOut || (!canModerate && (!!channel?.locked || !!channel?.read_only))
  );

  let nsfwAck = $state<Set<string>>(new Set());
  const nsfwGated = $derived(!!channel?.nsfw && !nsfwAck.has(cid));
  function ackNsfw() {
    nsfwAck = new Set(nsfwAck).add(cid);
  }

  let lockBusy = $state(false);
  async function toggleLock() {
    if (!channel) return;
    lockBusy = true;
    try {
      await setChannelLock(id, cid, !channel.locked);
    } catch (e) {
      console.error('lock channel', e);
    } finally {
      lockBusy = false;
    }
  }

  let deleteTarget = $state<ChannelMessage | null>(null);
  let deleteBusy = $state(false);

  function requestDelete(m: ChannelMessage) {
    deleteTarget = m;
  }

  async function confirmDelete() {
    if (!deleteTarget || !cid) return;
    const m = deleteTarget;
    deleteBusy = true;
    try {
      await api(`/api/channels/${cid}/messages/${encodeURIComponent(m.id)}`, {
        method: 'DELETE'
      });
      removeMessage(cid, m.id);
      deleteTarget = null;
    } catch (e) {
      console.error('delete message', e);
    } finally {
      deleteBusy = false;
    }
  }

  const REPORT_CATEGORIES = [
    { key: 'spam', label: 'Spam' },
    { key: 'harassment', label: 'Harcèlement' },
    { key: 'illegal', label: 'Contenu illégal' },
    { key: 'nsfw', label: 'Contenu explicite' },
    { key: 'other', label: 'Autre' }
  ];
  let reportTarget = $state<ChannelMessage | null>(null);
  let reportReason = $state('');
  let reportCategory = $state('spam');
  let reportBusy = $state(false);
  let reportErr = $state<string | null>(null);
  let reportDone = $state(false);

  function openReport(m: ChannelMessage) {
    reportTarget = m;
    reportReason = '';
    reportCategory = 'spam';
    reportErr = null;
    reportDone = false;
  }

  function captureContext(m: ChannelMessage) {
    const idx = channelMessages.findIndex((x) => x.id === m.id);
    if (idx < 0) return [{ id: m.id, author: authorOf(m), body: m.body.slice(0, 500), reported: true }];
    const from = Math.max(0, idx - 4);
    const to = Math.min(channelMessages.length, idx + 5);
    return channelMessages.slice(from, to).map((x) => ({
      id: x.id,
      author_id: x.fromResource || x.from,
      author: authorOf(x),
      body: x.body.slice(0, 500),
      reported: x.id === m.id
    }));
  }

  async function submitReport(e: Event) {
    e.preventDefault();
    if (!reportTarget) return;
    reportBusy = true;
    reportErr = null;
    try {
      await api('/api/reports', {
        method: 'POST',
        body: {
          target_type: 'message',
          target_id: reportTarget.fromResource || reportTarget.from,
          channel_id: cid,
          space_id: id,
          category: reportCategory,
          context: captureContext(reportTarget),
          reason: `[msg:${reportTarget.id}] ${reportReason.trim()} — « ${reportTarget.body.slice(0, 140)} »`
        }
      });
      reportDone = true;
      setTimeout(() => (reportTarget = null), 1200);
    } catch (err) {
      reportErr = err instanceof Error ? err.message : 'Signalement impossible';
    } finally {
      reportBusy = false;
    }
  }

  let taskMsg = $state<ChannelMessage | null>(null);
  let taskTitle = $state('');
  let taskBusy = $state(false);
  function openTaskFromMessage(m: ChannelMessage) {
    taskMsg = m;
    taskTitle = m.body.slice(0, 200);
  }
  async function confirmTask(e: Event) {
    e.preventDefault();
    const title = taskTitle.trim();
    if (!title || !taskMsg) return;
    taskBusy = true;
    try {
      await createTask(id, title, { source_archive_id: taskMsg.id, channel_id: cid });
      taskMsg = null;
    } catch (err) {
      console.error('create task', err);
    } finally {
      taskBusy = false;
    }
  }

  let followedMsgIds = $state(new Set<string>());
  async function handleFollowMessage(m: ChannelMessage, follow: boolean) {
    try {
      await api(`/api/channels/${cid}/messages/${encodeURIComponent(m.id)}/follow`, {
        method: follow ? 'PUT' : 'DELETE'
      });
      const next = new Set(followedMsgIds);
      if (follow) next.add(m.id);
      else next.delete(m.id);
      followedMsgIds = next;
    } catch (e) {
      console.error('follow message', e);
    }
  }

  let historyTarget = $state<ChannelMessage | null>(null);

  async function handleBulkDelete(ids: string[]) {
    if (!cid || !ids.length) return;
    await api(`/api/channels/${cid}/messages/bulk-delete`, {
      method: 'POST',
      body: { archive_ids: ids }
    });
    for (const archiveId of ids) removeMessage(cid, archiveId);
  }

  const PURGE_WINDOWS = [
    { h: 0, label: 'Tout' },
    { h: 1, label: '1 h' },
    { h: 24, label: '24 h' },
    { h: 168, label: '7 j' }
  ];
  let purgeTarget = $state<ChannelMessage | null>(null);
  let purgeWindow = $state(0);
  let purgeBusy = $state(false);

  function openPurge(m: ChannelMessage) {
    purgeTarget = m;
    purgeWindow = 0;
  }
  async function confirmPurge() {
    if (!purgeTarget || !cid) return;
    const author = purgeTarget.fromResource || purgeTarget.from;
    const within = purgeWindow;
    purgeBusy = true;
    try {
      const res = await api<{ deleted: number; archive_ids: string[] }>(
        `/api/channels/${cid}/messages/purge`,
        {
          method: 'POST',
          body: within > 0 ? { author_id: author, within_hours: within } : { author_id: author }
        }
      );
      for (const archiveId of res.archive_ids ?? []) removeMessage(cid, archiveId);
      purgeTarget = null;
    } catch (e) {
      console.error('purge', e);
    } finally {
      purgeBusy = false;
    }
  }

  let exportOpen = $state(false);
  let exportFormat = $state<ExportFormat>('html');
  let exportFirstIdx = $state(0);
  let exportLastIdx = $state(0);
  let exportBusy = $state(false);

  function openExport() {
    exportFirstIdx = 0;
    exportLastIdx = Math.max(0, channelMessages.length - 1);
    exportFormat = 'html';
    exportOpen = true;
  }

  function popOut() {
    const w = Math.min(820, Math.round(window.screen.availWidth * 0.5));
    window.open(
      `/app/spaces/${id}/channels/${cid}?popout=1`,
      `krovara-popout-${cid}`,
      `popup=yes,width=${w},height=${Math.round(window.screen.availHeight * 0.85)}`
    );
  }

  function exportLabel(m: ChannelMessage): string {
    const t = m.at.toLocaleString('fr-FR', {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
    return `${t} · ${authorOf(m)}: ${m.body.slice(0, 40)}`;
  }

  async function runExport() {
    const list = channelMessages;
    if (!list.length) return;
    const lo = Math.min(exportFirstIdx, exportLastIdx);
    const hi = Math.max(exportFirstIdx, exportLastIdx);
    const slice = list.slice(lo, hi + 1);
    exportBusy = true;
    try {
      const avatars: Record<string, string | null> = {};
      if (exportFormat === 'html') {
        const keyByUser: Record<string, string | null> = {};
        for (const mem of memberList) keyByUser[mem.user_id] = mem.avatar_key;
        const ids = [...new Set(slice.map(authorIdOf))];
        await Promise.all(
          ids.map(async (uid) => {
            const key = keyByUser[uid];
            if (!key) {
              avatars[uid] = null;
              return;
            }
            try {
              avatars[uid] = await authedDataURL(`/api/files/${key}`);
            } catch {
              avatars[uid] = null;
            }
          })
        );
      }
      const entries: ExportEntry[] = slice.map((m) => {
        const aid = authorIdOf(m);
        return {
          id: m.id,
          authorId: aid,
          author: authorOf(m),
          body: m.body,
          at: m.at,
          edited: !!m.edited,
          avatar: avatars[aid] ?? null
        };
      });
      const content = renderExport(exportFormat, entries, {
        channelName: channel?.name ?? 'salon',
        spaceName: space?.name,
        exportedAt: new Date()
      });
      const ext = exportFormat === 'txt' ? 'txt' : exportFormat;
      const stamp = new Date().toISOString().slice(0, 10);
      const safeName = (channel?.name ?? 'salon').replace(/[^\w-]+/g, '-');
      downloadFile(`krovara-${safeName}-${stamp}.${ext}`, content, mimeFor(exportFormat));
      exportOpen = false;
    } catch (e) {
      console.error('export', e);
    } finally {
      exportBusy = false;
    }
  }

  async function toggleCam() {
    if ($cameraOn) await disableCamera();
    else await enableCamera().catch((e) => console.error('camera', e));
  }
  async function toggleScreen() {
    if ($screenOn) await disableScreenShare();
    else await enableScreenShare().catch((e) => console.error('screen', e));
  }
</script>

<div class="flex h-full flex-col">
  <header class="flex items-center gap-3 border-b border-border px-4 py-3">
    {#if isVoice}
      <Volume2 size={20} class="shrink-0 text-muted" />
    {:else}
      <Hash size={20} class="shrink-0 text-muted" />
    {/if}
    <div class="min-w-0">
      <div class="flex items-center gap-1.5">
        <h1 class="truncate text-body font-semibold leading-tight">{channel?.name ?? '…'}</h1>
        {#if channel?.locked}
          <Lock size={14} class="shrink-0 text-warning" aria-label="Salon verrouillé" />
        {/if}
      </div>
      {#if channel?.topic}
        <p class="truncate text-label text-muted">{channel.topic}</p>
      {/if}
    </div>
      <div class="ml-auto flex items-center gap-1.5">
        <QuickAccess />
        <span class="mx-0.5 hidden h-5 w-px bg-border md:block" aria-hidden="true"></span>
        {#if $xmppState !== 'online'}
          <span
            class="flex items-center gap-1.5 rounded-full bg-warning/10 px-2.5 py-1 text-label text-warning"
          >
            <span class="size-1.5 animate-pulse rounded-full bg-warning"></span>
            <span class="hidden sm:inline">{$xmppState}</span>
          </span>
        {/if}
        <div class="hidden items-center gap-1.5 md:flex">
          {#if hasUnreadDivider}
            <button
              type="button"
              onclick={markAllRead}
              class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-label text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
            >
              <CheckCheck size={16} /> Marquer comme lu
            </button>
          {/if}
          {#if canModerate}
            <button
              type="button"
              title={channel?.locked ? 'Déverrouiller le salon' : 'Verrouiller le salon'}
              aria-pressed={channel?.locked ?? false}
              onclick={toggleLock}
              disabled={lockBusy}
              class="grid size-8 place-items-center rounded-md transition-colors duration-150 hover:bg-elevated disabled:opacity-50
                     {channel?.locked ? 'text-warning' : 'text-muted hover:text-content'}"
            >
              {#if channel?.locked}<Lock size={18} />{:else}<Unlock size={18} />{/if}
            </button>
          {/if}
          <Popover.Root>
            <Popover.Trigger
              title="Notifications du salon"
              class="grid size-8 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content
                     {notifLevel !== 'all' ? 'text-warning' : ''}"
            >
              {#if notifLevel === 'nothing'}<BellOff size={18} />{:else}<Bell size={18} />{/if}
            </Popover.Trigger>
            <Popover.Portal>
              <Popover.Content
                align="end"
                sideOffset={4}
                class="z-50 w-52 rounded-lg border border-border bg-overlay p-1 shadow-lg animate-fade-in"
              >
                {#each [{ v: 'all', l: 'Tous les messages' }, { v: 'mentions', l: 'Mentions seulement' }, { v: 'nothing', l: 'Rien' }] as o (o.v)}
                  <button
                    type="button"
                    onclick={() => applyNotifLevel(o.v as NotifLevel)}
                    class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-body transition-colors duration-150 hover:bg-elevated
                           {notifLevel === o.v ? 'text-content' : 'text-muted'}"
                  >
                    {o.l}
                    {#if notifLevel === o.v}<Check size={15} class="text-accent" />{/if}
                  </button>
                {/each}
                <div class="my-1 h-px bg-border"></div>
                <label class="flex cursor-pointer items-center justify-between gap-2 rounded px-2 py-1.5 text-label text-muted hover:bg-elevated">
                  Couper @everyone
                  <input type="checkbox" bind:checked={suppressEveryone} onchange={() => applyNotifLevel(notifLevel)} class="accent-primary" />
                </label>
              </Popover.Content>
            </Popover.Portal>
          </Popover.Root>
          <button
            type="button"
            title="Messages épinglés"
            onclick={() => (pinsOpen = !pinsOpen)}
            class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-muted transition-colors duration-150 hover:bg-elevated hover:text-content
                   {pinsOpen ? 'bg-elevated text-content' : ''}"
          >
            <Pin size={18} />
            {#if pinnedIds.size > 0}
              <span class="text-label tabular-nums">{pinnedIds.size}</span>
            {/if}
          </button>
          <button
            type="button"
            title="Sondages"
            onclick={() => (pollsOpen = !pollsOpen)}
            class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-muted transition-colors duration-150 hover:bg-elevated hover:text-content
                   {pollsOpen ? 'bg-elevated text-content' : ''}"
          >
            <BarChart3 size={18} />
            {#if pollCount > 0}
              <span class="text-label tabular-nums">{pollCount}</span>
            {/if}
          </button>
        </div>
        <Popover.Root>
          <Popover.Trigger
            title="Plus"
            class="grid size-8 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
          >
            <MoreVertical size={18} />
          </Popover.Trigger>
          <Popover.Portal>
            <Popover.Content
              align="end"
              sideOffset={4}
              class="z-50 w-56 rounded-lg border border-border bg-overlay p-1 shadow-lg animate-fade-in"
            >
              <div class="md:hidden">
                {#if hasUnreadDivider}
                  <Popover.Close
                    onclick={markAllRead}
                    class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                  >
                    <CheckCheck size={16} class="text-muted" /> Marquer comme lu
                  </Popover.Close>
                {/if}
                {#if canModerate}
                  <Popover.Close
                    onclick={toggleLock}
                    class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                  >
                    {#if channel?.locked}<Unlock size={16} class="text-muted" /> Déverrouiller le salon
                    {:else}<Lock size={16} class="text-muted" /> Verrouiller le salon{/if}
                  </Popover.Close>
                {/if}
                <Popover.Close
                  onclick={() => (pinsOpen = !pinsOpen)}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <Pin size={16} class="text-muted" /> Messages épinglés
                  {#if pinnedIds.size > 0}<span class="ml-auto text-label text-muted tabular-nums">{pinnedIds.size}</span>{/if}
                </Popover.Close>
                <Popover.Close
                  onclick={() => (pollsOpen = !pollsOpen)}
                  class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
                >
                  <BarChart3 size={16} class="text-muted" /> Sondages
                  {#if pollCount > 0}<span class="ml-auto text-label text-muted tabular-nums">{pollCount}</span>{/if}
                </Popover.Close>
                <div class="px-2 pb-1 pt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted/70">Notifications</div>
                {#each [{ v: 'all', l: 'Tous les messages' }, { v: 'mentions', l: 'Mentions seulement' }, { v: 'nothing', l: 'Rien' }] as o (o.v)}
                  <Popover.Close
                    onclick={() => applyNotifLevel(o.v as NotifLevel)}
                    class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-body transition-colors hover:bg-elevated
                           {notifLevel === o.v ? 'text-content' : 'text-muted'}"
                  >
                    {o.l}
                    {#if notifLevel === o.v}<Check size={15} class="text-accent" />{/if}
                  </Popover.Close>
                {/each}
                <div class="my-1 h-px bg-border"></div>
              </div>
              <Popover.Close
                onclick={openExport}
                class="flex w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated"
              >
                <Download size={16} class="text-muted" /> Exporter la conversation
              </Popover.Close>
              {#if !isPopout}
                <Popover.Close
                  onclick={popOut}
                  class="hidden w-full items-center gap-2.5 rounded px-2 py-1.5 text-left text-body text-content transition-colors hover:bg-elevated md:flex"
                >
                  <ExternalLink size={16} class="text-muted" /> Ouvrir dans une fenêtre
                </Popover.Close>
              {/if}
            </Popover.Content>
          </Popover.Portal>
        </Popover.Root>
      </div>
  </header>

  {#if isVoice}
    <div class="flex shrink-0 flex-col bg-surface shadow-inner z-10 border-b border-border" style="min-height: 45vh;">
      {#if $activeRoom === cid}
        <div class="flex-1 overflow-hidden relative bg-black/40">
          <VideoGrid />
        </div>
        <div class="flex items-center justify-between bg-surface px-6 py-3">
          <div class="flex items-center gap-2">
            <span class="flex size-2 rounded-full {$sfuState === 'connected' ? 'bg-success' : 'bg-danger animate-pulse'}"></span>
            <span class="text-label font-medium {$sfuState === 'connected' ? 'text-success' : 'text-danger'}">
              {$sfuState === 'connected' ? 'Voix connectée' : 'Connexion...'}
            </span>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              onclick={() => toggleMic()}
              title={$micOn ? 'Couper le micro' : 'Activer le micro'}
              class="grid size-10 place-items-center rounded-full transition-colors duration-150
                     {$micOn
                ? 'bg-elevated text-content hover:bg-elevated-hover'
                : 'bg-danger text-white hover:bg-danger-hover'}"
            >
              {#if $micOn}<Mic size={18} />{:else}<MicOff size={18} />{/if}
            </button>
            <button
              type="button"
              onclick={toggleCam}
              title={$cameraOn ? 'Couper la caméra' : 'Activer la caméra'}
              class="grid size-10 place-items-center rounded-full transition-colors duration-150
                     {$cameraOn
                ? 'bg-elevated text-content hover:bg-elevated-hover'
                : 'bg-danger text-white hover:bg-danger-hover'}"
            >
              {#if $cameraOn}<Video size={18} />{:else}<VideoOff size={18} />{/if}
            </button>
            <button
              type="button"
              onclick={toggleScreen}
              title={$screenOn ? 'Arrêter le partage' : 'Partager l’écran'}
              class="grid size-10 place-items-center rounded-full transition-colors duration-150
                     {$screenOn
                ? 'bg-brand text-white hover:bg-brand-hover'
                : 'bg-elevated text-content hover:bg-elevated-hover'}"
            >
              <MonitorUp size={18} />
            </button>
            <div class="w-px h-6 bg-border mx-1"></div>
            <button
              type="button"
              onclick={() => leaveVoice()}
              title="Quitter"
              class="grid px-4 h-10 place-items-center rounded-full bg-danger text-white font-semibold text-sm transition-colors duration-150 hover:bg-danger-hover flex items-center gap-2"
            >
              <PhoneOff size={16} /> Quitter
            </button>
          </div>
          <div class="w-[100px]"></div>
        </div>
      {:else}
        <div class="flex flex-1 flex-col items-center justify-center gap-4 bg-black/20 p-8">
          <div class="grid size-16 place-items-center rounded-full bg-success/20 text-success">
            <Volume2 size={32} />
          </div>
          <p class="text-body text-muted">Prêt à rejoindre l'appel ?</p>
          <button
            type="button"
            onclick={() => joinVoice(cid)}
            class="flex items-center gap-2 rounded-full bg-success px-6 py-3 text-body font-bold text-white transition-transform duration-150 hover:scale-105 active:scale-95"
          >
            Rejoindre le salon vocal
          </button>
        </div>
      {/if}
    </div>
  {/if}
    <div class="flex flex-1 overflow-hidden">
    <div
      class="relative flex flex-1 flex-col overflow-hidden"
      role="group"
      oncontextmenu={onChatBg}
      ondragenter={onDragEnter}
      ondragover={(e) => e.preventDefault()}
      ondragleave={onDragLeave}
      ondrop={onDrop}
    >
    {#if nsfwGated}
      <div class="flex flex-1 flex-col items-center justify-center gap-4 px-6 text-center">
        <div class="grid size-14 place-items-center rounded-full bg-danger/10 text-danger">
          <ShieldAlert size={28} />
        </div>
        <div class="space-y-1">
          <h2 class="text-subtitle font-semibold">Contenu sensible</h2>
          <p class="max-w-sm text-body text-muted">
            Ce salon est marqué comme pouvant contenir du contenu explicite (NSFW).
            Confirme que tu veux l'afficher.
          </p>
        </div>
        <Button variant="primary" onclick={ackNsfw}>Afficher le salon</Button>
      </div>
    {:else}
    {#key cid}
    <MessageList
      messages={channelMessages}
      onloadmore={loadMoreHistory}
      hasMore={hasMoreHistory}
      {loadingMore}
      {canModerate}
      selfId={$auth.user?.id}
      mentionUsernames={mentionUsernames}
      mentionRoles={mentionRoles}
      selfUsername={$auth.user?.username}
      reactions={$reactions}
      reads={$readMarkers[cid] ?? {}}
      canManage={canModerate}
      pinnedIds={pinnedIds}
      savedIds={$savedIds}
      spaceId={id}
      unreadAfterId={unreadBaseline}
      recentEmojis={$recentReactions}
      threadsByRoot={threadsByRoot}
      onthread={openThread}
      onreport={openReport}
      onforward={openForward}
      onreplyelsewhere={openReplyElsewhere}
      ondelete={requestDelete}
      onedit={handleEdit}
      onreact={handleReact}
      onreply={startReply}
      onpin={handlePin}
      onsave={handleSave}
      onmarkunread={handleMarkUnread}
      onhistory={(m) => (historyTarget = m)}
      ontask={openTaskFromMessage}
      onfollow={handleFollowMessage}
      followedIds={followedMsgIds}
      onbulkdelete={handleBulkDelete}
      onpurge={openPurge}
    />
    {/key}
    {#if dragActive}
      <div class="pointer-events-none absolute inset-0 z-10 grid place-items-center bg-base/70 backdrop-blur-sm">
        <div class="flex flex-col items-center gap-2 rounded-xl border-2 border-dashed border-brand px-10 py-8 text-center">
          <Paperclip size={28} class="text-brand" />
          <p class="text-body font-medium text-content">Dépose ton fichier pour l'envoyer</p>
        </div>
      </div>
    {/if}
    <PinsPanel
      open={pinsOpen}
      channelId={cid}
      canManage={canModerate}
      onjump={jumpToMessage}
      onclose={() => (pinsOpen = false)}
    />
    <PollsPanel
      open={pollsOpen}
      channelId={cid}
      selfId={$auth.user?.id}
      onclose={() => (pollsOpen = false)}
    />
    {#if typers.length}
      <p class="px-5 pt-1 text-label text-muted">
        <span class="inline-flex gap-0.5 align-middle">
          <span class="size-1 animate-bounce rounded-full bg-muted [animation-delay:-0.3s]"></span>
          <span class="size-1 animate-bounce rounded-full bg-muted [animation-delay:-0.15s]"></span>
          <span class="size-1 animate-bounce rounded-full bg-muted"></span>
        </span>
        {typers.length === 1
          ? `${typers[0]} est en train d'écrire…`
          : `${typers.slice(0, 3).join(', ')} écrivent…`}
      </p>
    {/if}
    {#if writeBlocked}
      <div class="shrink-0 px-4 pb-4 pt-1">
        <div
          class="flex items-center justify-center gap-2 rounded-lg border border-border bg-surface px-3 py-3 text-label text-muted"
        >
          {#if isTimedOut}
            <TimerOff size={16} class="shrink-0" />
            <span>Tu es en sourdine dans cet espace, tu ne peux pas écrire pour le moment.</span>
          {:else if channel?.read_only && !channel?.locked}
            <Megaphone size={16} class="shrink-0" />
            <span>Salon annonce : seuls les modérateurs peuvent publier ici.</span>
          {:else}
            <Lock size={16} class="shrink-0" />
            <span>Salon verrouillé : seuls les modérateurs peuvent écrire ici.</span>
          {/if}
        </div>
      </div>
    {:else}
      {#if slowmodeLeft > 0}
        <p class="px-4 pt-1 text-label text-warning">
          Mode lent : patiente {slowmodeLeft}s avant d'envoyer un nouveau message.
        </p>
      {/if}
      {#if attachErr}
        <p class="mx-4 mt-1 rounded-md border border-danger/40 bg-danger/10 px-3 py-1.5 text-label text-danger">
          {attachErr}
        </p>
      {/if}
      <MessageInput
        bind:this={inputCmp}
        onsend={handleSend}
        spaceId={id}
        draftKey={cid}
        disabled={$xmppState !== 'online'}
        placeholder={channel?.slowmode_seconds
          ? `Mode lent actif (${channel.slowmode_seconds}s)`
          : undefined}
        replyName={replyTarget ? authorOf(replyTarget) : null}
        replyPreview={replyTarget ? replyPreviewOf(replyTarget) : null}
        oncancelreply={() => (replyTarget = null)}
        mentionables={mentionables}
        mentionableRoles={mentionableRoleNames}
        ontyping={handleTyping}
        onattach={handleAttach}
        ongif={handleGif}
        onsticker={handleSticker}
        prefill={pendingForward?.text}
        prefillNonce={pendingForward?.nonce}
      />
    {/if}
    {/if}
    </div>
    <ThreadPanel
      thread={activeThread}
      spaceId={id}
      selfId={$auth.user?.id}
      selfUsername={$auth.user?.username}
      mentionUsernames={mentionUsernames}
      mentionRoles={mentionRoles}
      mentionables={mentionables}
      onclose={() => (activeThreadId = null)}
    />
    </div>
</div>

<BgContextMenu bind:open={chatBg.open} x={chatBg.x} y={chatBg.y} items={chatBgItems} />

<Modal open={!!taskMsg} title="Créer une tâche" onclose={() => (taskMsg = null)}>
  {#if taskMsg}
    <form onsubmit={confirmTask} class="space-y-3">
      <Input label="Intitulé de la tâche" bind:value={taskTitle} maxlength={280} required />
      <p class="text-label text-muted">Créée depuis ce message dans l'espace courant.</p>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" onclick={() => (taskMsg = null)}>Annuler</Button>
        <Button type="submit" loading={taskBusy}>Créer la tâche</Button>
      </div>
    </form>
  {/if}
</Modal>

<Modal open={!!deleteTarget} title="Supprimer ce message" onclose={() => (deleteTarget = null)}>
  {#if deleteTarget}
    <p class="text-body text-muted">Cette action est définitive.</p>
    {@const dtText = bodyText(deleteTarget)}
    {@const dtAttach = firstAttachment(deleteTarget)}
    <div class="mt-3 rounded-md border-l-2 border-border bg-base/50 px-3 py-2">
      {#if dtText}
        <p class="text-label text-muted">{dtText.slice(0, 160)}</p>
      {/if}
      {#if dtAttach}
        <AttachmentView url={dtAttach.url} name={dtAttach.name} sticker={dtAttach.name === 'sticker'} />
      {:else if !dtText}
        <p class="text-label italic text-muted">(message vide)</p>
      {/if}
    </div>
    <div class="mt-4 flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={() => (deleteTarget = null)}>Annuler</Button>
      <Button type="button" variant="danger" loading={deleteBusy} onclick={confirmDelete}>Supprimer</Button>
    </div>
  {/if}
</Modal>

<Modal open={!!reportTarget} title="Signaler ce message" onclose={() => (reportTarget = null)}>
  {#if reportDone}
    <p class="text-body text-success">Merci, le signalement a été transmis aux modérateurs.</p>
  {:else}
    <form onsubmit={submitReport} class="space-y-4">
      {#if reportTarget}
        <blockquote class="rounded-md border-l-2 border-border bg-base/50 px-3 py-2 text-label text-muted">
          {reportTarget.body.slice(0, 160)}
        </blockquote>
      {/if}
      <div>
        <p id="report-cat-label" class="mb-1.5 text-label font-medium text-muted">Catégorie</p>
        <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-labelledby="report-cat-label">
          {#each REPORT_CATEGORIES as c (c.key)}
            <button
              type="button"
              role="radio"
              aria-checked={reportCategory === c.key}
              onclick={() => (reportCategory = c.key)}
              class="rounded-md border px-3 py-1.5 text-label transition-colors duration-150
                     {reportCategory === c.key
                ? 'border-primary bg-primary/15 text-content'
                : 'border-border text-muted hover:bg-elevated hover:text-content'}"
            >
              {c.label}
            </button>
          {/each}
        </div>
      </div>
      <label class="block space-y-1.5">
        <span class="text-label font-medium text-muted">Raison</span>
        <textarea
          bind:value={reportReason}
          required
          rows="3"
          placeholder="Spam, contenu inapproprié, harcèlement…"
          class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none focus:border-brand"
        ></textarea>
      </label>
      <p class="text-[0.6875rem] text-muted/70">
        Les messages voisins sont joints automatiquement pour donner le contexte aux modérateurs.
      </p>
      {#if reportErr}
        <p class="text-label text-danger">{reportErr}</p>
      {/if}
      <Button type="submit" full loading={reportBusy}>Envoyer le signalement</Button>
    </form>
  {/if}
</Modal>

<Modal
  open={!!forwardTarget}
  title={citeMode === 'reply' ? 'Répondre dans un autre salon' : 'Transférer le message'}
  onclose={() => (forwardTarget = null)}
>
  {#if forwardTarget}
    <blockquote class="rounded-md border-l-2 border-border bg-base/50 px-3 py-2 text-label text-muted">
      {forwardTarget.body.slice(0, 160)}
    </blockquote>
    <p class="mt-4 mb-1.5 text-label font-medium text-muted">Vers le salon</p>
    <div class="max-h-64 space-y-0.5 overflow-y-auto">
      {#each textChannels as c (c.id)}
        <button
          type="button"
          onclick={() => chooseForward(c.id)}
          class="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-body text-content transition-colors duration-150 hover:bg-elevated"
        >
          <Hash size={15} class="shrink-0 text-muted" />
          <span class="truncate">{c.name}</span>
          {#if c.id === cid}<span class="ml-auto text-label text-muted">ici</span>{/if}
        </button>
      {/each}
    </div>
    <p class="mt-3 text-[0.6875rem] text-muted/70">
      Le message est pré-rempli dans le salon choisi ; ajoute un mot puis envoie.
    </p>
  {/if}
</Modal>

<EditHistory
  open={!!historyTarget}
  channelId={cid}
  archiveId={historyTarget?.id ?? null}
  onclose={() => (historyTarget = null)}
/>

<Modal open={!!purgeTarget} title="Purger les messages" onclose={() => (purgeTarget = null)}>
  {#if purgeTarget}
    <p class="text-body text-muted">
      Supprimer les messages de
      <span class="font-medium text-content">{authorOf(purgeTarget)}</span>
      dans ce salon. Cette action est définitive.
    </p>
    <div class="mt-4">
      <p id="purge-window-label" class="mb-1.5 text-label font-medium text-muted">Période</p>
      <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-labelledby="purge-window-label">
        {#each PURGE_WINDOWS as wdw (wdw.h)}
          <button
            type="button"
            role="radio"
            aria-checked={purgeWindow === wdw.h}
            onclick={() => (purgeWindow = wdw.h)}
            class="rounded-md border px-3 py-1.5 text-label transition-colors duration-150
                   {purgeWindow === wdw.h
              ? 'border-primary bg-primary/15 text-content'
              : 'border-border text-muted hover:bg-elevated hover:text-content'}"
          >
            {wdw.label}
          </button>
        {/each}
      </div>
    </div>
    <div class="mt-5 flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={() => (purgeTarget = null)}>Annuler</Button>
      <Button type="button" variant="danger" loading={purgeBusy} onclick={confirmPurge}>Purger</Button>
    </div>
  {/if}
</Modal>

<Modal open={exportOpen} title="Exporter la conversation" onclose={() => (exportOpen = false)}>
  <div class="space-y-4">
    <div>
      <p id="export-fmt-label" class="mb-1.5 text-label font-medium text-muted">Format</p>
      <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-labelledby="export-fmt-label">
        {#each [['html', 'HTML (mise en forme)'], ['txt', 'Texte'], ['json', 'JSON']] as [val, label] (val)}
          <button
            type="button"
            role="radio"
            aria-checked={exportFormat === val}
            onclick={() => (exportFormat = val as ExportFormat)}
            class="rounded-md border px-3 py-1.5 text-label transition-colors duration-150
                   {exportFormat === val
              ? 'border-primary bg-primary/15 text-content'
              : 'border-border text-muted hover:bg-elevated hover:text-content'}"
          >
            {label}
          </button>
        {/each}
      </div>
    </div>

    {#if channelMessages.length}
      <label class="block space-y-1.5">
        <span class="text-label font-medium text-muted">Premier message</span>
        <select
          bind:value={exportFirstIdx}
          class="w-full truncate rounded border border-border bg-base/50 px-2 py-2 text-label text-content outline-none focus:border-brand"
        >
          {#each channelMessages as m, i (m.id)}
            <option value={i}>{exportLabel(m)}</option>
          {/each}
        </select>
      </label>
      <label class="block space-y-1.5">
        <span class="text-label font-medium text-muted">Dernier message</span>
        <select
          bind:value={exportLastIdx}
          class="w-full truncate rounded border border-border bg-base/50 px-2 py-2 text-label text-content outline-none focus:border-brand"
        >
          {#each channelMessages as m, i (m.id)}
            <option value={i}>{exportLabel(m)}</option>
          {/each}
        </select>
      </label>
      <p class="text-[0.6875rem] text-muted/70">
        L'export HTML conserve la mise en forme, les avatars et les marques de modification.
      </p>
    {:else}
      <p class="text-body text-muted">Aucun message à exporter.</p>
    {/if}

    <div class="flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={() => (exportOpen = false)}>Annuler</Button>
      <Button type="button" loading={exportBusy} disabled={!channelMessages.length} onclick={runExport}>
        <Download size={16} /> Exporter
      </Button>
    </div>
  </div>
</Modal>
