import { writable } from 'svelte/store';
import { api } from '$lib/api';

export type Pin = {
  archive_id: string;
  channel_id: string;
  pinned_by: string;
  pinned_at: string;
  note: string;
  missing: boolean;
  author_id: string;
  body: string;
  at?: string;
};

export const pinsByChannel = writable<Record<string, Pin[]>>({});

export async function loadPins(channelId: string): Promise<void> {
  const list = await api<Pin[]>(`/api/channels/${channelId}/pins`);
  pinsByChannel.update((m) => ({ ...m, [channelId]: list }));
}

export async function pinMessage(channelId: string, archiveId: string, note = ''): Promise<void> {
  const pin = await api<Pin>(`/api/channels/${channelId}/pins`, {
    method: 'POST',
    body: { archive_id: archiveId, note }
  });
  pinsByChannel.update((m) => {
    const cur = m[channelId] ?? [];
    const without = cur.filter((p) => p.archive_id !== archiveId);
    return { ...m, [channelId]: [pin, ...without] };
  });
}

export async function unpinMessage(channelId: string, archiveId: string): Promise<void> {
  await api(`/api/channels/${channelId}/pins/${encodeURIComponent(archiveId)}`, {
    method: 'DELETE'
  });
  pinsByChannel.update((m) => {
    const cur = m[channelId];
    if (!cur) return m;
    return { ...m, [channelId]: cur.filter((p) => p.archive_id !== archiveId) };
  });
}
