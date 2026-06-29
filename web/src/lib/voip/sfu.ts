import { writable, type Readable } from 'svelte/store';
import { snapshot } from '$lib/stores/auth';
import { getTurnCredentials, toICEServers } from './turn';
import { wsUrl } from '$lib/config';

export type SfuPeer = {
  peerId: string;
  stream: MediaStream;
};

export type Layer = 'h' | 'm' | 'l';

type Signal = {
  type:
    | 'join'
    | 'offer'
    | 'answer'
    | 'candidate'
    | 'leave'
    | 'set-layer'
    | 'peer-joined'
    | 'peer-left';
  room?: string;
  sdp?: string;
  candidate?: string;
  sdpMid?: string | null;
  sdpMLineIndex?: number | null;
  peer_id?: string;
  rid?: Layer;
};

const CAMERA_ENCODINGS: RTCRtpEncodingParameters[] = [
  { rid: 'h', maxBitrate: 1_200_000 },
  { rid: 'm', maxBitrate: 400_000, scaleResolutionDownBy: 2 },
  { rid: 'l', maxBitrate: 150_000, scaleResolutionDownBy: 4 }
];

const _roomPeers = writable<SfuPeer[]>([]);
const _state = writable<'idle' | 'joining' | 'connected' | 'failed'>('idle');
const _localVideo = writable<MediaStream | null>(null);
const _cameraOn = writable<boolean>(false);
const _screenOn = writable<boolean>(false);
const _micOn = writable<boolean>(true);
const _peerVolumes = writable<Record<string, number>>({});

export const roomPeers: Readable<SfuPeer[]> = { subscribe: _roomPeers.subscribe };
export const peerVolumes: Readable<Record<string, number>> = { subscribe: _peerVolumes.subscribe };

export function setPeerVolume(peerId: string, vol: number): void {
  const v = Math.max(0, Math.min(1, vol));
  _peerVolumes.update((m) => ({ ...m, [peerId]: v }));
}
export const sfuState: Readable<'idle' | 'joining' | 'connected' | 'failed'> = {
  subscribe: _state.subscribe
};
export const localVideo: Readable<MediaStream | null> = { subscribe: _localVideo.subscribe };
export const cameraOn: Readable<boolean> = { subscribe: _cameraOn.subscribe };
export const screenOn: Readable<boolean> = { subscribe: _screenOn.subscribe };
export const micOn: Readable<boolean> = { subscribe: _micOn.subscribe };

type Session = {
  ws: WebSocket;
  pc: RTCPeerConnection;
  micStream: MediaStream;
  room: string;
  peersByStream: Map<string, SfuPeer>;
  makingOffer: boolean;
  isSettingRemoteAnswerPending: boolean;
  cameraTrack: MediaStreamTrack | null;
  screenTrack: MediaStreamTrack | null;
  screenAudioTrack: MediaStreamTrack | null;
};

let current: Session | null = null;
let joinInFlight = false;

export async function joinRoom(channelId: string): Promise<void> {
  if (current?.room === channelId || joinInFlight) return;
  joinInFlight = true;
  try {
    await joinRoomInner(channelId);
  } catch (err) {
    _state.set('failed');
    throw err;
  } finally {
    joinInFlight = false;
  }
}

async function joinRoomInner(channelId: string): Promise<void> {
  if (current) {
    await leaveRoom();
  }
  _state.set('joining');

  const token = snapshot().accessToken;
  if (!token) {
    throw new Error('not authenticated');
  }

  const cred = await getTurnCredentials();

  const ws = new WebSocket(buildWsUrl(token));
  await new Promise<void>((resolve, reject) => {
    ws.addEventListener('open', () => resolve(), { once: true });
    ws.addEventListener('error', () => reject(new Error('voip ws connect failed')), {
      once: true
    });
  });

  const pc = new RTCPeerConnection({ iceServers: toICEServers(cred) });
  const session: Session = {
    ws,
    pc,
    micStream: new MediaStream(),
    room: channelId,
    peersByStream: new Map(),
    makingOffer: false,
    isSettingRemoteAnswerPending: false,
    cameraTrack: null,
    screenTrack: null,
    screenAudioTrack: null
  };
  current = session;

  wireConnection(session);

  ws.addEventListener('message', (ev) => {
    let sig: Signal;
    try {
      sig = JSON.parse(typeof ev.data === 'string' ? ev.data : '');
    } catch {
      return;
    }
    void handleSignal(session, sig);
  });
  ws.addEventListener('close', () => {
    if (current !== session) return;
    current = null;
    teardown(session);
    _state.set('idle');
  });

  send(ws, { type: 'join', room: channelId });

  const mic = await navigator.mediaDevices.getUserMedia({
    audio: { echoCancellation: true, autoGainControl: true, noiseSuppression: true }
  });
  session.micStream = mic;
  mic.getAudioTracks().forEach((t) => pc.addTrack(t, mic));
}

export async function leaveRoom(): Promise<void> {
  const s = current;
  if (!s) return;
  current = null;
  try {
    send(s.ws, { type: 'leave' });
  } catch {
  }
  teardown(s);
  _state.set('idle');
}

export function requestLayer(peerId: string, rid: Layer): void {
  const s = current;
  if (!s) return;
  send(s.ws, { type: 'set-layer', peer_id: peerId, rid });
}

export function toggleMic(): boolean {
  const s = current;
  if (!s) return false;
  const tracks = s.micStream.getAudioTracks();
  const next = !tracks.every((t) => t.enabled);
  tracks.forEach((t) => (t.enabled = next));
  _micOn.set(next);
  return next;
}

export async function enableCamera(): Promise<void> {
  const s = current;
  if (!s || s.cameraTrack) return;
  const media = await navigator.mediaDevices.getUserMedia({
    video: { width: { ideal: 1280 }, height: { ideal: 720 } }
  });
  const track = media.getVideoTracks()[0];
  s.cameraTrack = track;
  track.addEventListener('ended', () => void disableCamera());
  s.pc.addTransceiver(track, { direction: 'sendonly', sendEncodings: CAMERA_ENCODINGS });
  refreshLocalVideo(s);
  _cameraOn.set(true);
}

export async function disableCamera(): Promise<void> {
  const s = current;
  if (!s || !s.cameraTrack) return;
  stopOutboundTrack(s, s.cameraTrack);
  s.cameraTrack = null;
  refreshLocalVideo(s);
  _cameraOn.set(false);
}

export async function enableScreenShare(): Promise<void> {
  const s = current;
  if (!s || s.screenTrack) return;
  const media = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: true });
  const track = media.getVideoTracks()[0];
  s.screenTrack = track;
  track.addEventListener('ended', () => void disableScreenShare());
  s.pc.addTransceiver(track, { direction: 'sendonly' });

  const audio = media.getAudioTracks()[0] ?? null;
  if (audio) {
    s.screenAudioTrack = audio;
    audio.addEventListener('ended', () => {
      if (s.screenAudioTrack) {
        stopOutboundTrack(s, s.screenAudioTrack);
        s.screenAudioTrack = null;
      }
    });
    s.pc.addTransceiver(audio, { direction: 'sendonly' });
  }

  refreshLocalVideo(s);
  _screenOn.set(true);
}

export async function disableScreenShare(): Promise<void> {
  const s = current;
  if (!s || !s.screenTrack) return;
  stopOutboundTrack(s, s.screenTrack);
  s.screenTrack = null;
  if (s.screenAudioTrack) {
    stopOutboundTrack(s, s.screenAudioTrack);
    s.screenAudioTrack = null;
  }
  refreshLocalVideo(s);
  _screenOn.set(false);
}

function wireConnection(s: Session): void {
  const { pc, ws } = s;

  pc.addEventListener('icecandidate', (ev) => {
    if (!ev.candidate) return;
    send(ws, {
      type: 'candidate',
      candidate: ev.candidate.candidate,
      sdpMid: ev.candidate.sdpMid,
      sdpMLineIndex: ev.candidate.sdpMLineIndex
    });
  });

  pc.addEventListener('track', (ev) => {
    const stream = ev.streams[0];
    if (!stream) return;
    addRemoteStream(s, stream);
    stream.addEventListener('removetrack', () => {
      if (stream.getTracks().length === 0) removeRemoteStream(s, stream.id);
    });
  });

  pc.addEventListener('connectionstatechange', () => {
    if (pc.connectionState === 'connected') {
      _state.set('connected');
    } else if (pc.connectionState === 'failed') {
      pc.restartIce();
      _state.set('joining');
    }
  });

  pc.addEventListener('negotiationneeded', () => {
    void (async () => {
      try {
        s.makingOffer = true;
        await pc.setLocalDescription();
        if (pc.localDescription) send(ws, { type: 'offer', sdp: pc.localDescription.sdp });
      } catch (err) {
        console.error('voip negotiation failed', err);
      } finally {
        s.makingOffer = false;
      }
    })();
  });
}

async function handleSignal(s: Session, sig: Signal): Promise<void> {
  const { pc, ws } = s;
  switch (sig.type) {
    case 'offer': {
      if (!sig.sdp) return;
      await pc.setRemoteDescription({ type: 'offer', sdp: sig.sdp });
      await pc.setLocalDescription();
      if (pc.localDescription) send(ws, { type: 'answer', sdp: pc.localDescription.sdp });
      break;
    }
    case 'answer': {
      if (!sig.sdp) return;
      s.isSettingRemoteAnswerPending = true;
      try {
        await pc.setRemoteDescription({ type: 'answer', sdp: sig.sdp });
      } finally {
        s.isSettingRemoteAnswerPending = false;
      }
      break;
    }
    case 'candidate': {
      if (!sig.candidate) return;
      try {
        await pc.addIceCandidate({
          candidate: sig.candidate,
          sdpMid: sig.sdpMid ?? undefined,
          sdpMLineIndex: sig.sdpMLineIndex ?? undefined
        });
      } catch {
      }
      break;
    }
    case 'peer-joined':
    case 'peer-left':
      break;
  }
}

function stopOutboundTrack(s: Session, track: MediaStreamTrack): void {
  track.stop();
  const tx = s.pc.getTransceivers().find((t) => t.sender.track === track);
  tx?.stop();
}

function refreshLocalVideo(s: Session): void {
  const tracks = [s.cameraTrack, s.screenTrack].filter(
    (t): t is MediaStreamTrack => t !== null
  );
  _localVideo.set(tracks.length > 0 ? new MediaStream(tracks) : null);
}

function addRemoteStream(s: Session, stream: MediaStream): void {
  if (s.peersByStream.has(stream.id)) return;
  s.peersByStream.set(stream.id, { peerId: stream.id, stream });
  publish(s);
}

function removeRemoteStream(s: Session, streamId: string): void {
  if (!s.peersByStream.delete(streamId)) return;
  publish(s);
}

function publish(s: Session): void {
  _roomPeers.set([...s.peersByStream.values()]);
}

function teardown(s: Session): void {
  s.micStream.getTracks().forEach((t) => t.stop());
  s.cameraTrack?.stop();
  s.screenTrack?.stop();
  s.screenAudioTrack?.stop();
  s.pc.getSenders().forEach((sender) => sender.track?.stop());
  s.pc.close();
  try {
    s.ws.close();
  } catch {
  }
  s.peersByStream.clear();
  _roomPeers.set([]);
  _peerVolumes.set({});
  _localVideo.set(null);
  _cameraOn.set(false);
  _screenOn.set(false);
  _micOn.set(true);
}

function send(ws: WebSocket, sig: Signal): void {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify(sig));
}

function buildWsUrl(token: string): string {
  const base = (import.meta.env.VITE_KROVARA_VOIP_WS as string | undefined) ?? defaultWsUrl();
  const sep = base.includes('?') ? '&' : '?';
  return `${base}${sep}token=${encodeURIComponent(token)}`;
}

function defaultWsUrl(): string {
  if (typeof window === 'undefined') return 'ws://localhost:8083/voip/ws';
  return wsUrl('/voip/ws');
}
