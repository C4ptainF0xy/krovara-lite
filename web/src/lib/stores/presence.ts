import { writable } from 'svelte/store';
import { onPresence, type RichPresence } from '$lib/xmpp/presence';

export const presences = writable<Record<string, RichPresence | null>>({});

let subscribed = false;
export function startPresenceIngest(): void {
  if (subscribed) return;
  subscribed = true;
  onPresence((bareJID, p) => {
    presences.update((m) => ({ ...m, [bareJID]: p }));
  });
}

export function userIDFromJID(jid: string): string {
  return jid.split('@')[0] ?? jid;
}
