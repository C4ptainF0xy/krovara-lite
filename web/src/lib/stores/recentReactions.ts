import { writable } from 'svelte/store';

const KEY = 'krovara:recent-reactions';
const MAX = 8;

function load(): string[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const raw = localStorage.getItem(KEY);
    const arr = raw ? JSON.parse(raw) : [];
    return Array.isArray(arr) ? arr.filter((x): x is string => typeof x === 'string').slice(0, MAX) : [];
  } catch {
    return [];
  }
}

export const recentReactions = writable<string[]>(load());

export function rememberReaction(emoji: string): void {
  recentReactions.update((cur) => {
    const next = [emoji, ...cur.filter((e) => e !== emoji)].slice(0, MAX);
    if (typeof localStorage !== 'undefined') {
      try {
        localStorage.setItem(KEY, JSON.stringify(next));
      } catch {
      }
    }
    return next;
  });
}
