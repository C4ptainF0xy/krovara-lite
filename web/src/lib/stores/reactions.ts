import { writable, get } from 'svelte/store';
import { onReaction } from '$lib/xmpp/client';

type ByEmoji = Record<string, string[]>;
export const reactions = writable<Record<string, ByEmoji>>({});

let started = false;
export function startReactionIngest(): void {
  if (started) return;
  started = true;
  onReaction(({ targetId, userId, emojis }) => {
    reactions.update((map) => {
      const cur: ByEmoji = { ...(map[targetId] ?? {}) };
      for (const e of Object.keys(cur)) {
        const next = cur[e].filter((u) => u !== userId);
        if (next.length) cur[e] = next;
        else delete cur[e];
      }
      for (const e of emojis) {
        cur[e] = [...(cur[e] ?? []), userId];
      }
      return { ...map, [targetId]: cur };
    });
  });
}

export type ReactionPill = { emoji: string; count: number; mine: boolean; users: string[] };

export function pillsFor(byEmoji: ByEmoji | undefined, selfId: string): ReactionPill[] {
  if (!byEmoji) return [];
  return Object.entries(byEmoji).map(([emoji, users]) => ({
    emoji,
    count: users.length,
    mine: users.includes(selfId),
    users
  }));
}

export function myEmojis(targetId: string, selfId: string): string[] {
  const byEmoji = get(reactions)[targetId];
  if (!byEmoji) return [];
  return Object.entries(byEmoji)
    .filter(([, users]) => users.includes(selfId))
    .map(([emoji]) => emoji);
}
