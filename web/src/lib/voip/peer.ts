import { writable, type Readable } from 'svelte/store';
import { getTurnCredentials, toICEServers } from './turn';

export type PeerState =
  | 'idle'
  | 'gathering'
  | 'have-local-offer'
  | 'have-remote-offer'
  | 'connected'
  | 'failed'
  | 'closed';

export type IceCounts = { host: number; srflx: number; relay: number; other: number };

export type Peer = {
  pc: RTCPeerConnection;
  state: Readable<PeerState>;
  iceCounts: Readable<IceCounts>;
  localStream: Readable<MediaStream | null>;
  remoteStream: Readable<MediaStream | null>;

  attachLocalMedia(constraints: MediaStreamConstraints): Promise<MediaStream>;
  createOffer(): Promise<RTCSessionDescriptionInit>;
  acceptOffer(remote: RTCSessionDescriptionInit): Promise<RTCSessionDescriptionInit>;
  acceptAnswer(remote: RTCSessionDescriptionInit): Promise<void>;
  addRemoteIce(c: RTCIceCandidateInit): Promise<void>;
  onLocalIce(cb: (c: RTCIceCandidate) => void): () => void;
  close(): void;
};

export async function newPeer(): Promise<Peer> {
  const cred = await getTurnCredentials();
  const pc = new RTCPeerConnection({ iceServers: toICEServers(cred) });

  const _state = writable<PeerState>('idle');
  const _ice = writable<IceCounts>({ host: 0, srflx: 0, relay: 0, other: 0 });
  const _local = writable<MediaStream | null>(null);
  const _remote = writable<MediaStream | null>(null);

  const iceListeners = new Set<(c: RTCIceCandidate) => void>();

  pc.addEventListener('icecandidate', (ev) => {
    if (!ev.candidate) return;
    const t = classifyCandidate(ev.candidate.candidate);
    _ice.update((c) => ({ ...c, [t]: c[t] + 1 }));
    iceListeners.forEach((fn) => fn(ev.candidate as RTCIceCandidate));
  });

  pc.addEventListener('connectionstatechange', () => {
    switch (pc.connectionState) {
      case 'connected':
        _state.set('connected');
        break;
      case 'failed':
        _state.set('failed');
        break;
      case 'closed':
        _state.set('closed');
        break;
    }
  });

  pc.addEventListener('track', (ev) => {
    const stream = ev.streams[0] ?? new MediaStream([ev.track]);
    _remote.set(stream);
  });

  return {
    pc,
    state: { subscribe: _state.subscribe },
    iceCounts: { subscribe: _ice.subscribe },
    localStream: { subscribe: _local.subscribe },
    remoteStream: { subscribe: _remote.subscribe },

    async attachLocalMedia(constraints) {
      const stream = await navigator.mediaDevices.getUserMedia(constraints);
      stream.getTracks().forEach((t) => pc.addTrack(t, stream));
      _local.set(stream);
      return stream;
    },

    async createOffer() {
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      _state.set('have-local-offer');
      return offer;
    },

    async acceptOffer(remote) {
      await pc.setRemoteDescription(remote);
      _state.set('have-remote-offer');
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      return answer;
    },

    async acceptAnswer(remote) {
      await pc.setRemoteDescription(remote);
    },

    async addRemoteIce(c) {
      await pc.addIceCandidate(c);
    },

    onLocalIce(cb) {
      iceListeners.add(cb);
      return () => iceListeners.delete(cb);
    },

    close() {
      iceListeners.clear();
      pc.getSenders().forEach((s) => s.track?.stop());
      pc.close();
      _state.set('closed');
    }
  };
}

export function classifyCandidate(line: string): keyof IceCounts {
  const m = /typ\s+(\w+)/.exec(line);
  if (!m) return 'other';
  switch (m[1]) {
    case 'host':
      return 'host';
    case 'srflx':
      return 'srflx';
    case 'relay':
      return 'relay';
    default:
      return 'other';
  }
}
