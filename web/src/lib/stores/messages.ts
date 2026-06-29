import { writable } from 'svelte/store';
import { onMessage, type IncomingMessage } from '$lib/xmpp/client';

export type ChannelMessage = IncomingMessage;

const MAX_PER_CHANNEL = 50;

type ByChannel = Record<string, ChannelMessage[]>;

export const messages = writable<ByChannel>({});

const expanded = new Set<string>();
export function expandChannelHistory(channelId: string): void {
  expanded.add(channelId);
}

let subscribed = false;
export function startMessageIngest(): void {
  if (subscribed) return;
  subscribed = true;
  onMessage((m) => {
    if (m.kind === 'dm') return;
    messages.update((map) => {
      const cur = map[m.channelId] ?? [];

      if (m.replaceId) {
        const idx = cur.findIndex(
          (x) =>
            (x.originId === m.replaceId || x.id === m.replaceId) &&
            x.from === m.from &&
            x.fromResource === m.fromResource
        );
        if (idx < 0) return map;
        const next = [...cur];
        next[idx] = { ...next[idx], body: m.body, edited: true };
        return { ...map, [m.channelId]: next };
      }

      if (cur.some((x) => x.id === m.id)) return map;
      const next = [...cur, m].sort((a, b) => a.at.getTime() - b.at.getTime());
      if (!expanded.has(m.channelId) && !m.fromHistory && next.length > MAX_PER_CHANNEL) {
        next.splice(0, next.length - MAX_PER_CHANNEL);
      }
      return { ...map, [m.channelId]: next };
    });
  });
}

export function removeMessage(channelId: string, id: string): void {
  messages.update((map) => {
    const cur = map[channelId];
    if (!cur) return map;
    return { ...map, [channelId]: cur.filter((m) => m.id !== id) };
  });
}

export function clearChannel(channelId: string): void {
  expanded.delete(channelId);
  messages.update((map) => {
    if (!(channelId in map)) return map;
    const { [channelId]: _, ...rest } = map;
    void _;
    return rest;
  });
}
