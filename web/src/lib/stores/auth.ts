import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

export type AuthUser = {
  id: string;
  username: string;
  email: string;
  display_name?: string | null;
  status?: string | null;
  is_admin?: boolean;
  tier?: 'free' | 'plus';
  totp_enabled?: boolean;
  avatar_key?: string | null;
  email_verified?: boolean;
};

export type AuthState = {
  accessToken: string | null;
  accessExpiresAt: number | null;
  user: AuthUser | null;
};

const initial: AuthState = { accessToken: null, accessExpiresAt: null, user: null };

export const auth = writable<AuthState>(initial);

const SESSION_FLAG = 'krovara.session';

function setSessionFlag(on: boolean): void {
  if (!browser) return;
  if (on) localStorage.setItem(SESSION_FLAG, '1');
  else localStorage.removeItem(SESSION_FLAG);
  localStorage.removeItem('krovara.refresh');
}

export function hasSession(): boolean {
  return browser && localStorage.getItem(SESSION_FLAG) === '1';
}

export function setSession(args: {
  accessToken: string;
  accessExpiresAt: string | number;
  refreshToken?: string;
  user?: AuthUser | null;
}): void {
  const exp =
    typeof args.accessExpiresAt === 'string'
      ? Date.parse(args.accessExpiresAt)
      : args.accessExpiresAt;
  auth.update((s) => ({
    ...s,
    accessToken: args.accessToken,
    accessExpiresAt: exp,
    user: args.user ?? s.user
  }));
  setSessionFlag(true);
}

export function clearSession(): void {
  auth.set(initial);
  setSessionFlag(false);
}

export function snapshot(): AuthState {
  return get(auth);
}
