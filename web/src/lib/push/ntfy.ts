import { api } from '$lib/api';

export type Device = {
  id: string;
  name: string;
  ntfy_topic: string;
  created_at: string;
};

let activeES: EventSource | null = null;

export async function registerThisBrowser(): Promise<Device> {
  return api<Device>('/api/me/devices', {
    method: 'POST',
    body: { name: navigator.userAgent.slice(0, 60) || 'browser' }
  });
}

export function subscribe(topic: string): () => void {
  if (activeES && activeES.url.includes(topic)) {
    return () => {};
  }
  if (activeES) {
    activeES.close();
    activeES = null;
  }
  const url = `/ntfy/${encodeURIComponent(topic)}/sse`;
  const es = new EventSource(url);
  activeES = es;
  es.onmessage = (ev) => {
    if (!ev.data) return;
    try {
      const msg = JSON.parse(ev.data) as { title?: string; message?: string };
      if (msg.message) showNotification(msg.title || 'Krovara', msg.message);
    } catch {
    }
  };
  es.onerror = () => {
  };
  return () => {
    es.close();
    if (activeES === es) activeES = null;
  };
}

async function showNotification(title: string, body: string): Promise<void> {
  if (typeof Notification === 'undefined') return;
  if (Notification.permission === 'default') {
    try {
      await Notification.requestPermission();
    } catch {
      return;
    }
  }
  if (Notification.permission !== 'granted') return;
  new Notification(title, { body });
}

export async function requestPermission(): Promise<NotificationPermission> {
  if (typeof Notification === 'undefined') return 'denied';
  if (Notification.permission !== 'default') return Notification.permission;
  try {
    return await Notification.requestPermission();
  } catch {
    return 'denied';
  }
}
