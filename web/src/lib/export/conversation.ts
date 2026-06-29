import type { ChannelMessage } from '$lib/stores/messages';

export type ExportEntry = {
  id: string;
  authorId: string;
  author: string;
  body: string;
  at: Date;
  edited: boolean;
  avatar?: string | null;
};

export type ExportMeta = {
  channelName: string;
  spaceName?: string;
  exportedAt: Date;
};

export type ExportFormat = 'html' | 'txt' | 'json';

export function authorIdOf(m: ChannelMessage): string {
  return m.fromResource || m.from;
}

function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function initials(name: string): string {
  return name.trim().slice(0, 2).toUpperCase() || '?';
}

function fmtTime(d: Date): string {
  return d.toLocaleString('fr-FR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  });
}

export function toJSON(entries: ExportEntry[], meta: ExportMeta): string {
  return JSON.stringify(
    {
      channel: meta.channelName,
      space: meta.spaceName,
      exported_at: meta.exportedAt.toISOString(),
      message_count: entries.length,
      messages: entries.map((e) => ({
        id: e.id,
        author: e.author,
        author_id: e.authorId,
        body: e.body,
        at: e.at.toISOString(),
        edited: e.edited
      }))
    },
    null,
    2
  );
}

export function toText(entries: ExportEntry[], meta: ExportMeta): string {
  const head =
    `# ${meta.channelName}${meta.spaceName ? ` · ${meta.spaceName}` : ''}\n` +
    `# ${entries.length} messages · exporté le ${fmtTime(meta.exportedAt)}\n\n`;
  return (
    head +
    entries
      .map((e) => `[${fmtTime(e.at)}] ${e.author}${e.edited ? ' (modifié)' : ''}:\n${e.body}`)
      .join('\n\n') +
    '\n'
  );
}

const HTML_CSS = `
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;background:linear-gradient(135deg,#0F0F14,#1A1A22);background-attachment:fixed;color:#E7E5EE;font-family:'Onest',system-ui,-apple-system,sans-serif;font-size:15px;line-height:1.5}
.wrap{max-width:760px;margin:0 auto;padding:32px 20px 64px}
.hd{border-bottom:1px solid #2A2A35;padding-bottom:16px;margin-bottom:8px}
.hd h1{font-size:20px;font-weight:600;margin:0 0 4px}
.hd p{margin:0;color:#9A98A8;font-size:13px}
.grp{display:flex;gap:12px;margin-top:18px}
.av{width:40px;height:40px;border-radius:50%;object-fit:cover;flex:0 0 auto;background:#1F1F29}
.av.init{display:grid;place-items:center;background:#756E92;color:#fff;font-weight:600;font-size:14px}
.body{min-width:0;flex:1}
.meta{display:flex;gap:8px;align-items:baseline;flex-wrap:wrap}
.auth{font-weight:600;color:#F2F1F6}
.time{color:#9A98A8;font-size:12px}
.msg{margin-top:2px;white-space:pre-wrap;word-wrap:break-word;overflow-wrap:anywhere}
.cont{margin-left:52px}
.edit{color:#9A98A8;font-size:12px}
`;

export function toHTML(entries: ExportEntry[], meta: ExportMeta): string {
  const items: string[] = [];
  let lastAuthor = '';
  let lastAt = 0;
  for (const e of entries) {
    const newGroup = e.authorId !== lastAuthor || e.at.getTime() - lastAt > 5 * 60 * 1000;
    lastAuthor = e.authorId;
    lastAt = e.at.getTime();
    const body =
      `<div class="msg">${esc(e.body).replace(/\n/g, '<br>')}` +
      `${e.edited ? ' <span class="edit">(modifié)</span>' : ''}</div>`;
    if (newGroup) {
      const av = e.avatar
        ? `<img class="av" src="${e.avatar}" alt="">`
        : `<div class="av init">${esc(initials(e.author))}</div>`;
      items.push(
        `<div class="grp">${av}<div class="body">` +
          `<div class="meta"><span class="auth">${esc(e.author)}</span>` +
          `<span class="time">${esc(fmtTime(e.at))}</span></div>${body}</div></div>`
      );
    } else {
      items.push(`<div class="cont">${body}</div>`);
    }
  }
  return (
    `<!doctype html>\n<html lang="fr"><head><meta charset="utf-8">` +
    `<meta name="viewport" content="width=device-width,initial-scale=1">` +
    `<title>${esc(meta.channelName)} · export Krovara</title>` +
    `<style>${HTML_CSS}</style></head><body><div class="wrap">` +
    `<div class="hd"><h1># ${esc(meta.channelName)}</h1>` +
    `<p>${meta.spaceName ? esc(meta.spaceName) + ' · ' : ''}${entries.length} messages · ` +
    `exporté le ${esc(fmtTime(meta.exportedAt))}</p></div>` +
    `${items.join('')}</div></body></html>`
  );
}

export function render(format: ExportFormat, entries: ExportEntry[], meta: ExportMeta): string {
  if (format === 'json') return toJSON(entries, meta);
  if (format === 'txt') return toText(entries, meta);
  return toHTML(entries, meta);
}

export function mimeFor(format: ExportFormat): string {
  return format === 'json'
    ? 'application/json'
    : format === 'txt'
      ? 'text/plain;charset=utf-8'
      : 'text/html;charset=utf-8';
}

export function downloadFile(name: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
