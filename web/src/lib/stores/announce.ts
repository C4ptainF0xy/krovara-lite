import { writable } from 'svelte/store';

export type Announcement = { text: string; assertive: boolean; n: number };

export const announcement = writable<Announcement>({ text: '', assertive: false, n: 0 });

let counter = 0;

export function announce(text: string, assertive = false): void {
  if (!text) return;
  announcement.set({ text, assertive, n: ++counter });
}
