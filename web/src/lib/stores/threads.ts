import { writable } from 'svelte/store';
import { api } from '$lib/api';

export type Thread = {
  id: string;
  channel_id: string;
  root_archive_id: string;
  title: string;
  created_by: string;
  created_at: string;
  last_activity_at: string;
  reply_count: number;
  is_subscribed: boolean;
};

type ByChannel = Record<string, Thread[]>;

export const threadsByChannel = writable<ByChannel>({});

export function threadRoom(threadId: string): string {
  return `thread-${threadId}`;
}

export async function loadThreads(channelId: string): Promise<void> {
  const list = await api<Thread[]>(`/api/channels/${channelId}/threads`);
  threadsByChannel.update((map) => ({ ...map, [channelId]: list }));
}

export async function createThread(
  channelId: string,
  rootArchiveId: string,
  title: string
): Promise<Thread> {
  const t = await api<Thread>(`/api/channels/${channelId}/threads`, {
    method: 'POST',
    body: { root_archive_id: rootArchiveId, title }
  });
  threadsByChannel.update((map) => {
    const cur = map[channelId] ?? [];
    return { ...map, [channelId]: [t, ...cur.filter((x) => x.id !== t.id)] };
  });
  return t;
}

export async function subscribeThread(channelId: string, threadId: string): Promise<void> {
  await api(`/api/threads/${threadId}/subscribe`, { method: 'POST' });
  patchThread(channelId, threadId, { is_subscribed: true });
}

export async function unsubscribeThread(channelId: string, threadId: string): Promise<void> {
  await api(`/api/threads/${threadId}/subscribe`, { method: 'DELETE' });
  patchThread(channelId, threadId, { is_subscribed: false });
}

export async function touchThread(threadId: string): Promise<void> {
  try {
    await api(`/api/threads/${threadId}/touch`, { method: 'POST' });
  } catch {
  }
}

function patchThread(channelId: string, threadId: string, patch: Partial<Thread>): void {
  threadsByChannel.update((map) => {
    const cur = map[channelId];
    if (!cur) return map;
    return {
      ...map,
      [channelId]: cur.map((t) => (t.id === threadId ? { ...t, ...patch } : t))
    };
  });
}
