import { api } from '$lib/api';

export type TurnCredential = {
  uris: string[];
  username: string;
  credential: string;
  ttl_seconds: number;
  expires_at: string;
};

let cached: { cred: TurnCredential; fetchedAt: number } | null = null;
const REFRESH_BEFORE_MS = 5 * 60 * 1000;

const stunOnly: TurnCredential = {
  uris: [],
  username: '',
  credential: '',
  ttl_seconds: 0,
  expires_at: new Date(0).toISOString()
};

export async function getTurnCredentials(): Promise<TurnCredential> {
  if (cached) {
    const exp = Date.parse(cached.cred.expires_at);
    if (exp - Date.now() > REFRESH_BEFORE_MS) return cached.cred;
  }
  const cred = await api<TurnCredential>('/api/voip/turn-credentials').catch(() => null);
  if (!cred) return stunOnly;
  cached = { cred, fetchedAt: Date.now() };
  return cred;
}

export function toICEServers(c: TurnCredential): RTCIceServer[] {
  if (c.uris.length === 0) {
    return [{ urls: 'stun:stun.l.google.com:19302' }];
  }
  return [
    {
      urls: c.uris,
      username: c.username,
      credential: c.credential
    }
  ];
}
