import { writable, derived } from 'svelte/store';
import { api } from '$lib/api';

export type ReadState = {
  channel_id: string;
  last_read_archive_id: string;
  last_read_sort_id: number;
  unread_count: number;
  updated_at?: string;
};

export const readState = writable<Record<string, ReadState>>({});

export const unreadByChannel = derived(readState, ($rs) => {
  const out: Record<string, number> = {};
  for (const [cid, rs] of Object.entries($rs)) out[cid] = rs.unread_count;
  return out;
});

let loadedOnce = false;

export async function loadReadState(): Promise<void> {
  const list = await api<ReadState[]>('/api/me/read-state');
  const map: Record<string, ReadState> = {};
  for (const rs of list) map[rs.channel_id] = rs;
  readState.set(map);
  loadedOnce = true;
}

export async function ensureReadStateLoaded(): Promise<void> {
  if (loadedOnce) return;
  await loadReadState();
}

export async function markRead(channelId: string, archiveId: string): Promise<void> {
  const rs = await api<ReadState>(`/api/channels/${channelId}/read-state`, {
    method: 'PUT',
    body: { archive_id: archiveId, mode: 'read' }
  });
  readState.update((m) => ({ ...m, [channelId]: rs }));
}

export async function markUnread(channelId: string, archiveId: string): Promise<void> {
  const rs = await api<ReadState>(`/api/channels/${channelId}/read-state`, {
    method: 'PUT',
    body: { archive_id: archiveId, mode: 'unread' }
  });
  readState.update((m) => ({ ...m, [channelId]: rs }));
}
