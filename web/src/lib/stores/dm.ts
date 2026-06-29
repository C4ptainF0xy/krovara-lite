import { writable, get } from 'svelte/store';
import { onMessage, type IncomingMessage } from '$lib/xmpp/client';
import { sendDirectMessage, retractDirectMessage, fetchDmHistory, myUuid } from '$lib/xmpp/dm';

export type DmMessage = IncomingMessage & { peer: string };

const MAX_PER_PEER = 200;

export const dmByPeer = writable<Record<string, DmMessage[]>>({});
export const dmUnread = writable<Record<string, number>>({});

let activePeer: string | null = null;
let subscribed = false;

function insert(peer: string, m: DmMessage) {
  dmByPeer.update((map) => {
    const cur = map[peer] ?? [];
    if (cur.some((x) => x.id === m.id || (m.originId && x.originId === m.originId))) return map;
    const next = [...cur, m].slice(-MAX_PER_PEER);
    return { ...map, [peer]: next };
  });
}

function applyEdit(peer: string, targetId: string, body: string) {
  dmByPeer.update((map) => {
    const cur = map[peer];
    if (!cur) return map;
    return {
      ...map,
      [peer]: cur.map((x) =>
        x.id === targetId || x.originId === targetId ? { ...x, body, edited: true } : x
      )
    };
  });
}

function applyRetract(peer: string, targetId: string) {
  dmByPeer.update((map) => {
    const cur = map[peer];
    if (!cur) return map;
    return { ...map, [peer]: cur.filter((x) => x.id !== targetId && x.originId !== targetId) };
  });
}

export function startDmIngest(): void {
  if (subscribed) return;
  subscribed = true;
  onMessage((m) => {
    if (m.kind !== 'dm' || !m.peer) return;
    if (m.retractId) {
      applyRetract(m.peer, m.retractId);
      return;
    }
    if (m.replaceId) {
      applyEdit(m.peer, m.replaceId, m.body);
      return;
    }
    insert(m.peer, m as DmMessage);
    if (!m.mine && !m.fromHistory && m.peer !== activePeer) {
      dmUnread.update((u) => ({ ...u, [m.peer!]: (u[m.peer!] ?? 0) + 1 }));
    }
  });
}

export function openConversation(peerUuid: string): void {
  activePeer = peerUuid;
  dmUnread.update((u) => ({ ...u, [peerUuid]: 0 }));
  if (!(peerUuid in get(dmByPeer))) {
    void fetchDmHistory(peerUuid).catch(() => {});
  }
}

export function closeConversation(): void {
  activePeer = null;
}

export async function sendDm(
  peerUuid: string,
  body: string,
  opts?: { oobs?: { url: string; name?: string }[] }
): Promise<void> {
  const id = await sendDirectMessage(peerUuid, body, opts);
  insert(peerUuid, {
    id,
    originId: id,
    from: myUuid(),
    channelId: myUuid(),
    kind: 'dm',
    peer: peerUuid,
    mine: true,
    body,
    oobs: opts?.oobs,
    at: new Date()
  });
}

export async function editDm(peerUuid: string, message: DmMessage, body: string): Promise<void> {
  const target = message.originId ?? message.id;
  applyEdit(peerUuid, target, body);
  await sendDirectMessage(peerUuid, body, { replaceId: target });
}

export async function deleteDm(peerUuid: string, message: DmMessage): Promise<void> {
  const target = message.originId ?? message.id;
  applyRetract(peerUuid, target);
  await retractDirectMessage(peerUuid, target);
}

export function totalDmUnread(): number {
  return Object.values(get(dmUnread)).reduce((n, v) => n + v, 0);
}
