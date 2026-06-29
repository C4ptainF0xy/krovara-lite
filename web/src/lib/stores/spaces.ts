import { writable, get } from 'svelte/store';
import { api } from '$lib/api';

export type Space = {
  id: string;
  owner_id: string;
  name: string;
  icon_key: string | null;
  description?: string | null;
  rules?: string | null;
  banner_key?: string | null;
  tags?: string[];
  language?: string | null;
  vanity_slug?: string | null;
  created_at: string;
};

export type Channel = {
  id: string;
  space_id: string;
  name: string;
  topic: string | null;
  type: string | null;
  position: number | null;
  is_private: boolean | null;
  category_id?: string | null;
  slowmode_seconds?: number;
  nsfw?: boolean;
  read_only?: boolean;
  icon_emoji?: string | null;
  locked?: boolean;
  locked_by?: string;
  locked_at?: string;
  created_at: string;
};

export type Category = {
  id: string;
  space_id: string;
  name: string;
  position: number;
  created_at: string;
};

type AsyncSlice<T> = {
  data: T;
  loading: boolean;
  error: string | null;
};

function empty<T>(seed: T): AsyncSlice<T> {
  return { data: seed, loading: false, error: null };
}

export const spaces = writable<AsyncSlice<Space[]>>(empty<Space[]>([]));

export const channelsBySpace = writable<Record<string, AsyncSlice<Channel[]>>>({});

export const categoriesBySpace = writable<Record<string, AsyncSlice<Category[]>>>({});

export async function loadSpaces(): Promise<void> {
  spaces.update((s) => ({ ...s, loading: true, error: null }));
  try {
    const data = await api<Space[]>('/api/spaces');
    spaces.set({ data, loading: false, error: null });
  } catch (e) {
    spaces.update((s) => ({ ...s, loading: false, error: errMsg(e) }));
  }
}

export async function createSpace(name: string): Promise<Space> {
  const created = await api<Space>('/api/spaces', { method: 'POST', body: { name } });
  spaces.update((s) => ({ ...s, data: [...s.data, created] }));
  return created;
}

function patchSpaceInStore(updated: Space): void {
  spaces.update((s) => ({ ...s, data: s.data.map((sp) => (sp.id === updated.id ? updated : sp)) }));
}

export type SpaceSettingsPatch = {
  name?: string;
  description?: string | null;
  rules?: string | null;
  banner_key?: string | null;
  icon_key?: string | null;
  tags?: string[];
  language?: string | null;
};

export async function updateSpaceSettings(spaceId: string, patch: SpaceSettingsPatch): Promise<Space> {
  const updated = await api<Space>(`/api/spaces/${spaceId}`, { method: 'PATCH', body: patch });
  patchSpaceInStore(updated);
  return updated;
}

export async function setVanity(spaceId: string, slug: string | null): Promise<Space> {
  const updated = await api<Space>(`/api/spaces/${spaceId}/vanity`, {
    method: 'PUT',
    body: { slug }
  });
  patchSpaceInStore(updated);
  return updated;
}

export async function transferOwnership(spaceId: string, newOwnerId: string): Promise<Space> {
  const updated = await api<Space>(`/api/spaces/${spaceId}/transfer`, {
    method: 'POST',
    body: { new_owner_id: newOwnerId }
  });
  patchSpaceInStore(updated);
  return updated;
}

export async function deleteSpaceSecure(spaceId: string, password: string): Promise<void> {
  await api(`/api/spaces/${spaceId}`, { method: 'DELETE', body: { password } });
  spaces.update((s) => ({ ...s, data: s.data.filter((sp) => sp.id !== spaceId) }));
  channelsBySpace.update((m) => {
    const next = { ...m };
    delete next[spaceId];
    return next;
  });
}

export async function leaveSpace(spaceId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/members/me/leave`, { method: 'POST' });
  spaces.update((s) => ({ ...s, data: s.data.filter((sp) => sp.id !== spaceId) }));
  channelsBySpace.update((m) => {
    const next = { ...m };
    delete next[spaceId];
    return next;
  });
}

export async function loadChannels(spaceId: string): Promise<void> {
  channelsBySpace.update((m) => ({
    ...m,
    [spaceId]: { ...(m[spaceId] ?? empty<Channel[]>([])), loading: true, error: null }
  }));
  try {
    const data = await api<Channel[]>(`/api/spaces/${spaceId}/channels`);
    channelsBySpace.update((m) => ({
      ...m,
      [spaceId]: { data, loading: false, error: null }
    }));
  } catch (e) {
    channelsBySpace.update((m) => ({
      ...m,
      [spaceId]: { data: m[spaceId]?.data ?? [], loading: false, error: errMsg(e) }
    }));
  }
}

export async function createChannel(
  spaceId: string,
  name: string,
  type: 'text' | 'voice' = 'text',
  categoryId?: string | null
): Promise<Channel> {
  const created = await api<Channel>(`/api/spaces/${spaceId}/channels`, {
    method: 'POST',
    body: { name, type, category_id: categoryId ?? undefined }
  });
  channelsBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return { ...m, [spaceId]: { data: [...cur, created], loading: false, error: null } };
  });
  return created;
}

export async function loadCategories(spaceId: string): Promise<void> {
  categoriesBySpace.update((m) => ({
    ...m,
    [spaceId]: { ...(m[spaceId] ?? empty<Category[]>([])), loading: true, error: null }
  }));
  try {
    const data = await api<Category[]>(`/api/spaces/${spaceId}/categories`);
    categoriesBySpace.update((m) => ({ ...m, [spaceId]: { data, loading: false, error: null } }));
  } catch (e) {
    categoriesBySpace.update((m) => ({
      ...m,
      [spaceId]: { data: m[spaceId]?.data ?? [], loading: false, error: errMsg(e) }
    }));
  }
}

export async function createCategory(spaceId: string, name: string): Promise<Category> {
  const created = await api<Category>(`/api/spaces/${spaceId}/categories`, {
    method: 'POST',
    body: { name }
  });
  categoriesBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return { ...m, [spaceId]: { data: [...cur, created], loading: false, error: null } };
  });
  return created;
}

export async function updateCategory(
  spaceId: string,
  categoryId: string,
  patch: { name?: string; position?: number }
): Promise<Category> {
  const updated = await api<Category>(`/api/spaces/${spaceId}/categories/${categoryId}`, {
    method: 'PATCH',
    body: patch
  });
  categoriesBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: {
        ...(m[spaceId] ?? empty<Category[]>([])),
        data: cur.map((c) => (c.id === categoryId ? updated : c))
      }
    };
  });
  return updated;
}

export async function deleteCategory(spaceId: string, categoryId: string): Promise<void> {
  await api(`/api/spaces/${spaceId}/categories/${categoryId}`, { method: 'DELETE' });
  categoriesBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: { ...(m[spaceId] ?? empty<Category[]>([])), data: cur.filter((c) => c.id !== categoryId) }
    };
  });
  channelsBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: {
        ...(m[spaceId] ?? empty<Channel[]>([])),
        data: cur.map((c) => (c.category_id === categoryId ? { ...c, category_id: null } : c))
      }
    };
  });
}

export type Overwrite = {
  target_type: 'role' | 'member';
  target_id: string;
  allow: number;
  deny: number;
};

export async function loadOverwrites(channelId: string): Promise<Overwrite[]> {
  return api<Overwrite[]>(`/api/channels/${channelId}/overwrites`);
}

export async function setOverwrite(
  channelId: string,
  targetType: 'role' | 'member',
  targetId: string,
  allow: number,
  deny: number
): Promise<Overwrite> {
  return api<Overwrite>(`/api/channels/${channelId}/overwrites/${targetType}/${targetId}`, {
    method: 'PUT',
    body: { allow, deny }
  });
}

export async function clearOverwrite(
  channelId: string,
  targetType: 'role' | 'member',
  targetId: string
): Promise<void> {
  await api(`/api/channels/${channelId}/overwrites/${targetType}/${targetId}`, { method: 'DELETE' });
}

export type ChannelSettings = {
  name?: string;
  topic?: string | null;
  slowmode_seconds?: number;
  nsfw?: boolean;
  read_only?: boolean;
  icon_emoji?: string | null;
};

export async function updateChannel(
  spaceId: string,
  channelId: string,
  patch: ChannelSettings
): Promise<Channel> {
  const updated = await api<Channel>(`/api/channels/${channelId}`, {
    method: 'PATCH',
    body: patch
  });
  channelsBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: {
        ...(m[spaceId] ?? empty<Channel[]>([])),
        data: cur.map((c) => (c.id === channelId ? updated : c))
      }
    };
  });
  return updated;
}

export async function moveChannel(
  spaceId: string,
  channelId: string,
  categoryId: string | null,
  position: number
): Promise<Channel> {
  const updated = await api<Channel>(`/api/channels/${channelId}/move`, {
    method: 'PATCH',
    body: { category_id: categoryId, position }
  });
  channelsBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: {
        ...(m[spaceId] ?? empty<Channel[]>([])),
        data: cur.map((c) => (c.id === channelId ? updated : c))
      }
    };
  });
  return updated;
}

export async function setChannelLock(
  spaceId: string,
  channelId: string,
  locked: boolean
): Promise<Channel> {
  const updated = await api<Channel>(`/api/channels/${channelId}/lock`, {
    method: 'PUT',
    body: { locked }
  });
  channelsBySpace.update((m) => {
    const cur = m[spaceId]?.data ?? [];
    return {
      ...m,
      [spaceId]: {
        ...(m[spaceId] ?? empty<Channel[]>([])),
        data: cur.map((c) => (c.id === channelId ? updated : c))
      }
    };
  });
  return updated;
}

export function channelsFor(spaceId: string): AsyncSlice<Channel[]> {
  return get(channelsBySpace)[spaceId] ?? empty<Channel[]>([]);
}

function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : 'request failed';
}
