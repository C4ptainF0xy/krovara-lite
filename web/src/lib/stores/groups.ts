import { writable, get } from 'svelte/store';
import { api } from '$lib/api';

export type GroupMember = { id: string; username: string; display_name: string; avatar_key: string | null };
export type DMGroup = {
  id: string;
  owner_id: string;
  name: string | null;
  icon_key: string | null;
  member_count?: number;
  members?: GroupMember[];
};
export type GroupMessage = {
  id: string;
  author_id: string;
  username?: string;
  avatar_key?: string | null;
  body: string;
  at: string;
  mine?: boolean;
};

export const myGroups = writable<DMGroup[]>([]);
export const groupById = writable<Record<string, DMGroup>>({});
export const groupMessages = writable<Record<string, GroupMessage[]>>({});
export const groupUnread = writable<Record<string, number>>({});

let activeGroup: string | null = null;

export async function loadMyGroups(): Promise<DMGroup[]> {
  const list = await api<DMGroup[]>('/api/dm-groups');
  myGroups.set(list);
  return list;
}

export async function loadGroup(id: string): Promise<DMGroup> {
  const g = await api<DMGroup>(`/api/dm-groups/${id}`);
  groupById.update((m) => ({ ...m, [id]: g }));
  return g;
}

export async function loadGroupMessages(id: string): Promise<void> {
  const msgs = await api<GroupMessage[]>(`/api/dm-groups/${id}/messages`);
  groupMessages.update((m) => ({ ...m, [id]: msgs }));
}

export function openGroup(id: string): void {
  activeGroup = id;
  groupUnread.update((u) => ({ ...u, [id]: 0 }));
  if (!(id in get(groupMessages))) void loadGroupMessages(id).catch(() => {});
  void loadGroup(id).catch(() => {});
}

export function closeGroup(): void {
  activeGroup = null;
}

export async function createGroup(memberIds: string[], name?: string): Promise<DMGroup> {
  const g = await api<DMGroup>('/api/dm-groups', { method: 'POST', body: { member_ids: memberIds, name } });
  await loadMyGroups().catch(() => {});
  return g;
}

export async function sendGroupMessage(id: string, body: string): Promise<void> {
  await api(`/api/dm-groups/${id}/messages`, { method: 'POST', body: { body } });
}

function patchGroupLocal(id: string, patch: Partial<DMGroup>): void {
  groupById.update((m) => (m[id] ? { ...m, [id]: { ...m[id], ...patch } } : m));
  myGroups.update((list) => list.map((g) => (g.id === id ? { ...g, ...patch } : g)));
}

export async function renameGroup(id: string, name: string): Promise<void> {
  await api(`/api/dm-groups/${id}`, { method: 'PATCH', body: { name } });
  patchGroupLocal(id, { name });
}
export async function setGroupIcon(id: string, iconKey: string): Promise<void> {
  await api(`/api/dm-groups/${id}`, { method: 'PATCH', body: { icon_key: iconKey } });
  patchGroupLocal(id, { icon_key: iconKey });
}
export async function leaveGroup(id: string): Promise<void> {
  await api(`/api/dm-groups/${id}/leave`, { method: 'POST' });
  await loadMyGroups().catch(() => {});
}
export async function kickFromGroup(id: string, userId: string): Promise<void> {
  await api(`/api/dm-groups/${id}/members/${userId}`, { method: 'DELETE' });
}
export async function transferGroup(id: string, userId: string): Promise<void> {
  await api(`/api/dm-groups/${id}/transfer`, { method: 'POST', body: { user_id: userId } });
}
export async function createGroupInvite(id: string): Promise<string> {
  const r = await api<{ code: string }>(`/api/dm-groups/${id}/invites`, { method: 'POST' });
  return r.code;
}
export async function listGroupInvites(id: string): Promise<{ code: string }[]> {
  return api<{ code: string }[]>(`/api/dm-groups/${id}/invites`);
}
export async function revokeGroupInvite(id: string, code: string): Promise<void> {
  await api(`/api/dm-groups/${id}/invites/${code}`, { method: 'DELETE' });
}
export async function joinGroup(code: string): Promise<string> {
  const r = await api<{ group_id: string }>(`/api/dm-groups/join/${code}`, { method: 'POST' });
  await loadMyGroups().catch(() => {});
  return r.group_id;
}

export function onGroupMessage(selfId: string, data: { group_id: string; id: string; author_id: string; body: string; at: string }): void {
  const gid = data.group_id;
  const msg: GroupMessage = {
    id: data.id,
    author_id: data.author_id,
    body: data.body,
    at: data.at,
    mine: data.author_id === selfId
  };
  groupMessages.update((m) => {
    const cur = m[gid] ?? [];
    if (cur.some((x) => x.id === msg.id)) return m;
    return { ...m, [gid]: [...cur, msg] };
  });
  if (!msg.mine && gid !== activeGroup) {
    groupUnread.update((u) => ({ ...u, [gid]: (u[gid] ?? 0) + 1 }));
  }
}

export function onGroupUpdate(data: { group_id: string; kind: string }): void {
  void loadMyGroups().catch(() => {});
  if (data.kind === 'left' || data.kind === 'kicked') {
    if (activeGroup === data.group_id) activeGroup = null;
  } else {
    void loadGroup(data.group_id).catch(() => {});
  }
}

export function totalGroupUnread(): number {
  return Object.values(get(groupUnread)).reduce((n, v) => n + v, 0);
}
