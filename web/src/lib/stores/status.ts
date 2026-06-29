import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { onXmppPresence } from '$lib/xmpp/client';
import { broadcastStatus, broadcastInvisible } from '$lib/xmpp/muc';

export type Availability = 'online' | 'idle' | 'dnd' | 'offline';
export type SelfStatus = 'online' | 'idle' | 'dnd' | 'invisible';

export type StatusMeta = {
  label: string;
  dot: string;
  text: string;
};

export const STATUS_META: Record<Availability | 'invisible', StatusMeta> = {
  online: { label: 'En ligne', dot: 'bg-success', text: 'text-success' },
  idle: { label: 'Absent', dot: 'bg-warning', text: 'text-warning' },
  dnd: { label: 'Ne pas déranger', dot: 'bg-danger', text: 'text-danger' },
  invisible: { label: 'Invisible', dot: 'bg-muted/60', text: 'text-muted' },
  offline: { label: 'Hors ligne', dot: 'bg-muted/50', text: 'text-muted' }
};

function showToAvailability(show?: string): Availability {
  switch (show) {
    case 'away':
    case 'xa':
      return 'idle';
    case 'dnd':
      return 'dnd';
    default:
      return 'online';
  }
}

export type PeerStatus = { availability: Availability; text?: string };

export const peerStatus = writable<Record<string, PeerStatus>>({});

const STATUS_KEY = 'krovara.status';
const TEXT_KEY = 'krovara.status.text';

function loadSelf(): SelfStatus {
  if (!browser) return 'online';
  const v = localStorage.getItem(STATUS_KEY);
  return v === 'idle' || v === 'dnd' || v === 'invisible' ? v : 'online';
}

export const selfStatus = writable<SelfStatus>(loadSelf());
export const customStatus = writable<string>(browser ? localStorage.getItem(TEXT_KEY) ?? '' : '');

const EMOJI_KEY = 'krovara.status.emoji';
const EXPIRES_KEY = 'krovara.status.expires';
export const customStatusEmoji = writable<string>(browser ? localStorage.getItem(EMOJI_KEY) ?? '' : '');
export const customStatusExpiresAt = writable<number>(
  browser ? Number(localStorage.getItem(EXPIRES_KEY) ?? '0') : 0
);

let expiryTimer: ReturnType<typeof setTimeout> | null = null;
function scheduleExpiry(): void {
  if (!browser) return;
  if (expiryTimer) clearTimeout(expiryTimer);
  const at = get(customStatusExpiresAt);
  if (!at) return;
  const ms = at - Date.now();
  if (ms <= 0) {
    clearCustomStatus();
    return;
  }
  expiryTimer = setTimeout(clearCustomStatus, ms);
}

const AUTOIDLE_KEY = 'krovara.autoidle';
const ACTIVITY_KEY = 'krovara.activity';
const AUTODND_GAME_KEY = 'krovara.autodnd.game';

export const autoIdle = writable<boolean>(browser ? localStorage.getItem(AUTOIDLE_KEY) !== '0' : true);
export const shareActivity = writable<boolean>(browser ? localStorage.getItem(ACTIVITY_KEY) !== '0' : true);
export const autoDndGame = writable<boolean>(browser ? localStorage.getItem(AUTODND_GAME_KEY) === '1' : false);

export function setAutoIdle(v: boolean): void {
  autoIdle.set(v);
  if (browser) localStorage.setItem(AUTOIDLE_KEY, v ? '1' : '0');
  if (!v && afk) {
    afk = false;
    broadcast();
  }
}

export function setShareActivity(v: boolean): void {
  shareActivity.set(v);
  if (browser) localStorage.setItem(ACTIVITY_KEY, v ? '1' : '0');
}

export function setAutoDndGame(v: boolean): void {
  autoDndGame.set(v);
  if (browser) localStorage.setItem(AUTODND_GAME_KEY, v ? '1' : '0');
  broadcast();
}

let gaming = false;

export function setGaming(active: boolean): void {
  if (gaming === active) return;
  gaming = active;
  broadcast();
}

export const selfAvailability = writable<Availability | 'invisible'>(loadSelf());

let afk = false;

function composedStatusText(): string | undefined {
  const emoji = get(customStatusEmoji).trim();
  const txt = get(customStatus).trim();
  const joined = [emoji, txt].filter(Boolean).join(' ');
  return joined || undefined;
}

function broadcast(): void {
  const chosen = get(selfStatus);
  const text = composedStatusText();
  if (chosen === 'invisible') {
    broadcastInvisible();
    selfAvailability.set('invisible');
    return;
  }
  let effective: SelfStatus = chosen;
  if (chosen === 'online') {
    if (gaming && get(autoDndGame)) effective = 'dnd';
    else if (afk) effective = 'idle';
  }
  const show = effective === 'idle' ? 'away' : effective === 'dnd' ? 'dnd' : undefined;
  broadcastStatus(show, text);
  selfAvailability.set(effective);
}

export function setStatus(s: SelfStatus): void {
  selfStatus.set(s);
  if (browser) localStorage.setItem(STATUS_KEY, s);
  broadcast();
}

export function setCustomStatus(text: string, emoji = '', expiresInMinutes = 0): void {
  customStatus.set(text);
  customStatusEmoji.set(emoji);
  const at = expiresInMinutes > 0 ? Date.now() + expiresInMinutes * 60_000 : 0;
  customStatusExpiresAt.set(at);
  if (browser) {
    localStorage.setItem(TEXT_KEY, text);
    localStorage.setItem(EMOJI_KEY, emoji);
    localStorage.setItem(EXPIRES_KEY, String(at));
  }
  scheduleExpiry();
  broadcast();
}

export function clearCustomStatus(): void {
  customStatus.set('');
  customStatusEmoji.set('');
  customStatusExpiresAt.set(0);
  if (browser) {
    localStorage.removeItem(TEXT_KEY);
    localStorage.removeItem(EMOJI_KEY);
    localStorage.removeItem(EXPIRES_KEY);
  }
  if (expiryTimer) clearTimeout(expiryTimer);
  broadcast();
}

export function publishStatus(): void {
  broadcast();
}

const IDLE_MS = 5 * 60 * 1000;
let idleTimer: ReturnType<typeof setTimeout> | null = null;

function markActive(): void {
  if (afk) {
    afk = false;
    broadcast();
  }
  if (idleTimer) clearTimeout(idleTimer);
  idleTimer = setTimeout(() => {
    if (!get(autoIdle)) return;
    afk = true;
    broadcast();
  }, IDLE_MS);
}

let started = false;

export function startStatusIngest(): void {
  if (started) return;
  started = true;

  const roomsByUser = new Map<string, Map<string, { show?: string; text?: string }>>();
  onXmppPresence(({ userId, room, available, show, statusText }) => {
    let rooms = roomsByUser.get(userId);
    if (available) {
      if (!rooms) {
        rooms = new Map();
        roomsByUser.set(userId, rooms);
      }
      rooms.set(room, { show, text: statusText });
    } else if (rooms) {
      rooms.delete(room);
      if (rooms.size === 0) roomsByUser.delete(userId);
    }
    const present = roomsByUser.get(userId);
    const RANK: Record<Availability, number> = { online: 0, idle: 1, dnd: 2, offline: 3 };
    let availability: Availability = 'offline';
    let text: string | undefined;
    if (present && present.size > 0) {
      let best: Availability = 'offline';
      for (const v of present.values()) {
        const a = showToAvailability(v.show);
        if (RANK[a] < RANK[best]) best = a;
        if (v.text) text = v.text;
      }
      availability = best === 'offline' ? 'online' : best;
    }
    peerStatus.update((m) => ({ ...m, [userId]: { availability, text } }));
  });

  if (browser) {
    for (const ev of ['mousemove', 'keydown', 'pointerdown', 'visibilitychange'] as const) {
      window.addEventListener(ev, markActive, { passive: true });
    }
    markActive();
    scheduleExpiry();
  }
}
