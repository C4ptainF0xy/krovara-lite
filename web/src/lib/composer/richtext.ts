export function serializeEditor(root: HTMLElement): string {
  let out = '';
  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      out += node.textContent ?? '';
      return;
    }
    const el = node as HTMLElement;
    if (el.dataset && el.dataset.emoji) {
      out += el.dataset.emoji;
      return;
    }
    if (el.nodeName === 'BR') {
      out += '\n';
      return;
    }
    const isBlock = el.nodeName === 'DIV' || el.nodeName === 'P';
    if (isBlock && out.length > 0 && !out.endsWith('\n')) out += '\n';
    for (const child of el.childNodes) walk(child);
  };
  for (const child of root.childNodes) walk(child);
  return out;
}

export function getCaretOffset(root: HTMLElement): number {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return serializeEditor(root).length;
  const range = sel.getRangeAt(0);
  if (!root.contains(range.startContainer)) return serializeEditor(root).length;
  const pre = document.createRange();
  pre.selectNodeContents(root);
  pre.setEnd(range.startContainer, range.startOffset);
  const tmp = document.createElement('div');
  tmp.appendChild(pre.cloneContents());
  return serializeEditor(tmp).length;
}

export function setCaretOffset(root: HTMLElement, offset: number): void {
  const sel = window.getSelection();
  if (!sel) return;
  let remaining = offset;
  const range = document.createRange();
  let placed = false;

  const walk = (node: Node): boolean => {
    if (placed) return true;
    if (node.nodeType === Node.TEXT_NODE) {
      const len = (node.textContent ?? '').length;
      if (remaining <= len) {
        range.setStart(node, remaining);
        placed = true;
        return true;
      }
      remaining -= len;
      return false;
    }
    const el = node as HTMLElement;
    if (el.dataset && el.dataset.emoji) {
      const len = el.dataset.emoji.length;
      if (remaining < len) {
        range.setStartBefore(el);
        placed = true;
        return true;
      }
      remaining -= len;
      return false;
    }
    if (el.nodeName === 'BR') {
      if (remaining < 1) {
        range.setStartBefore(el);
        placed = true;
        return true;
      }
      remaining -= 1;
      return false;
    }
    for (const child of el.childNodes) if (walk(child)) return true;
    return false;
  };

  for (const child of root.childNodes) if (walk(child)) break;

  if (!placed) {
    range.selectNodeContents(root);
    range.collapse(false);
  } else {
    range.collapse(true);
  }
  sel.removeAllRanges();
  sel.addRange(range);
}

export function getSelectionOffsets(root: HTMLElement): [number, number] {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) {
    const n = serializeEditor(root).length;
    return [n, n];
  }
  const range = sel.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    const n = serializeEditor(root).length;
    return [n, n];
  }
  const measure = (container: Node, off: number): number => {
    const pre = document.createRange();
    pre.selectNodeContents(root);
    pre.setEnd(container, off);
    const tmp = document.createElement('div');
    tmp.appendChild(pre.cloneContents());
    return serializeEditor(tmp).length;
  };
  return [measure(range.startContainer, range.startOffset), measure(range.endContainer, range.endOffset)];
}

export function setSelectionOffsets(root: HTMLElement, start: number, end: number): void {
  const sel = window.getSelection();
  if (!sel) return;
  setCaretOffset(root, start);
  if (end <= start) return;
  const r1 = sel.getRangeAt(0);
  setCaretOffset(root, end);
  const r2 = sel.getRangeAt(0);
  const range = document.createRange();
  range.setStart(r1.startContainer, r1.startOffset);
  range.setEnd(r2.startContainer, r2.startOffset);
  sel.removeAllRanges();
  sel.addRange(range);
}

export type RenderPart =
  | { type: 'text'; value: string }
  | { type: 'emoji'; name: string; token: string; key?: string };

export function tokenizeForRender(s: string, isEmoji: (name: string) => boolean): RenderPart[] {
  const parts: RenderPart[] = [];
  const re = /<:([a-z0-9_]{2,32}):([a-f0-9-]{36})>|:([a-z0-9_]{2,32}):/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s))) {
    if (m[2]) {
      if (m.index > last) parts.push({ type: 'text', value: s.slice(last, m.index) });
      parts.push({ type: 'emoji', name: m[1], token: m[0], key: m[2] });
      last = m.index + m[0].length;
    } else {
      const name = m[3];
      if (!isEmoji(name)) continue;
      if (m.index > last) parts.push({ type: 'text', value: s.slice(last, m.index) });
      parts.push({ type: 'emoji', name, token: m[0] });
      last = m.index + m[0].length;
    }
  }
  if (last < s.length) parts.push({ type: 'text', value: s.slice(last) });
  return parts;
}
