import { writable } from 'svelte/store';

export type ForwardDraft = { channelId: string; text: string; nonce: number } | null;

export const forwardDraft = writable<ForwardDraft>(null);

let nonce = 0;
export function setForward(channelId: string, text: string): void {
  forwardDraft.set({ channelId, text, nonce: ++nonce });
}

export function clearForward(): void {
  forwardDraft.set(null);
}
