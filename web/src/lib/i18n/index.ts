import { derived, writable } from 'svelte/store';
import { browser } from '$app/environment';
import { fr } from './fr';
import { en } from './en';

export type Locale = 'fr' | 'en';
export const LOCALES: { value: Locale; label: string }[] = [
  { value: 'fr', label: 'Français' },
  { value: 'en', label: 'English' }
];

const DICTS: Record<Locale, Record<string, string>> = { fr, en };
const KEY = 'krovara.locale';

function initial(): Locale {
  if (!browser) return 'fr';
  const saved = localStorage.getItem(KEY);
  if (saved === 'fr' || saved === 'en') return saved;
  return navigator.language?.toLowerCase().startsWith('en') ? 'en' : 'fr';
}

export const locale = writable<Locale>(initial());

if (browser) {
  locale.subscribe((l) => {
    localStorage.setItem(KEY, l);
    document.documentElement.lang = l;
  });
}

export const t = derived(locale, (l) => {
  const dict = DICTS[l];
  return (key: string, vars?: Record<string, string | number>): string => {
    let s = dict[key] ?? fr[key] ?? key;
    if (vars) {
      for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
    }
    return s;
  };
});
