import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Layout = {
  channelSidebarWidth: number;
  membersPanelWidth: number;
  membersPanelOpen: boolean;
};

export const CHANNEL_SIDEBAR_MIN = 180;
export const CHANNEL_SIDEBAR_MAX = 420;
export const MEMBERS_PANEL_MIN = 180;
export const MEMBERS_PANEL_MAX = 420;

const DEFAULTS: Layout = {
  channelSidebarWidth: 240,
  membersPanelWidth: 240,
  membersPanelOpen: true
};
const KEY = 'krovara.layout';

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

function load(): Layout {
  if (!browser) return DEFAULTS;
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return DEFAULTS;
    const parsed = { ...DEFAULTS, ...(JSON.parse(raw) as Partial<Layout>) };
    parsed.channelSidebarWidth = clamp(
      parsed.channelSidebarWidth,
      CHANNEL_SIDEBAR_MIN,
      CHANNEL_SIDEBAR_MAX
    );
    parsed.membersPanelWidth = clamp(parsed.membersPanelWidth, MEMBERS_PANEL_MIN, MEMBERS_PANEL_MAX);
    return parsed;
  } catch {
    return DEFAULTS;
  }
}

export const layout = writable<Layout>(load());

function persist(l: Layout): void {
  if (browser) localStorage.setItem(KEY, JSON.stringify(l));
}

export function setChannelSidebarWidth(px: number): void {
  layout.update((l) => {
    const next = { ...l, channelSidebarWidth: clamp(px, CHANNEL_SIDEBAR_MIN, CHANNEL_SIDEBAR_MAX) };
    persist(next);
    return next;
  });
}

export function setMembersPanelWidth(px: number): void {
  layout.update((l) => {
    const next = { ...l, membersPanelWidth: clamp(px, MEMBERS_PANEL_MIN, MEMBERS_PANEL_MAX) };
    persist(next);
    return next;
  });
}

export function toggleMembersPanel(): void {
  layout.update((l) => {
    const next = { ...l, membersPanelOpen: !l.membersPanelOpen };
    persist(next);
    return next;
  });
}

export const focusMode = writable<boolean>(false);

export function toggleFocusMode(): void {
  focusMode.update((v) => !v);
}
