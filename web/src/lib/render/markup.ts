const D = String.fromCharCode(0);

const MENTION_SELF = 'rounded bg-warning/20 px-1 font-medium text-warning';
const MENTION_OTHER = 'rounded bg-primary/15 px-1 font-medium text-accent';
const MENTION_ROLE = 'rounded bg-primary/10 px-1 font-medium text-accent';
const HEX_COLOR = /^#[0-9a-fA-F]{3,8}$/;

const svgIcon = (inner: string) =>
  `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${inner}</svg>`;
const COPY_ICON =
  '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';

const CALLOUTS: Record<string, { cls: string; label: string; icon: string }> = {
  NOTE: {
    cls: 'callout-note',
    label: 'Note',
    icon: svgIcon('<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>')
  },
  TIP: {
    cls: 'callout-tip',
    label: 'Astuce',
    icon: svgIcon(
      '<path d="M9 18h6"/><path d="M10 22h4"/><path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/>'
    )
  },
  IMPORTANT: {
    cls: 'callout-important',
    label: 'Important',
    icon: svgIcon(
      '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/><path d="M12 7v4"/><path d="M12 15h.01"/>'
    )
  },
  WARNING: {
    cls: 'callout-warning',
    label: 'Attention',
    icon: svgIcon(
      '<path d="m21.73 18-8-14a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>'
    )
  },
  DANGER: {
    cls: 'callout-danger',
    label: 'Danger',
    icon: svgIcon('<path d="M15.3 2H8.7L2 8.7v6.6L8.7 22h6.6L22 15.3V8.7z"/><path d="M12 8v4"/><path d="M12 16h.01"/>')
  },
  SPOILER: {
    cls: 'callout-spoiler',
    label: 'Spoiler',
    icon: svgIcon('<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7"/><circle cx="12" cy="12" r="3"/>')
  }
};
CALLOUTS.CAUTION = CALLOUTS.DANGER;

const TS_LOCALE = 'fr-FR';

function tsDate(unix: number): Date | null {
  const d = new Date(unix * 1000);
  return Number.isNaN(d.getTime()) ? null : d;
}

const RTF = new Intl.RelativeTimeFormat(TS_LOCALE, { numeric: 'auto' });
const REL_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1]
];
const TS_STYLES: Record<string, Intl.DateTimeFormatOptions> = {
  t: { hour: '2-digit', minute: '2-digit' },
  T: { hour: '2-digit', minute: '2-digit', second: '2-digit' },
  d: { day: '2-digit', month: '2-digit', year: 'numeric' },
  D: { day: 'numeric', month: 'long', year: 'numeric' },
  f: { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' },
  F: { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }
};
const TS_FMT: Record<string, Intl.DateTimeFormat> = {};
for (const [k, opt] of Object.entries(TS_STYLES)) TS_FMT[k] = new Intl.DateTimeFormat(TS_LOCALE, opt);

export function relativeTime(unix: number): string {
  const diff = unix - Date.now() / 1000;
  const abs = Math.abs(diff);
  for (const [unit, secs] of REL_UNITS) {
    if (abs >= secs || unit === 'second') return RTF.format(Math.round(diff / secs), unit);
  }
  return '';
}

function absTime(d: Date, style: string): string {
  return (TS_FMT[style] ?? TS_FMT.f).format(d);
}

function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export type MarkupOpts = {
  usernames?: Set<string>;
  selfUsername?: string;
  roles?: Map<string, string | null>;
  userColors?: Map<string, string>;
  emojis?: Map<string, string>;
  emojiByKey?: Map<string, string>;
  stripUrls?: Set<string>;
};

function inline(text: string, opts: MarkupOpts): string {
  text = text
    .replace(
      /\|\|([^\n]+?)\|\|/g,
      '<span class="spoiler" tabindex="0" role="button" title="Spoiler">$1</span>'
    )
    .replace(/\*\*([^*\n]+?)\*\*/g, '<strong>$1</strong>')
    .replace(/__([^_\n]+?)__/g, '<u>$1</u>')
    .replace(/(^|[^*])\*([^*\n]+?)\*/g, '$1<em>$2</em>')
    .replace(/~~([^~\n]+?)~~/g, '<del>$1</del>')
    .replace(/==([^=\n]+?)==/g, '<mark class="rounded bg-warning/25 px-0.5 text-content">$1</mark>');

  text = text.replace(/&lt;t:(\d{1,15})(?::([tTdDfFR]))?&gt;/g, (m, sec: string, style: string) => {
    const unix = Number(sec);
    const d = tsDate(unix);
    if (!d) return m;
    const title = esc(absTime(d, 'F'));
    if (style === 'R') {
      return `<span class="ts-badge" data-ts="${unix}" title="${title}">${esc(relativeTime(unix))}</span>`;
    }
    return `<time class="ts-badge" datetime="${esc(d.toISOString())}" title="${title}">${esc(absTime(d, style || 'f'))}</time>`;
  });

  text = text.replace(/@(everyone|here)\b/g, `<span class="${MENTION_SELF}">@$1</span>`);

  const hasUsers = !!opts.usernames?.size;
  const hasRoles = !!opts.roles?.size;
  if (hasUsers || hasRoles) {
    const self = opts.selfUsername?.toLowerCase();
    text = text.replace(/@([A-Za-z0-9_][A-Za-z0-9_-]{0,31})/g, (m, name: string) => {
      const lower = name.toLowerCase();
      if (opts.usernames?.has(lower)) {
        const cls = self && lower === self ? MENTION_SELF : MENTION_OTHER;
        const color = opts.userColors?.get(lower);
        const style = color && HEX_COLOR.test(color) ? ` style="color:${color};--tw-text-opacity:1;"` : '';
        return `<span class="${cls} cursor-pointer hover:underline" data-mention-user="${esc(name)}"${style}>@${name}</span>`;
      }
      if (opts.roles?.has(lower)) {
        const color = opts.roles.get(lower);
        const style = color && HEX_COLOR.test(color) ? ` style="color:${color}"` : '';
        return `<span class="${MENTION_ROLE}"${style}>@${name}</span>`;
      }
      return m;
    });
  }

  text = text.replace(/&lt;:([a-z0-9_]{2,32}):([a-f0-9-]{36})&gt;/g, (_m, name: string, key: string) => {
    const url = opts.emojiByKey?.get(key);
    if (!url) return `:${name}:`;
    return `<img class="inline-emoji" src="${esc(url)}" alt=":${name}:" title=":${name}:" data-emoji-name="${esc(name)}" data-emoji-key="${esc(key)}" draggable="false">`;
  });

  if (opts.emojis?.size) {
    text = text.replace(/:([a-z0-9_]{2,32}):/g, (m, name: string) => {
      const url = opts.emojis!.get(name);
      if (!url) return m;
      return `<img class="inline-emoji" src="${esc(url)}" alt=":${name}:" title=":${name}:" data-emoji-name="${esc(name)}" draggable="false">`;
    });
  }
  return text;
}

const PLACEHOLDER_LINE = new RegExp(`^${D}\\d+${D}$`);

function isTableDelim(line: string): boolean {
  const t = line.trim();
  if (!t.includes('-')) return false;
  return /^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?$/.test(t);
}
function isTableStart(line: string, next: string | undefined): boolean {
  return next !== undefined && line.includes('|') && isTableDelim(next);
}

function startsNewBlock(line: string, next: string | undefined): boolean {
  return (
    /^\s*$/.test(line) ||
    /^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line) ||
    /^#{1,6}\s+\S/.test(line) ||
    /^\s*&gt;/.test(line) ||
    /^\s*(?:[-*]\s+|\d+\.\s+)/.test(line) ||
    isTableStart(line, next) ||
    PLACEHOLDER_LINE.test(line.trim())
  );
}

function headingHtml(lvl: number, inner: string): string {
  const cls =
    lvl <= 1
      ? 'mt-3 mb-1 text-subtitle font-bold text-content first:mt-0'
      : lvl === 2
        ? 'mt-3 mb-1 text-[1.0625rem] font-semibold text-content first:mt-0'
        : lvl === 3
          ? 'mt-2 mb-0.5 text-body font-semibold text-content first:mt-0'
          : 'mt-2 mb-0.5 text-body font-semibold text-muted first:mt-0';
  return `<h${lvl} class="${cls}">${inner}</h${lvl}>`;
}

function calloutHtml(type: string, title: string, bodyLines: string[], opts: MarkupOpts): string {
  const c = CALLOUTS[type];
  const titleHtml = title.trim() ? inline(title, opts) : c.label;
  const body = bodyLines.map((x) => inline(x, opts)).join('<br>');
  const isSpoiler = type === 'SPOILER';
  return (
    `<div class="callout ${c.cls} my-1 rounded-md px-3 py-2 first:mt-0">` +
    `<div class="callout-head mb-0.5 flex items-center gap-1.5 font-semibold">${c.icon}<span>${titleHtml}</span></div>` +
    `<div class="callout-body${isSpoiler ? ' spoiler-block' : ''}">${body}</div>` +
    `</div>`
  );
}

function blockquoteHtml(qlines: string[], opts: MarkupOpts): string {
  const inner = qlines.map((l) => l.replace(/^\s*&gt;\s?/, ''));
  const cm = /^\[!(\w+)\]\s*(.*)$/.exec(inner[0] ?? '');
  if (cm && CALLOUTS[cm[1].toUpperCase()]) {
    return calloutHtml(cm[1].toUpperCase(), cm[2], inner.slice(1), opts);
  }
  const parts: string[] = [];
  let buf: string[] = [];
  const flush = () => {
    if (buf.length) {
      parts.push(buf.map((x) => inline(x, opts)).join('<br>'));
      buf = [];
    }
  };
  let j = 0;
  while (j < inner.length) {
    if (/^\s*&gt;/.test(inner[j])) {
      flush();
      const s = j;
      while (j < inner.length && /^\s*&gt;/.test(inner[j])) j++;
      parts.push(blockquoteHtml(inner.slice(s, j), opts));
    } else {
      buf.push(inner[j]);
      j++;
    }
  }
  flush();
  return `<blockquote class="my-1 border-l-2 border-border-strong pl-3 text-muted first:mt-0">${parts.join('')}</blockquote>`;
}

function tableHtml(tlines: string[], opts: MarkupOpts): string {
  const splitRow = (row: string): string[] =>
    row
      .replace(/^\s*\|/, '')
      .replace(/\|\s*$/, '')
      .split('|')
      .map((c) => c.trim());

  const header = splitRow(tlines[0]);
  const align = splitRow(tlines[1]).map((c) => {
    const l = c.startsWith(':');
    const r = c.endsWith(':');
    return l && r ? 'text-center' : r ? 'text-right' : 'text-left';
  });
  const alignCls = (k: number) => align[k] ?? 'text-left';

  const th = header
    .map(
      (c, k) =>
        `<th class="border border-border px-2 py-1 font-semibold ${alignCls(k)}">${inline(c, opts)}</th>`
    )
    .join('');
  const body = tlines
    .slice(2)
    .filter((r) => r.includes('|'))
    .map((r) => {
      const cells = splitRow(r);
      const tds = header
        .map(
          (_h, k) =>
            `<td class="border border-border px-2 py-1 ${alignCls(k)}">${inline(cells[k] ?? '', opts)}</td>`
        )
        .join('');
      return `<tr>${tds}</tr>`;
    })
    .join('');
  return `<div class="my-1 overflow-x-auto first:mt-0"><table class="w-full border-collapse text-label"><thead><tr>${th}</tr></thead><tbody>${body}</tbody></table></div>`;
}

type ListItem = { indent: number; ordered: boolean; task: -1 | 0 | 1; content: string };

function listHtml(llines: string[], opts: MarkupOpts): string {
  const items: ListItem[] = llines.map((l) => {
    const m = /^(\s*)([-*]|\d+\.)\s+(.*)$/.exec(l)!;
    const indent = m[1].replace(/\t/g, '  ').length;
    const ordered = /\d/.test(m[2]);
    let content = m[3];
    let task: -1 | 0 | 1 = -1;
    const tm = /^\[([ xX])\]\s+(.*)$/.exec(content);
    if (!ordered && tm) {
      task = /[xX]/.test(tm[1]) ? 1 : 0;
      content = tm[2];
    }
    return { indent, ordered, task, content };
  });

  let idx = 0;
  const build = (minIndent: number): string => {
    const ordered = items[idx].ordered;
    const liParts: string[] = [];
    while (idx < items.length && items[idx].indent >= minIndent) {
      const it = items[idx];
      if (it.indent > minIndent) {
        const nested = build(it.indent);
        if (liParts.length) {
          liParts[liParts.length - 1] = liParts[liParts.length - 1].replace(
            /<\/li>$/,
            `${nested}</li>`
          );
        } else {
          liParts.push(nested);
        }
        continue;
      }
      idx++;
      if (it.task >= 0) {
        const box = it.task
          ? `<span role="checkbox" aria-checked="true" class="mt-0.5 grid size-3.5 shrink-0 place-items-center rounded-[3px] border border-primary bg-primary text-white"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg></span>`
          : `<span role="checkbox" aria-checked="false" class="mt-0.5 size-3.5 shrink-0 rounded-[3px] border border-border-strong"></span>`;
        liParts.push(
          `<li class="-ml-5 list-none"><span class="flex items-start gap-1.5">${box}<span>${inline(it.content, opts)}</span></span></li>`
        );
      } else {
        liParts.push(`<li>${inline(it.content, opts)}</li>`);
      }
    }
    const tag = ordered ? 'ol' : 'ul';
    const listCls = ordered
      ? 'my-1 ml-5 list-decimal space-y-0.5 marker:text-muted first:mt-0'
      : 'my-1 ml-5 list-disc space-y-0.5 marker:text-muted first:mt-0';
    return `<${tag} class="${listCls}">${liParts.join('')}</${tag}>`;
  };
  return build(items[0].indent);
}

const FOLD_LINES = 18;
function codeBlockHtml(raw: string): string {
  const body = raw.replace(/^\n/, '').replace(/\n+$/, '');
  const nl = body.indexOf('\n');
  const head = (nl === -1 ? body : body.slice(0, nl)).trim();
  let lang = '';
  let code = body;
  if (nl !== -1 && /^[a-zA-Z][\w+#.-]{0,19}$/.test(head)) {
    lang = head;
    code = body.slice(nl + 1);
  }
  const isDiff = lang.toLowerCase() === 'diff';
  const lines = code.split('\n');
  const numbered = lines.length > 1;
  const foldable = lines.length > FOLD_LINES;

  const rows = lines
    .map((ln) => {
      let cls = 'code-ln';
      if (isDiff) {
        if (/^\+(?!\+\+)/.test(ln)) cls += ' code-add';
        else if (/^-(?!--)/.test(ln)) cls += ' code-del';
      }
      return `<span class="${cls}">${esc(ln) || ' '}</span>`;
    })
    .join('');

  return (
    `<div class="code-block${foldable ? ' code-foldable' : ''} my-1 overflow-hidden rounded-md border border-border bg-base first:mt-0" data-code="${esc(code)}">` +
    `<div class="flex items-center justify-between border-b border-border/70 px-3 py-1 text-[0.6875rem] text-muted">` +
    `<span class="select-none font-mono lowercase">${esc(lang || 'code')}</span>` +
    `<button type="button" data-copy class="flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:bg-elevated hover:text-content">${COPY_ICON}<span class="copy-label">Copier</span></button>` +
    `</div>` +
    `<div class="code-scroll overflow-x-auto"><pre class="code-pre${numbered ? ' code-numbered' : ''} m-0 py-3 font-mono text-label leading-relaxed text-content/90">${rows}</pre></div>` +
    (foldable
      ? `<button type="button" data-fold class="flex w-full items-center justify-center border-t border-border/70 py-1 text-[0.6875rem] text-muted transition-colors hover:bg-elevated hover:text-content">Déplier</button>`
      : '') +
    `</div>`
  );
}

function parseBlocks(text: string, opts: MarkupOpts): string {
  const lines = text.split('\n');
  const blocks: { kind: string; html: string }[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    if (/^\s*$/.test(line)) {
      i++;
      continue;
    }
    if (/^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      blocks.push({ kind: 'hr', html: '<hr class="my-2 border-border">' });
      i++;
      continue;
    }
    const h = /^(#{1,6})\s+(.*\S)\s*$/.exec(line);
    if (h) {
      blocks.push({ kind: 'h', html: headingHtml(h[1].length, inline(h[2], opts)) });
      i++;
      continue;
    }
    if (PLACEHOLDER_LINE.test(line.trim())) {
      blocks.push({ kind: 'raw', html: line.trim() });
      i++;
      continue;
    }
    if (/^\s*&gt;/.test(line)) {
      const s = i;
      while (i < lines.length && /^\s*&gt;/.test(lines[i])) i++;
      blocks.push({ kind: 'quote', html: blockquoteHtml(lines.slice(s, i), opts) });
      continue;
    }
    if (isTableStart(line, lines[i + 1])) {
      const s = i;
      i += 2;
      while (i < lines.length && lines[i].includes('|') && !/^\s*$/.test(lines[i])) i++;
      blocks.push({ kind: 'table', html: tableHtml(lines.slice(s, i), opts) });
      continue;
    }
    if (/^\s*(?:[-*]\s+|\d+\.\s+)/.test(line)) {
      const s = i;
      while (i < lines.length && /^\s*(?:[-*]\s+|\d+\.\s+)/.test(lines[i])) i++;
      blocks.push({ kind: 'list', html: listHtml(lines.slice(s, i), opts) });
      continue;
    }
    const para: string[] = [];
    while (i < lines.length && !/^\s*$/.test(lines[i]) && !startsNewBlock(lines[i], lines[i + 1])) {
      para.push(inline(lines[i], opts));
      i++;
    }
    blocks.push({ kind: 'p', html: para.join('<br>') });
  }

  if (blocks.length === 1 && blocks[0].kind === 'p') return blocks[0].html;

  return blocks
    .map((b) => (b.kind === 'p' ? `<p class="mt-2 first:mt-0">${b.html}</p>` : b.html))
    .join('');
}

const EMOJI_TOKEN = /<a?:\w{2,32}:[a-f0-9-]{36}>|:\w{2,32}:/g;

export function isEmojiOnly(src: string | null | undefined): boolean {
  if (!src) return false;
  const hadCustom = EMOJI_TOKEN.test(src);
  EMOJI_TOKEN.lastIndex = 0;
  let rest = src.replace(EMOJI_TOKEN, '');
  rest = rest.replace(
    /[\p{Extended_Pictographic}\u{1F1E6}-\u{1F1FF}\u{1F3FB}-\u{1F3FF}\u{FE0F}\u{200D}\u{20E3}]/gu,
    ''
  );
  const hadUnicode = rest.length < src.replace(EMOJI_TOKEN, '').length;
  return rest.trim() === '' && (hadCustom || hadUnicode);
}

export function renderMarkup(src: string, opts: MarkupOpts = {}): string {
  const slots: string[] = [];
  const stash = (html: string) => `${D}${slots.push(html) - 1}${D}`;

  let text = (src ?? '').replace(/\0/g, '');

  text = text.replace(/```([\s\S]*?)```/g, (_m, code: string) => stash(codeBlockHtml(code)));
  text = text.replace(/`([^`\n]+?)`/g, (_m, code: string) =>
    stash(`<code class="rounded bg-base px-1 py-0.5 font-mono text-[0.85em] text-accent">${esc(code)}</code>`)
  );
  text = text.replace(/\bhttps?:\/\/[^\s<]+/g, (url: string) => {
    if (opts.stripUrls?.has(url)) return stash('');
    const safe = esc(url);
    return stash(
      `<a href="${safe}" target="_blank" rel="noopener noreferrer" class="text-accent underline underline-offset-2 hover:text-primary-hover">${safe}</a>`
    );
  });
  text = text.replace(/\\([\\`*_~|>#=+\-.!()[\]])/g, (_m, ch: string) => stash(esc(ch)));

  text = esc(text);

  text = parseBlocks(text, opts);

  return text.replace(new RegExp(`${D}(\\d+)${D}`, 'g'), (_m, i: string) => slots[Number(i)]);
}

export function mentions(body: string, username: string): boolean {
  if (!username) return false;
  const re = new RegExp(`@${username.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`, 'i');
  return re.test(body);
}
