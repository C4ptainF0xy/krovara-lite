export type MediaLink =
  | { kind: 'image'; url: string }
  | { kind: 'video'; url: string }
  | { kind: 'youtube'; url: string; id: string }
  | { kind: 'twitch-clip'; url: string; slug: string }
  | { kind: 'twitch-channel'; url: string; channel: string };

const IMAGE_RE = /\.(gif|png|jpe?g|webp|avif|bmp|svg|apng)(\?.*)?$/i;
const VIDEO_RE = /\.(mp4|webm|mov|m4v)(\?.*)?$/i;
const URL_RE = /https?:\/\/[^\s<]+/g;

function youtubeId(u: URL): string | null {
  const h = u.hostname.replace(/^www\./, '');
  if (h === 'youtu.be') return u.pathname.slice(1).split('/')[0] || null;
  if (h === 'youtube.com' || h === 'm.youtube.com') {
    if (u.pathname === '/watch') return u.searchParams.get('v');
    if (u.pathname.startsWith('/shorts/')) return u.pathname.split('/')[2] || null;
    if (u.pathname.startsWith('/embed/')) return u.pathname.split('/')[2] || null;
  }
  return null;
}

function classify(raw: string): MediaLink | null {
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  const path = u.pathname;
  const yt = youtubeId(u);
  if (yt) return { kind: 'youtube', url: raw, id: yt };

  const host = u.hostname.replace(/^www\./, '');
  if (host === 'clips.twitch.tv') {
    const slug = path.slice(1).split('/')[0];
    if (slug) return { kind: 'twitch-clip', url: raw, slug };
  }
  if (host === 'twitch.tv' || host === 'm.twitch.tv') {
    const seg = path.slice(1).split('/');
    if (seg[0] === 'clip' && seg[1]) return { kind: 'twitch-clip', url: raw, slug: seg[1] };
    if (seg[0] && !['videos', 'directory'].includes(seg[0]))
      return { kind: 'twitch-channel', url: raw, channel: seg[0] };
  }

  if (IMAGE_RE.test(path)) return { kind: 'image', url: raw };
  if (VIDEO_RE.test(path)) return { kind: 'video', url: raw };
  return null;
}

export function extractMediaLinks(body: string | null | undefined, max = 4): MediaLink[] {
  if (!body) return [];
  const out: MediaLink[] = [];
  const seen = new Set<string>();
  for (const m of body.matchAll(URL_RE)) {
    const link = classify(m[0]);
    if (link && !seen.has(link.url)) {
      seen.add(link.url);
      out.push(link);
      if (out.length >= max) break;
    }
  }
  return out;
}
