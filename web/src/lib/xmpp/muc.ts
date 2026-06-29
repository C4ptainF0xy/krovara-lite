import { client, xml, boundNickname } from './client';
import { snapshot } from '$lib/stores/auth';

const MUC_COMPONENT_PREFIX = 'conference.';
const MAM_NS = 'urn:xmpp:mam:2';
const RSM_NS = 'http://jabber.org/protocol/rsm';

function mucDomain(): string {
  const access = snapshot();
  const c = client();
  const local = c ? (c.options as { domain?: string }).domain ?? '' : '';
  return MUC_COMPONENT_PREFIX + local;
  void access;
}

export function roomJID(channelId: string): string {
  return `${channelId}@${mucDomain()}`;
}

function ourNickname(): string {
  return boundNickname() ?? snapshot().user?.id ?? 'user';
}

const joinedRooms = new Set<string>();

export function joinedChannelIds(): string[] {
  return [...joinedRooms];
}

export async function joinMUC(channelId: string, show?: string, statusText?: string): Promise<void> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  joinedRooms.add(channelId);
  await c.send(
    xml(
      'presence',
      { to: `${roomJID(channelId)}/${ourNickname()}` },
      xml('x', { xmlns: 'http://jabber.org/protocol/muc' }),
      ...presenceChildren(show, statusText)
    )
  );
}

export function spacePresenceRoomId(spaceId: string): string {
  return 'space-' + spaceId;
}

export async function joinSpacePresence(spaceId: string): Promise<void> {
  await joinMUC(spacePresenceRoomId(spaceId));
}

export function broadcastStatus(show?: string, statusText?: string): void {
  const c = client();
  if (!c) return;
  const kids = presenceChildren(show, statusText);
  void c.send(xml('presence', {}, ...kids));
  for (const id of joinedRooms) {
    void c.send(xml('presence', { to: `${roomJID(id)}/${ourNickname()}` }, ...kids));
  }
}

export function broadcastInvisible(): void {
  const c = client();
  if (!c) return;
  for (const id of joinedRooms) {
    void c.send(xml('presence', { to: `${roomJID(id)}/${ourNickname()}`, type: 'unavailable' }));
  }
}

function presenceChildren(show?: string, statusText?: string) {
  const kids = [];
  if (show) kids.push(xml('show', {}, show));
  if (statusText) kids.push(xml('status', {}, statusText));
  return kids;
}

export async function leaveMUC(channelId: string): Promise<void> {
  const c = client();
  joinedRooms.delete(channelId);
  if (!c) return;
  await c.send(
    xml('presence', {
      to: `${roomJID(channelId)}/${ourNickname()}`,
      type: 'unavailable'
    })
  );
}

export async function sendMessage(
  channelId: string,
  body: string,
  opts: {
    replaceId?: string;
    replyToId?: string;
    oobs?: { url: string; desc?: string; spoiler?: boolean }[];
  } = {}
): Promise<string> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  const originId = messageID();
  const children = [
    xml('body', {}, body),
    xml('origin-id', { xmlns: 'urn:xmpp:sid:0', id: originId })
  ];
  if (opts.replaceId) {
    children.push(xml('replace', { xmlns: 'urn:xmpp:message-correct:0', id: opts.replaceId }));
  }
  if (opts.replyToId) {
    children.push(
      xml('reply', { xmlns: 'urn:xmpp:reply:0', to: roomJID(channelId), id: opts.replyToId })
    );
  }
  for (const oob of opts.oobs ?? []) {
    const oobChildren = [xml('url', {}, oob.url)];
    if (oob.desc) oobChildren.push(xml('desc', {}, oob.desc));
    const attrs: Record<string, string> = { xmlns: 'jabber:x:oob' };
    if (oob.spoiler) attrs.spoiler = '1';
    children.push(xml('x', attrs, ...oobChildren));
  }
  await c.send(xml('message', { to: roomJID(channelId), type: 'groupchat', id: originId }, ...children));
  return originId;
}

export async function sendReactions(
  channelId: string,
  targetId: string,
  emojis: string[]
): Promise<void> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  await c.send(
    xml(
      'message',
      { to: roomJID(channelId), type: 'groupchat', id: messageID() },
      xml(
        'reactions',
        { xmlns: 'urn:xmpp:reactions:0', id: targetId },
        ...emojis.map((e) => xml('reaction', {}, e))
      ),
      xml('store', { xmlns: 'urn:xmpp:hints' })
    )
  );
}

export async function sendDisplayed(channelId: string, messageId: string): Promise<void> {
  const c = client();
  if (!c) return;
  await c.send(
    xml(
      'message',
      { to: roomJID(channelId), type: 'groupchat', id: messageID() },
      xml('displayed', { xmlns: 'urn:xmpp:chat-markers:0', id: messageId })
    )
  );
}

export async function sendChatState(channelId: string, state: 'composing' | 'paused' | 'active'): Promise<void> {
  const c = client();
  if (!c) return;
  await c.send(
    xml(
      'message',
      { to: roomJID(channelId), type: 'groupchat', id: messageID() },
      xml(state, { xmlns: 'http://jabber.org/protocol/chatstates' })
    )
  );
}

function messageID(): string {
  const buf = new Uint8Array(8);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('');
}

export async function fetchHistory(channelId: string, count = 50): Promise<void> {
  const c = client();
  if (!c) return;
  await c.send(
    xml(
      'iq',
      { type: 'set', to: roomJID(channelId), id: 'mam-' + Date.now() },
      xml(
        'query',
        { xmlns: MAM_NS, queryid: 'mam-' + channelId },
        xml('set', { xmlns: RSM_NS }, xml('max', {}, String(count)), xml('before', {}))
      )
    )
  );
}

export async function fetchHistoryBefore(
  channelId: string,
  beforeId: string,
  count = 50
): Promise<{ complete: boolean }> {
  const c = client();
  if (!c) return { complete: true };
  const iq = (c as unknown as { iqCaller: { request: (el: unknown) => Promise<unknown> } }).iqCaller;
  const res = (await iq.request(
    xml(
      'iq',
      { type: 'set', to: roomJID(channelId) },
      xml(
        'query',
        { xmlns: MAM_NS, queryid: 'mam-' + channelId },
        xml('set', { xmlns: RSM_NS }, xml('max', {}, String(count)), xml('before', {}, beforeId))
      )
    )
  )) as {
    getChild: (n: string, ns?: string) => { attrs?: Record<string, string> } | undefined;
  };
  const fin = res.getChild('fin', MAM_NS);
  return { complete: fin?.attrs?.complete === 'true' };
}
