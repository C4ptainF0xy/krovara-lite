import { auth, clearSession, hasSession, setSession, snapshot, type AuthUser } from './stores/auth';
import { apiUrl } from './config';

export class ApiError extends Error {
  constructor(public status: number, public body: unknown) {
    super(typeof body === 'object' && body && 'error' in body ? String((body as { error: unknown }).error) : `HTTP ${status}`);
  }
}

type Init = Omit<RequestInit, 'body'> & { body?: unknown };

export async function api<T = unknown>(path: string, init: Init = {}): Promise<T> {
  const res = await call(path, init, true);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? safeJSON(text) : null;
  if (!res.ok) throw new ApiError(res.status, data);
  return data as T;
}

export async function loadMe(): Promise<void> {
  const me = await api<AuthUser>('/api/me');
  auth.update((s) => ({ ...s, user: me }));
}

export async function authedObjectURL(path: string): Promise<string> {
  const res = await call(path, {}, true);
  if (!res.ok) throw new ApiError(res.status, await res.text());
  return URL.createObjectURL(await res.blob());
}

export async function authedDataURL(path: string): Promise<string> {
  const res = await call(path, {}, true);
  if (!res.ok) throw new ApiError(res.status, await res.text());
  const blob = await res.blob();
  return await new Promise<string>((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(fr.result as string);
    fr.onerror = () => reject(fr.error);
    fr.readAsDataURL(blob);
  });
}

async function call(path: string, init: Init, allowRefresh: boolean): Promise<Response> {
  if (allowRefresh && !snapshot().accessToken && hasSession()) {
    await tryRefresh();
  }
  const headers = new Headers(init.headers ?? {});
  const access = snapshot().accessToken;
  if (access) headers.set('Authorization', `Bearer ${access}`);
  let body: BodyInit | undefined;
  if (init.body !== undefined) {
    if (init.body instanceof FormData || typeof init.body === 'string') {
      body = init.body as BodyInit;
    } else {
      headers.set('Content-Type', 'application/json');
      body = JSON.stringify(init.body);
    }
  }
  const res = await fetch(apiUrl(path), { ...init, headers, body, credentials: 'include' });
  if (res.status !== 401 || !allowRefresh) return res;

  const refreshed = await tryRefresh();
  if (!refreshed) {
    clearSession();
    return res;
  }
  return call(path, init, false);
}

let inflight: Promise<boolean> | null = null;

function tryRefresh(): Promise<boolean> {
  if (inflight) return inflight;
  if (!hasSession()) return Promise.resolve(false);

  inflight = (async () => {
    try {
      const res = await fetch(apiUrl('/api/auth/refresh'), { method: 'POST', credentials: 'include' });
      if (!res.ok) return false;
      const data = (await res.json()) as {
        access_token: string;
        access_expires_at: string;
      };
      setSession({
        accessToken: data.access_token,
        accessExpiresAt: data.access_expires_at
      });
      return true;
    } catch {
      return false;
    } finally {
      inflight = null;
    }
  })();
  return inflight;
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
