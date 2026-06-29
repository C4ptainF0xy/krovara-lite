import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Theme = 'dark' | 'oled' | 'light';
export type Scale = 'compact' | 'normal' | 'large';
export type Density = 'cosy' | 'compact';

export type Prefs = {
  theme: Theme;
  scale: Scale;
  density: Density;
  reduceMotion: boolean;
  developerMode: boolean;
  customCss: string;
};

const DEFAULTS: Prefs = {
  theme: 'dark',
  scale: 'normal',
  density: 'cosy',
  reduceMotion: false,
  developerMode: false,
  customCss: ''
};
const KEY = 'krovara.prefs';

const SCALE_PX: Record<Scale, string> = {
  compact: '14px',
  normal: '16px',
  large: '18px'
};

function load(): Prefs {
  if (!browser) return DEFAULTS;
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return DEFAULTS;
    return { ...DEFAULTS, ...(JSON.parse(raw) as Partial<Prefs>) };
  } catch {
    return DEFAULTS;
  }
}

export const prefs = writable<Prefs>(load());

export function apply(p: Prefs): void {
  if (!browser) return;
  const root = document.documentElement;
  root.classList.toggle('theme-oled', p.theme === 'oled');
  root.classList.toggle('theme-light', p.theme === 'light');
  root.style.colorScheme = p.theme === 'light' ? 'light' : 'dark';
  root.classList.toggle('reduce-motion', p.reduceMotion);
  root.classList.toggle('density-compact', p.density === 'compact');
  root.style.setProperty('--app-font-size', SCALE_PX[p.scale]);
  applyCustomCss(p.customCss);
}

function applyCustomCss(css: string | undefined): void {
  if (!browser) return;
  let el = document.getElementById('krovara-custom-css') as HTMLStyleElement | null;
  if (!css || !css.trim()) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement('style');
    el.id = 'krovara-custom-css';
    document.head.appendChild(el);
  }
  el.textContent = css;
}

export function setPrefs(patch: Partial<Prefs>): void {
  prefs.update((p) => {
    const next = { ...p, ...patch };
    if (browser) localStorage.setItem(KEY, JSON.stringify(next));
    apply(next);
    return next;
  });
}

if (browser) apply(load());
