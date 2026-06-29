export const API_ORIGIN: string = (
  (import.meta.env.VITE_KROVARA_API_ORIGIN as string | undefined) ?? ''
).replace(/\/$/, '');

export function apiUrl(path: string): string {
  return API_ORIGIN + path;
}

export function wsUrl(path: string): string {
  if (API_ORIGIN) {
    const u = new URL(API_ORIGIN);
    return `${u.protocol === 'https:' ? 'wss:' : 'ws:'}//${u.host}${path}`;
  }
  const { protocol, host } = window.location;
  return `${protocol === 'https:' ? 'wss:' : 'ws:'}//${host}${path}`;
}
