import { api } from '$lib/api';

const cache = new Map<string, string | null>();
let pending = new Set<string>();
let timer: ReturnType<typeof setTimeout> | undefined;
let waiters: Array<() => void> = [];

export function verdict(url: string): string | null | undefined {
  return cache.get(url);
}

export async function checkUrls(urls: string[]): Promise<void> {
  let queued = false;
  for (const u of urls) {
    if (!cache.has(u) && !pending.has(u)) {
      pending.add(u);
      queued = true;
    }
  }
  if (!queued && pending.size === 0) return;
  await new Promise<void>((resolve) => {
    waiters.push(resolve);
    clearTimeout(timer);
    timer = setTimeout(flush, 150);
  });
}

async function flush(): Promise<void> {
  const urls = [...pending];
  pending = new Set();
  const resolves = waiters;
  waiters = [];
  try {
    for (let i = 0; i < urls.length; i += 50) {
      const chunk = urls.slice(i, i + 50);
      const res = await api<{ malicious: { url: string; threat: string }[] }>('/api/links/check', {
        method: 'POST',
        body: { urls: chunk }
      });
      const bad = new Map(res.malicious.map((m) => [m.url, m.threat]));
      for (const u of chunk) cache.set(u, bad.get(u) ?? null);
    }
  } catch {
    for (const u of urls) if (!cache.has(u)) cache.set(u, null);
  } finally {
    for (const r of resolves) r();
  }
}
