import { writable } from 'svelte/store';
import { onReadMarker } from '$lib/xmpp/client';

export const readMarkers = writable<Record<string, Record<string, string>>>({});

let started = false;
export function startReadIngest(): void {
  if (started) return;
  started = true;
  onReadMarker(({ channelId, userId, messageId }) => {
    readMarkers.update((map) => ({
      ...map,
      [channelId]: { ...(map[channelId] ?? {}), [userId]: messageId }
    }));
  });
}

export function seenBy(
  byChannel: Record<string, string> | undefined,
  messageId: string,
  selfId: string,
  authorId?: string
): string[] {
  if (!byChannel) return [];
  return Object.entries(byChannel)
    .filter(([uid, mid]) => mid === messageId && uid !== selfId && uid !== authorId)
    .map(([uid]) => uid);
}
