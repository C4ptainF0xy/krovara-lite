import { writable, derived } from 'svelte/store';
import { api } from '$lib/api';

export type SavedMessage = {
  archive_id: string;
  channel_id: string;
  space_id: string;
  folder: string;
  saved_at: string;
  missing: boolean;
  author_id: string;
  body: string;
  at?: string;
};

export const saved = writable<SavedMessage[]>([]);

export const savedIds = derived(saved, ($s) => new Set($s.map((m) => m.archive_id)));

let loaded = false;
export async function ensureSavesLoaded(): Promise<void> {
  if (loaded) return;
  loaded = true;
  try {
    saved.set(await api<SavedMessage[]>('/api/me/saves'));
  } catch (e) {
    loaded = false;
    throw e;
  }
}

export async function reloadSaves(): Promise<void> {
  saved.set(await api<SavedMessage[]>('/api/me/saves'));
}

export async function saveMessage(
  channelId: string,
  archiveId: string,
  folder = ''
): Promise<void> {
  const sv = await api<SavedMessage>('/api/me/saves', {
    method: 'POST',
    body: { channel_id: channelId, archive_id: archiveId, folder }
  });
  saved.update((list) => [sv, ...list.filter((m) => m.archive_id !== archiveId)]);
}

export async function unsaveMessage(archiveId: string): Promise<void> {
  await api(`/api/me/saves/${encodeURIComponent(archiveId)}`, { method: 'DELETE' });
  saved.update((list) => list.filter((m) => m.archive_id !== archiveId));
}
