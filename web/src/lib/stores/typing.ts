import { writable } from 'svelte/store';
import { onChatState, onMessage } from '$lib/xmpp/client';

export const typing = writable<Record<string, string[]>>({});

const TIMEOUT_MS = 7000;
const timers = new Map<string, ReturnType<typeof setTimeout>>();

function key(channelId: string, userId: string) {
  return `${channelId}|${userId}`;
}

function setTyping(channelId: string, userId: string, on: boolean) {
  const k = key(channelId, userId);
  const t = timers.get(k);
  if (t) {
    clearTimeout(t);
    timers.delete(k);
  }
  typing.update((map) => {
    const cur = map[channelId] ?? [];
    const has = cur.includes(userId);
    if (on && !has) return { ...map, [channelId]: [...cur, userId] };
    if (!on && has) return { ...map, [channelId]: cur.filter((u) => u !== userId) };
    return map;
  });
  if (on) {
    timers.set(
      k,
      setTimeout(() => setTyping(channelId, userId, false), TIMEOUT_MS)
    );
  }
}

let started = false;
export function startTypingIngest(): void {
  if (started) return;
  started = true;
  onChatState(({ channelId, userId, state }) => {
    setTyping(channelId, userId, state === 'composing');
  });
  onMessage((m) => {
    const uid = m.fromResource || m.from;
    if (uid) setTyping(m.channelId, uid, false);
  });
}
