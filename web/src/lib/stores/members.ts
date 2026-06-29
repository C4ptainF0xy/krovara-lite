import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api';
import { onXmppPresence } from '$lib/xmpp/client';

export type Member = {
  id: string;
  user_id: string;
  username: string;
  nickname: string | null;
  avatar_key: string | null;
  bio?: string | null;
  role_color?: string | null;
  role_icon?: string | null;
  hoist_role?: string | null;
  hoist_position?: number | null;
  badges?: string[];
};

export const membersBySpace = writable<Record<string, Member[]>>({});

export async function loadMembers(spaceId: string): Promise<Member[]> {
  if (!spaceId) return [];
  const data = await api<Member[]>(`/api/spaces/${spaceId}/members`);
  membersBySpace.update((m) => ({ ...m, [spaceId]: data }));
  return data;
}

export async function reloadLoadedSpaces(): Promise<void> {
  const ids = Object.keys(get(membersBySpace));
  await Promise.all(ids.map((id) => loadMembers(id).catch(() => {})));
}

let presenceSyncStarted = false;
const reloadTimers = new Map<string, ReturnType<typeof setTimeout>>();
function scheduleReload(spaceId: string) {
  if (reloadTimers.has(spaceId)) return;
  reloadTimers.set(
    spaceId,
    setTimeout(() => {
      reloadTimers.delete(spaceId);
      void loadMembers(spaceId).catch(() => {});
    }, 400)
  );
}
export function startMemberPresenceSync(): void {
  if (presenceSyncStarted) return;
  presenceSyncStarted = true;
  onXmppPresence(({ userId, room, available }) => {
    if (!available || !room.startsWith('space-')) return;
    const spaceId = room.slice('space-'.length);
    const roster = get(membersBySpace)[spaceId];
    if (roster && !roster.some((m) => m.user_id === userId)) scheduleReload(spaceId);
  });
}

export type SpaceProfile = { nickname: string | null; avatar_key: string | null; bio: string | null };

export async function updateMySpaceProfile(spaceId: string, p: SpaceProfile): Promise<void> {
  await api(`/api/spaces/${spaceId}/members/me/profile`, { method: 'PUT', body: p });
  await loadMembers(spaceId);
}

export function displayName(m: Member): string {
  return m.nickname || m.username;
}

export const memberNames = derived(membersBySpace, ($bySpace) => {
  const out: Record<string, string> = {};
  for (const list of Object.values($bySpace)) {
    for (const m of list) out[m.user_id] = displayName(m);
  }
  return out;
});
