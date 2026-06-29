import { writable, derived } from 'svelte/store';
import { api } from '$lib/api';

export type Friend = { id: string; username: string; avatar_key: string | null; since?: string };
export type FriendRequest = { id: string; user_id: string; username: string; avatar_key: string | null };
export type Blocked = { id: string; username: string; avatar_key: string | null };
export type WhoCanAdd = 'everyone' | 'friends_of_friends' | 'nobody';

export const friends = writable<Friend[]>([]);
export const incoming = writable<FriendRequest[]>([]);
export const outgoing = writable<FriendRequest[]>([]);
export const blocked = writable<Blocked[]>([]);

export const blockedIds = derived(blocked, ($b) => new Set($b.map((u) => u.id)));

export async function loadFriends(): Promise<void> {
  friends.set(await api<Friend[]>('/api/me/friends'));
}

export async function loadRequests(): Promise<void> {
  const r = await api<{ incoming: FriendRequest[]; outgoing: FriendRequest[] }>(
    '/api/me/friends/requests'
  );
  incoming.set(r.incoming);
  outgoing.set(r.outgoing);
}

export async function loadBlocks(): Promise<void> {
  blocked.set(await api<Blocked[]>('/api/me/blocks'));
}

export async function sendRequest(handle: string): Promise<void> {
  await api('/api/me/friends', { method: 'POST', body: { handle } });
}

export async function acceptRequest(id: string): Promise<void> {
  await api(`/api/me/friends/${id}/accept`, { method: 'POST' });
}

export async function removeFriendship(id: string): Promise<void> {
  await api(`/api/me/friends/${id}`, { method: 'DELETE' });
}

export async function blockHandle(handle: string): Promise<void> {
  await api('/api/me/blocks', { method: 'POST', body: { handle } });
  await loadBlocks();
}

export async function unblock(userId: string): Promise<void> {
  await api(`/api/me/blocks/${userId}`, { method: 'DELETE' });
  await loadBlocks();
}

export async function setWhoCanAdd(who: WhoCanAdd): Promise<void> {
  await api('/api/me/friend-settings', { method: 'PUT', body: { who_can_add: who } });
}
