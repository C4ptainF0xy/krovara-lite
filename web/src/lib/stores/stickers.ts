import { writable, get } from 'svelte/store';
import { api, authedObjectURL } from '$lib/api';

export type CustomSticker = {
  id: string;
  space_id: string;
  name: string;
  file_key: string;
  animated: boolean;
};

export const stickersBySpace = writable<Record<string, CustomSticker[]>>({});
const urlCache = new Map<string, string>();

export const myStickers = writable<CustomSticker[]>([]);
let myStickersLoaded = false;

export async function loadMyStickers(force = false): Promise<CustomSticker[]> {
  if (myStickersLoaded && !force) return get(myStickers);
  const list = await api<CustomSticker[]>('/api/me/stickers');
  myStickers.set(list);
  myStickersLoaded = true;
  return list;
}

export async function loadStickers(spaceId: string): Promise<CustomSticker[]> {
  const list = await api<CustomSticker[]>(`/api/spaces/${spaceId}/stickers`);
  stickersBySpace.update((m) => ({ ...m, [spaceId]: list }));
  return list;
}

export function stickersFor(spaceId: string): CustomSticker[] {
  return get(stickersBySpace)[spaceId] ?? [];
}

export async function stickerUrl(fileKey: string): Promise<string> {
  const hit = urlCache.get(fileKey);
  if (hit) return hit;
  const url = await authedObjectURL(`/api/files/${fileKey}`);
  urlCache.set(fileKey, url);
  return url;
}

export async function uploadSticker(
  spaceId: string,
  name: string,
  file: File
): Promise<CustomSticker> {
  const form = new FormData();
  form.append('file', file);
  const up = await api<{ id: string }>('/api/files?kind=sticker', { method: 'POST', body: form });
  const animated = file.type === 'image/gif' || file.type === 'image/webp';
  const sticker = await api<CustomSticker>(`/api/spaces/${spaceId}/stickers`, {
    method: 'POST',
    body: { name, file_key: up.id, animated }
  });
  stickersBySpace.update((m) => ({
    ...m,
    [spaceId]: [...(m[spaceId] ?? []), sticker].sort((a, b) => a.name.localeCompare(b.name))
  }));
  return sticker;
}

export async function deleteSticker(spaceId: string, stickerId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/stickers/${stickerId}`, { method: 'DELETE' });
  stickersBySpace.update((m) => ({
    ...m,
    [spaceId]: (m[spaceId] ?? []).filter((e) => e.id !== stickerId)
  }));
}
