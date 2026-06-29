import { writable, get } from 'svelte/store';
import { api, authedObjectURL } from '$lib/api';

export type CustomEmoji = {
  id: string;
  space_id: string;
  name: string;
  file_key: string;
  animated: boolean;
};

export const emojisBySpace = writable<Record<string, CustomEmoji[]>>({});

export const myEmojis = writable<CustomEmoji[]>([]);
let myEmojisLoaded = false;

export async function loadMyEmojis(force = false): Promise<CustomEmoji[]> {
  if (myEmojisLoaded && !force) return get(myEmojis);
  const list = await api<CustomEmoji[]>('/api/me/emojis');
  myEmojis.set(list);
  myEmojisLoaded = true;
  return list;
}
const urlCache = new Map<string, string>();

export async function loadEmojis(spaceId: string): Promise<CustomEmoji[]> {
  const list = await api<CustomEmoji[]>(`/api/spaces/${spaceId}/emojis`);
  emojisBySpace.update((m) => ({ ...m, [spaceId]: list }));
  return list;
}

export function emojisFor(spaceId: string): CustomEmoji[] {
  return get(emojisBySpace)[spaceId] ?? [];
}

export async function emojiUrl(fileKey: string): Promise<string> {
  const hit = urlCache.get(fileKey);
  if (hit) return hit;
  const url = await authedObjectURL(`/api/files/${fileKey}`);
  urlCache.set(fileKey, url);
  return url;
}

export async function uploadEmoji(
  spaceId: string,
  name: string,
  file: File
): Promise<CustomEmoji> {
  const form = new FormData();
  form.append('file', file);
  const up = await api<{ id: string }>('/api/files?kind=emoji', { method: 'POST', body: form });
  const animated = file.type === 'image/gif' || file.type === 'image/webp';
  const emoji = await api<CustomEmoji>(`/api/spaces/${spaceId}/emojis`, {
    method: 'POST',
    body: { name, file_key: up.id, animated }
  });
  emojisBySpace.update((m) => ({ ...m, [spaceId]: [...(m[spaceId] ?? []), emoji].sort((a, b) => a.name.localeCompare(b.name)) }));
  return emoji;
}

export async function deleteEmoji(spaceId: string, emojiId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/emojis/${emojiId}`, { method: 'DELETE' });
  emojisBySpace.update((m) => ({ ...m, [spaceId]: (m[spaceId] ?? []).filter((e) => e.id !== emojiId) }));
}
