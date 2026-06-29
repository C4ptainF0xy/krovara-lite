import { writable, type Readable } from 'svelte/store';
import { client, registerIQHandler, xml, type XmlNode } from '$lib/xmpp/client';
import { newPeer, type Peer } from './peer';

const NS = 'urn:krovara:voip:1';

export type CallDirection = 'outgoing' | 'incoming';

export type SessionInfo = {
  sid: string;
  peerJID: string;
  direction: CallDirection;
  room: string;
};

export type ActiveSession = SessionInfo & {
  peer: Peer;
};

export type IncomingCallHandler = (
  info: SessionInfo,
  accept: () => Promise<void>,
  reject: () => void
) => void;

const incomingHandlers = new Set<IncomingCallHandler>();
const sessionsMap = new Map<string, ActiveSession>();
const _sessions = writable<ActiveSession[]>([]);

export const sessions: Readable<ActiveSession[]> = { subscribe: _sessions.subscribe };

const joinedRooms = new Set<string>();

let installed = false;
function install() {
  if (installed) return;
  installed = true;
  registerIQHandler((stanza) => {
    const v = stanza.getChild('voip', NS);
    if (!v) return;
    void handle(stanza, v);
  });
}

function publish() {
  _sessions.set([...sessionsMap.values()]);
}

async function handle(iq: XmlNode, v: XmlNode): Promise<void> {
  const from = iq.attrs.from ?? '';
  const peerJID = bareJID(from);
  const type = v.attrs.type;
  const sid = v.attrs.sid;
  if (!type || !sid) return;

  switch (type) {
    case 'offer':
      await handleOffer(peerJID, sid, v);
      break;
    case 'answer':
      await handleAnswer(sid, v);
      break;
    case 'candidate':
      await handleCandidate(sid, v);
      break;
    case 'bye':
      handleBye(sid);
      break;
  }
}

async function handleOffer(peerJID: string, sid: string, v: XmlNode): Promise<void> {
  if (sessionsMap.has(sid)) return;
  const sdp = v.getChildText('sdp');
  if (!sdp) return;
  const room = v.attrs.room ?? '';
  const info: SessionInfo = { sid, peerJID, direction: 'incoming', room };

  const accept = async () => {
    const peer = await newPeer();
    sessionsMap.set(sid, { ...info, peer });
    publish();
    await peer.attachLocalMedia(
      room ? { audio: true } : { audio: true, video: true }
    );
    const answer = await peer.acceptOffer({ type: 'offer', sdp });
    peer.onLocalIce((c) => sendCandidate(peerJID, sid, c));
    await sendIQ(peerJID, 'answer', sid, room, xml('sdp', {}, answer.sdp ?? ''));
  };
  const reject = () => {
    void sendIQ(peerJID, 'bye', sid, room);
  };

  if (room && joinedRooms.has(room)) {
    await accept();
    return;
  }
  incomingHandlers.forEach((fn) => fn(info, accept, reject));
}

async function handleAnswer(sid: string, v: XmlNode): Promise<void> {
  const s = sessionsMap.get(sid);
  if (!s) return;
  const sdp = v.getChildText('sdp');
  if (!sdp) return;
  await s.peer.acceptAnswer({ type: 'answer', sdp });
}

async function handleCandidate(sid: string, v: XmlNode): Promise<void> {
  const s = sessionsMap.get(sid);
  if (!s) return;
  const c = v.getChild('candidate');
  if (!c) return;
  await s.peer.addRemoteIce({
    candidate: nodeText(c),
    sdpMid: c.attrs.sdpMid,
    sdpMLineIndex: c.attrs.sdpMLineIndex ? Number(c.attrs.sdpMLineIndex) : undefined
  });
}

function handleBye(sid: string): void {
  const s = sessionsMap.get(sid);
  if (!s) return;
  s.peer.close();
  sessionsMap.delete(sid);
  publish();
}

export async function placeCall(peerJID: string, room: string = ''): Promise<ActiveSession> {
  install();
  const peer = await newPeer();
  await peer.attachLocalMedia(room ? { audio: true } : { audio: true, video: true });
  const sid = randomSid();
  const session: ActiveSession = {
    sid,
    peerJID: bareJID(peerJID),
    direction: 'outgoing',
    room,
    peer
  };
  sessionsMap.set(sid, session);
  publish();

  peer.onLocalIce((c) => sendCandidate(session.peerJID, sid, c));
  const offer = await peer.createOffer();
  await sendIQ(session.peerJID, 'offer', sid, room, xml('sdp', {}, offer.sdp ?? ''));
  return session;
}

export function onIncomingCall(fn: IncomingCallHandler): () => void {
  install();
  incomingHandlers.add(fn);
  return () => incomingHandlers.delete(fn);
}

export async function hangup(sid: string): Promise<void> {
  const s = sessionsMap.get(sid);
  if (!s) return;
  try {
    await sendIQ(s.peerJID, 'bye', sid, s.room);
  } catch {
  }
  s.peer.close();
  sessionsMap.delete(sid);
  publish();
}

export async function hangupRoom(room: string): Promise<void> {
  for (const s of [...sessionsMap.values()]) {
    if (s.room === room) await hangup(s.sid);
  }
}

export function setRoomMembership(room: string, joined: boolean): void {
  if (joined) joinedRooms.add(room);
  else joinedRooms.delete(room);
}

export function currentDirect(): ActiveSession | null {
  for (const s of sessionsMap.values()) if (!s.room) return s;
  return null;
}

type XMLChild = ReturnType<typeof xml>;

async function sendIQ(
  toBare: string,
  type: string,
  sid: string,
  room: string,
  ...children: XMLChild[]
): Promise<void> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  const id = 'voip-' + Math.random().toString(36).slice(2, 10);
  const attrs: Record<string, string> = { xmlns: NS, type, sid };
  if (room) attrs.room = room;
  await c.send(xml('iq', { type: 'set', to: toBare, id }, xml('voip', attrs, ...children)));
}

function sendCandidate(toBare: string, sid: string, c: RTCIceCandidate): void {
  const s = sessionsMap.get(sid);
  const room = s?.room ?? '';
  const child = xml(
    'candidate',
    {
      sdpMid: c.sdpMid ?? '',
      sdpMLineIndex: c.sdpMLineIndex == null ? '' : String(c.sdpMLineIndex)
    },
    c.candidate
  );
  void sendIQ(toBare, 'candidate', sid, room, child).catch(() => {});
}

function bareJID(jid: string): string {
  const slash = jid.indexOf('/');
  return slash < 0 ? jid : jid.slice(0, slash);
}

function randomSid(): string {
  const buf = new Uint8Array(8);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('');
}

function nodeText(n: XmlNode): string {
  const children = (n as unknown as { children?: unknown[] }).children;
  if (!children) return '';
  for (const c of children) if (typeof c === 'string') return c;
  return '';
}
