import { writable, derived } from 'svelte/store';
import { api } from '$lib/api';
import { announce } from './announce';

export type InboxItem = {
  id: string;
  kind: 'mention' | 'everyone' | 'reply' | 'join_approved' | 'join_rejected';
  space_id?: string;
  channel_id?: string;
  archive_id: string;
  author_id?: string;
  preview: string | null;
  read: boolean;
  created_at: string;
};

export const inboxItems = writable<InboxItem[]>([]);
export const inboxUnread = writable<number>(0);

export const mentionsByChannel = derived(inboxItems, ($items) => {
  const out: Record<string, number> = {};
  for (const it of $items) {
    if (it.read || !it.channel_id) continue;
    if (it.kind === 'mention' || it.kind === 'everyone' || it.kind === 'reply') {
      out[it.channel_id] = (out[it.channel_id] ?? 0) + 1;
    }
  }
  return out;
});

export async function loadInbox(): Promise<void> {
  const r = await api<{ items: InboxItem[]; unread: number }>('/api/me/inbox');
  inboxItems.set(r.items);
  inboxUnread.set(r.unread);
}

let lastSeenUnread = -1;

export async function refreshInboxUnread(): Promise<void> {
  try {
    const r = await api<{ unread: number }>('/api/me/inbox');
    if (lastSeenUnread >= 0 && r.unread > lastSeenUnread) {
      announce('Nouvelle notification dans la boîte de réception.');
    }
    lastSeenUnread = r.unread;
    inboxUnread.set(r.unread);
  } catch {
  }
}

export async function markRead(id: string): Promise<void> {
  await api(`/api/me/inbox/${id}/read`, { method: 'PUT' });
  inboxItems.update((items) => items.map((i) => (i.id === id ? { ...i, read: true } : i)));
  inboxUnread.update((n) => Math.max(0, n - 1));
}

export async function markAllRead(): Promise<void> {
  await api('/api/me/inbox/read', { method: 'PUT' });
  inboxItems.update((items) => items.map((i) => ({ ...i, read: true })));
  inboxUnread.set(0);
}

export function markChannelMentionsRead(channelId: string): void {
  let toClear: string[] = [];
  inboxItems.update((items) => {
    toClear = items.filter((i) => !i.read && i.channel_id === channelId).map((i) => i.id);
    if (!toClear.length) return items;
    return items.map((i) => (i.channel_id === channelId ? { ...i, read: true } : i));
  });
  if (!toClear.length) return;
  inboxUnread.update((n) => Math.max(0, n - toClear.length));
  for (const id of toClear) {
    void api(`/api/me/inbox/${id}/read`, { method: 'PUT' }).catch(() => {});
  }
}

export type NotifLevel = 'all' | 'mentions' | 'nothing';

export async function setNotifSetting(
  scopeType: 'space' | 'channel',
  scopeId: string,
  opts: { level: NotifLevel; muteMinutes?: number; suppressEveryone?: boolean }
): Promise<void> {
  await api('/api/me/notif-settings', {
    method: 'PUT',
    body: {
      scope_type: scopeType,
      scope_id: scopeId,
      level: opts.level,
      mute_minutes: opts.muteMinutes ?? 0,
      suppress_everyone: opts.suppressEveryone ?? false
    }
  });
}

const DEFAULT_JOIN_NOTIF_KEY = 'krovara.defaultJoinNotif';

export function getDefaultJoinNotif(): NotifLevel {
  if (typeof localStorage === 'undefined') return 'all';
  const v = localStorage.getItem(DEFAULT_JOIN_NOTIF_KEY);
  return v === 'mentions' || v === 'nothing' ? v : 'all';
}

export function setDefaultJoinNotif(level: NotifLevel): void {
  try {
    localStorage.setItem(DEFAULT_JOIN_NOTIF_KEY, level);
  } catch {
  }
}

export async function applyDefaultNotifOnJoin(spaceId: string): Promise<void> {
  const level = getDefaultJoinNotif();
  if (level === 'all') return;
  try {
    await setNotifSetting('space', spaceId, { level });
  } catch {
  }
}
