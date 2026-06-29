import { client as createClient, xml } from '@xmpp/client/browser';
import { writable, type Readable } from 'svelte/store';
import { api } from '$lib/api';
import { wsUrl } from '$lib/config';

export type ConnectionState = 'idle' | 'connecting' | 'online' | 'offline' | 'error';

export type IncomingMessage = {
  id: string;
  from: string;
  fromResource?: string;
  channelId: string;
  body: string;
  at: Date;
  kind?: 'muc' | 'dm';
  peer?: string;
  mine?: boolean;
  fromHistory?: boolean;
  originId?: string;
  replaceId?: string;
  edited?: boolean;
  retractId?: string;
  replyToId?: string;
  oobUrl?: string;
  oobName?: string;
  oobs?: { url: string; name?: string; spoiler?: boolean }[];
};

type AppXmppClient = ReturnType<typeof createClient>;

const _state = writable<ConnectionState>('idle');
export const xmppState: Readable<ConnectionState> = { subscribe: _state.subscribe };

const messageListeners = new Set<(m: IncomingMessage) => void>();
export function onMessage(fn: (m: IncomingMessage) => void): () => void {
  messageListeners.add(fn);
  return () => messageListeners.delete(fn);
}

export type PresenceUpdate = {
  userId: string;
  room: string;
  available: boolean;
  show?: string;
  statusText?: string;
};
const presenceListeners = new Set<(p: PresenceUpdate) => void>();
export function onXmppPresence(fn: (p: PresenceUpdate) => void): () => void {
  presenceListeners.add(fn);
  return () => presenceListeners.delete(fn);
}

export type ReactionUpdate = {
  channelId: string;
  targetId: string;
  userId: string;
  emojis: string[];
};
const reactionListeners = new Set<(r: ReactionUpdate) => void>();
export function onReaction(fn: (r: ReactionUpdate) => void): () => void {
  reactionListeners.add(fn);
  return () => reactionListeners.delete(fn);
}

export type ReadMarker = { channelId: string; userId: string; messageId: string };
const readListeners = new Set<(m: ReadMarker) => void>();
export function onReadMarker(fn: (m: ReadMarker) => void): () => void {
  readListeners.add(fn);
  return () => readListeners.delete(fn);
}

export type ChatStateUpdate = { channelId: string; userId: string; state: string };
const chatStateListeners = new Set<(c: ChatStateUpdate) => void>();
export function onChatState(fn: (c: ChatStateUpdate) => void): () => void {
  chatStateListeners.add(fn);
  return () => chatStateListeners.delete(fn);
}
const NS_CHATSTATES = 'http://jabber.org/protocol/chatstates';

let activeClient: AppXmppClient | null = null;
let backoffMs = 1000;
let stopRequested = false;
let connectPromise: Promise<void> | null = null;

let boundLocal: string | null = null;
export function boundNickname(): string | null {
  return boundLocal;
}

const MAX_BACKOFF = 30_000;

export function start(): Promise<void> {
  if (activeClient || connectPromise) return connectPromise ?? Promise.resolve();
  stopRequested = false;
  connectPromise = connectOnce();
  return connectPromise;
}

export function stop(): void {
  stopRequested = true;
  if (activeClient) {
    void activeClient.stop().catch(() => {});
    activeClient = null;
  }
  _state.set('idle');
}

export function client(): AppXmppClient | null {
  return activeClient;
}

async function connectOnce(): Promise<void> {
  _state.set('connecting');
  try {
    const tok = await api<{ token: string; jid: string; expires_at: string }>(
      '/api/xmpp/token',
      { method: 'POST' }
    );
    const [local, domain] = tok.jid.split('@');
    if (!local || !domain) throw new Error('invalid jid');
    boundLocal = local;

    const wsURL = wsUrl('/xmpp-websocket');
    const xmpp = createClient({
      service: wsURL,
      domain,
      username: local,
      password: tok.token,
      resource: cryptoResource()
    });

    activeClient = xmpp;

    xmpp.on('online', async () => {
      backoffMs = 1000;
      await xmpp.send(
        xml('iq', { type: 'set' }, xml('enable', { xmlns: 'urn:xmpp:carbons:2' }))
      ).catch(console.warn);
      await xmpp.send(xml('presence', {})).catch(console.warn);
      _state.set('online');
    });
    xmpp.on('offline', () => {
      _state.set('offline');
      if (!stopRequested) scheduleReconnect();
    });
    xmpp.on('error', (err: unknown) => {
      console.warn('[xmpp] error', err);
      _state.set('error');
    });
    xmpp.on('stanza', handleStanza);

    await xmpp.start();
  } catch (err) {
    console.warn('[xmpp] connect failed', err);
    _state.set('error');
    if (!stopRequested) scheduleReconnect();
  } finally {
    connectPromise = null;
  }
}

function scheduleReconnect(): void {
  const delay = backoffMs;
  backoffMs = Math.min(MAX_BACKOFF, backoffMs * 2);
  setTimeout(() => {
    activeClient = null;
    if (!stopRequested) void start();
  }, delay);
}

function handleStanza(raw: unknown): void {
  const stanza = raw as XmlNode & { is: (n: string) => boolean };
  if (typeof stanza?.is !== 'function') return;
  if (stanza.is('message')) {
    if (presenceParser(stanza)) return;
    const reaction = parseReactions(stanza);
    if (reaction) {
      reactionListeners.forEach((fn) => fn(reaction));
      return;
    }
    const marker = parseReadMarker(stanza);
    if (marker) {
      readListeners.forEach((fn) => fn(marker));
      return;
    }
    const cs = parseChatState(stanza);
    if (cs) chatStateListeners.forEach((fn) => fn(cs));
    const m = parseMessage(stanza);
    if (m) messageListeners.forEach((fn) => fn(m));
  } else if (stanza.is('iq')) {
    iqHandlers.forEach((fn) => fn(stanza));
  } else if (stanza.is('presence')) {
    const p = parsePresence(stanza);
    if (p) presenceListeners.forEach((fn) => fn(p));
  }
}

function parsePresence(stanza: XmlNode): PresenceUpdate | null {
  const from = stanza.attrs.from ?? '';
  const [bare, resource] = splitJid(from);
  if (!resource) return null;
  const domain = bare.split('@')[1] ?? '';
  if (!domain.startsWith('conference.')) return null;
  return {
    userId: resource,
    room: bare.split('@')[0] ?? '',
    available: stanza.attrs.type !== 'unavailable',
    show: stanza.getChildText('show') || undefined,
    statusText: stanza.getChildText('status') || undefined
  };
}

const iqHandlers = new Set<(stanza: XmlNode) => void>();
export function registerIQHandler(fn: (stanza: XmlNode) => void): () => void {
  iqHandlers.add(fn);
  return () => iqHandlers.delete(fn);
}

export type XmlNode = {
  attrs: Record<string, string>;
  getChild: (name: string, ns?: string) => XmlNode | undefined;
  getChildText: (name: string) => string | undefined;
};

let presenceParser: (stanza: XmlNode) => boolean = () => false;
export function registerPresenceParser(p: (stanza: XmlNode) => boolean): void {
  presenceParser = p;
}

const NS_REACTIONS = 'urn:xmpp:reactions:0';
function parseReactions(stanza: XmlNode): ReactionUpdate | null {
  type El = {
    attrs: Record<string, string>;
    getChild: (n: string, ns?: string) => El | undefined;
    getChildren: (n: string, ns?: string) => El[];
    text: () => string;
  };
  let el = stanza as unknown as El;

  const mam = el.getChild('result', 'urn:xmpp:mam:2');
  if (mam) {
    const inner = mam.getChild('forwarded', 'urn:xmpp:forward:0')?.getChild('message');
    if (!inner) return null;
    el = inner;
  }

  const reactions = el.getChild('reactions', NS_REACTIONS);
  if (!reactions) return null;
  const targetId = reactions.attrs?.id;
  if (!targetId) return null;

  const fromFull = el.attrs?.from ?? '';
  const [bareRoom, resource] = splitJid(fromFull);
  const [channelId] = bareRoom.split('@');
  const emojis = reactions
    .getChildren('reaction', NS_REACTIONS)
    .map((c) => c.text().trim())
    .filter(Boolean);
  return { channelId: channelId ?? '', targetId, userId: resource, emojis };
}

function parseChatState(stanza: XmlNode): ChatStateUpdate | null {
  const [bareRoom, resource] = splitJid(stanza.attrs?.from ?? '');
  const domain = bareRoom.split('@')[1] ?? '';
  if (!domain.startsWith('conference.') || !resource) return null;
  const [channelId] = bareRoom.split('@');
  for (const state of ['composing', 'paused', 'active', 'inactive', 'gone']) {
    if (stanza.getChild(state, NS_CHATSTATES)) {
      return { channelId: channelId ?? '', userId: resource, state };
    }
  }
  return null;
}

function parseReadMarker(stanza: XmlNode): ReadMarker | null {
  const displayed = stanza.getChild('displayed', 'urn:xmpp:chat-markers:0');
  const messageId = displayed?.attrs?.id;
  if (!messageId) return null;
  const [bareRoom, resource] = splitJid(stanza.attrs?.from ?? '');
  const domain = bareRoom.split('@')[1] ?? '';
  if (!domain.startsWith('conference.') || !resource) return null;
  const [channelId] = bareRoom.split('@');
  return { channelId: channelId ?? '', userId: resource, messageId };
}

function parseMessage(stanza: XmlNode): IncomingMessage | null {
  const mamResult = stanza.getChild('result', 'urn:xmpp:mam:2');
  if (mamResult) {
    const fwd = mamResult.getChild('forwarded', 'urn:xmpp:forward:0');
    const inner = fwd?.getChild('message');
    if (inner) {
      const archiveID = mamResult.attrs?.id;
      const stamp = fwd?.getChild('delay', 'urn:xmpp:delay')?.attrs?.stamp;
      const at = stamp ? new Date(stamp) : undefined;
      const m = parseLive(inner, archiveID, at && !isNaN(at.getTime()) ? at : undefined);
      if (m) m.fromHistory = true;
      return m;
    }
  }

  const carbonSent = stanza.getChild('sent', 'urn:xmpp:carbons:2');
  const carbonReceived = stanza.getChild('received', 'urn:xmpp:carbons:2');
  const carbon = carbonSent || carbonReceived;
  if (carbon) {
    const fwd = carbon.getChild('forwarded', 'urn:xmpp:forward:0');
    const inner = fwd?.getChild('message');
    if (inner) {
      return parseLive(inner, undefined);
    }
  }

  return parseLive(stanza, undefined);
}

function parseLive(stanza: XmlNode, archiveID?: string, at?: Date): IncomingMessage | null {
  const body = stanza.getChildText('body');
  if (!body) return null;
  const fromFull = stanza.attrs.from ?? '';
  const stanzaIDEl = stanza.getChild('stanza-id', 'urn:xmpp:sid:0');
  const id =
    archiveID ??
    stanzaIDEl?.attrs?.id ??
    stanza.attrs.id ??
    cryptoResource();
  const [bareRoom, resource] = splitJid(fromFull);
  const [channelId, fromDomain] = bareRoom.split('@');
  const isDm = !!fromDomain && !fromDomain.startsWith('conference.');
  let peer: string | undefined;
  let mine = false;
  if (isDm) {
    const me = boundNickname() ?? '';
    const [toLocal] = splitJid(stanza.attrs.to ?? '')[0].split('@');
    mine = !!me && channelId === me;
    peer = mine ? toLocal : channelId;
  }
  const originId = stanza.getChild('origin-id', 'urn:xmpp:sid:0')?.attrs?.id;
  const replaceId = stanza.getChild('replace', 'urn:xmpp:message-correct:0')?.attrs?.id;
  const retractId = stanza.getChild('retract', 'urn:xmpp:message-retract:0')?.attrs?.id;
  const replyToId = stanza.getChild('reply', 'urn:xmpp:reply:0')?.attrs?.id;
  const oobEls = (
    stanza as unknown as { getChildren: (n: string, ns?: string) => XmlNode[] }
  ).getChildren('x', 'jabber:x:oob');
  const oobs = oobEls
    .map((el) => ({
      url: el.getChildText('url') || '',
      name: el.getChildText('desc') || undefined,
      spoiler: (el as unknown as { attrs?: Record<string, string> }).attrs?.spoiler === '1'
    }))
    .filter((o) => o.url);
  let when = at;
  if (!when) {
    const stamp = stanza.getChild('delay', 'urn:xmpp:delay')?.attrs?.stamp;
    if (stamp) {
      const d = new Date(stamp);
      if (!isNaN(d.getTime())) when = d;
    }
  }
  return {
    id,
    from: bareRoom,
    fromResource: resource,
    channelId: channelId ?? '',
    kind: isDm ? 'dm' : 'muc',
    peer,
    mine,
    body,
    at: when ?? new Date(),
    originId,
    replaceId,
    retractId,
    replyToId,
    oobUrl: oobs[0]?.url,
    oobName: oobs[0]?.name,
    oobs: oobs.length ? oobs : undefined
  };
}

function splitJid(jid: string): [string, string] {
  const slash = jid.indexOf('/');
  if (slash < 0) return [jid, ''];
  return [jid.slice(0, slash), jid.slice(slash + 1)];
}

function cryptoResource(): string {
  const buf = new Uint8Array(6);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('');
}

export { xml };
