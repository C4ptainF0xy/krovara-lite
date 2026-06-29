<script lang="ts">
  import {
    SendHorizontal,
    Reply,
    X,
    Smile,
    Paperclip,
    AtSign,
    Film,
    ChevronLeft,
    ChevronRight,
    FileText,
    EyeOff,
    Eye
  } from '@lucide/svelte';
  import GifPicker from './GifPicker.svelte';
  import EmojiPicker from './EmojiPicker.svelte';
  import StickerPicker from './StickerPicker.svelte';
  import { Sticker } from '@lucide/svelte';
  import { stickerUrl, type CustomSticker } from '$lib/stores/stickers';
  import { emojisBySpace, loadEmojis, emojiUrl } from '$lib/stores/emojis';
  import { searchEmoji } from '$lib/emoji/data';
  import {
    serializeEditor,
    getCaretOffset,
    setCaretOffset,
    getSelectionOffsets,
    setSelectionOffsets,
    tokenizeForRender
  } from '$lib/composer/richtext';

  type Mentionable = { username: string; name: string };
  type MentionSuggestion = { insert: string; label: string; sub: string };

  type Props = {
    onsend: (text: string) => void | Promise<void>;
    disabled?: boolean;
    placeholder?: string;
    replyName?: string | null;
    replyPreview?: string | null;
    oncancelreply?: () => void;
    mentionables?: Mentionable[];
    mentionableRoles?: string[];
    ontyping?: () => void;
    onattach?: (files: File[], captions?: string[], spoilers?: boolean[]) => void | Promise<void>;
    ongif?: (gif: { url: string; name: string }) => void | Promise<void>;
    onsticker?: (sticker: CustomSticker) => void | Promise<void>;
    maxlength?: number;
    prefill?: string;
    prefillNonce?: number;
    draftKey?: string;
    spaceId?: string;
  };
  let {
    onsend,
    disabled = false,
    placeholder = 'Écris un message…',
    replyName = null,
    replyPreview = null,
    oncancelreply,
    mentionables = [],
    mentionableRoles = [],
    ontyping,
    onattach,
    ongif,
    onsticker,
    maxlength = 4000,
    prefill,
    prefillNonce,
    draftKey,
    spaceId
  }: Props = $props();

  let text = $state('');

  let editor = $state<HTMLDivElement | null>(null);
  let chipUrls = $state<Record<string, string>>({});

  $effect(() => {
    function onKey(e: KeyboardEvent) {
      if (disabled || e.ctrlKey || e.metaKey || e.altKey || e.key.length !== 1) return;
      const ae = document.activeElement as HTMLElement | null;
      if (ae && (ae.tagName === 'INPUT' || ae.tagName === 'TEXTAREA' || ae.isContentEditable)) return;
      if (!editor) return;
      editor.focus();
      setCaretOffset(editor, text.length);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const knownEmoji = (name: string) => !!serverEmojiList.find((e) => e.name === name);

  function ensureChipUrl(name: string) {
    if (chipUrls[name]) return;
    const e = serverEmojiList.find((x) => x.name === name);
    if (!e) return;
    void emojiUrl(e.file_key).then((u) => {
      chipUrls = { ...chipUrls, [name]: u };
      editor?.querySelectorAll(`img[data-emoji=":${name}:"]`).forEach((img) => {
        (img as HTMLImageElement).src = u;
      });
    });
  }

  function ensureChipUrlByKey(key: string, token: string) {
    if (chipUrls[key]) return;
    void emojiUrl(key).then((u) => {
      chipUrls = { ...chipUrls, [key]: u };
      editor?.querySelectorAll(`img[data-emoji="${token}"]`).forEach((img) => {
        (img as HTMLImageElement).src = u;
      });
    });
  }

  function renderEditor() {
    if (!editor) return;
    editor.replaceChildren();
    for (const part of tokenizeForRender(text, knownEmoji)) {
      if (part.type === 'emoji') {
        const img = document.createElement('img');
        img.className = 'emoji-chip';
        img.dataset.emoji = part.token;
        img.alt = part.token;
        img.setAttribute('contenteditable', 'false');
        img.draggable = false;
        if (part.key) {
          ensureChipUrlByKey(part.key, part.token);
          if (chipUrls[part.key]) img.src = chipUrls[part.key];
        } else {
          ensureChipUrl(part.name);
          if (chipUrls[part.name]) img.src = chipUrls[part.name];
        }
        editor.appendChild(img);
      } else {
        editor.appendChild(document.createTextNode(part.value));
      }
    }
  }

  const caret = () => (editor ? getCaretOffset(editor) : text.length);

  $effect(() => {
    if (spaceId && $emojisBySpace[spaceId] === undefined) {
      void loadEmojis(spaceId).catch(() => {});
    }
  });

  $effect(() => {
    void serverEmojiList;
    if (!editor || document.activeElement === editor) return;
    if (/:([a-z0-9_]{2,32}):/.test(text) && tokenizeForRender(text, knownEmoji).some((p) => p.type === 'emoji')) {
      renderEditor();
    }
  });

  function applyString(newText: string, caretPos?: number) {
    text = newText.slice(0, maxlength);
    renderEditor();
    const pos = Math.min(caretPos ?? text.length, text.length);
    queueMicrotask(() => {
      if (editor) {
        editor.focus();
        setCaretOffset(editor, pos);
      }
    });
    persistDraft();
  }

  function persistDraft() {
    if (!draftKey) return;
    try {
      if (text.trim()) localStorage.setItem('krovara:draft:' + draftKey, text);
      else localStorage.removeItem('krovara:draft:' + draftKey);
    } catch {
    }
  }
  let loadedDraftFor: string | undefined;
  $effect(() => {
    const k = draftKey;
    if (k === loadedDraftFor) return;
    loadedDraftFor = k;
    if (!k) return;
    try {
      text = localStorage.getItem('krovara:draft:' + k) ?? '';
    } catch {
      text = '';
    }
    queueMicrotask(renderEditor);
  });

  let appliedNonce = $state(-1);
  $effect(() => {
    if (prefill != null && prefillNonce != null && prefillNonce !== appliedNonce) {
      appliedNonce = prefillNonce;
      applyString(prefill);
    }
  });
  let fileInput: HTMLInputElement | null = $state(null);

  type Staged = { id: string; file: File; url: string; caption: string; isImage: boolean; spoiler: boolean };
  let staged = $state<Staged[]>([]);

  function stagedId(): string {
    const b = new Uint8Array(4);
    crypto.getRandomValues(b);
    return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
  }

  export function stageFiles(files: File[]) {
    for (const file of files) {
      staged.push({
        id: stagedId(),
        file,
        url: URL.createObjectURL(file),
        caption: '',
        isImage: file.type.startsWith('image/'),
        spoiler: false
      });
    }
  }

  function unstage(id: string) {
    const i = staged.findIndex((s) => s.id === id);
    if (i < 0) return;
    URL.revokeObjectURL(staged[i].url);
    staged.splice(i, 1);
  }

  function moveStaged(id: string, dir: -1 | 1) {
    const i = staged.findIndex((s) => s.id === id);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= staged.length) return;
    [staged[i], staged[j]] = [staged[j], staged[i]];
  }

  let emojiOpen = $state(false);
  let gifOpen = $state(false);
  let stickerOpen = $state(false);

  let toolbarEl = $state<HTMLElement | null>(null);
  let pickerWrapEl = $state<HTMLElement | null>(null);
  function onWindowPointerDown(e: PointerEvent) {
    if (!emojiOpen && !gifOpen && !stickerOpen) return;
    const t = e.target as Node | null;
    if (t && (toolbarEl?.contains(t) || pickerWrapEl?.contains(t))) return;
    emojiOpen = gifOpen = stickerOpen = false;
  }

  function chooseGif(gif: { url: string; name: string }) {
    gifOpen = false;
    void ongif?.(gif);
  }

  let pendingSticker = $state<CustomSticker | null>(null);
  let pendingStickerUrl = $state<string | null>(null);
  function chooseSticker(sticker: CustomSticker) {
    stickerOpen = false;
    pendingSticker = sticker;
    pendingStickerUrl = null;
    void stickerUrl(sticker.file_key).then((u) => {
      if (pendingSticker?.id === sticker.id) pendingStickerUrl = u;
    });
  }
  function clearPendingSticker() {
    pendingSticker = null;
    pendingStickerUrl = null;
  }

  let lastTyped = 0;
  function notifyTyping() {
    const now = performance.now();
    if (now - lastTyped > 3000) {
      lastTyped = now;
      ontyping?.();
    }
  }

  let mentionOpen = $state(false);
  let mentionQuery = $state('');
  let mentionIndex = $state(0);
  const mentionMatches = $derived.by<MentionSuggestion[]>(() => {
    if (!mentionOpen) return [];
    const specials: MentionSuggestion[] = [
      { insert: 'everyone', label: '@everyone', sub: 'tout le salon' },
      { insert: 'here', label: '@here', sub: 'membres en ligne' }
    ];
    const users: MentionSuggestion[] = mentionables.map((m) => ({
      insert: m.username,
      label: m.name,
      sub: '@' + m.username
    }));
    const roles: MentionSuggestion[] = mentionableRoles.map((name) => ({
      insert: name,
      label: '@' + name,
      sub: 'rôle'
    }));
    return [...specials, ...roles, ...users]
      .filter(
        (s) =>
          s.insert.toLowerCase().includes(mentionQuery) ||
          s.label.toLowerCase().includes(mentionQuery)
      )
      .slice(0, 6);
  });

  function refreshMention() {
    if (!editor) return;
    const c = caret();
    const before = text.slice(0, c);
    const m = before.match(/(?:^|\s)@(\w*)$/);
    if (m) {
      mentionOpen = true;
      mentionQuery = m[1].toLowerCase();
      mentionIndex = 0;
    } else {
      mentionOpen = false;
    }
  }

  function applyMention(item: MentionSuggestion) {
    if (!editor) return;
    const c = caret();
    const before = text.slice(0, c).replace(/@(\w*)$/, `@${item.insert} `);
    mentionOpen = false;
    applyString(before + text.slice(c), before.length);
  }

  type EmojiSuggestion = { insert: string; label: string; char?: string; thumb?: string };
  let emojiAutoOpen = $state(false);
  let emojiAutoQuery = $state('');
  let emojiAutoIndex = $state(0);
  let emojiAutoThumbs = $state<Record<string, string>>({});

  const serverEmojiList = $derived(spaceId ? ($emojisBySpace[spaceId] ?? []) : []);
  const emojiAutoMatches = $derived.by<EmojiSuggestion[]>(() => {
    if (!emojiAutoOpen) return [];
    const q = emojiAutoQuery;
    const server: EmojiSuggestion[] = serverEmojiList
      .filter((e) => e.name.includes(q))
      .slice(0, 8)
      .map((e) => ({ insert: `:${e.name}:`, label: `:${e.name}:`, thumb: emojiAutoThumbs[e.name] }));
    const standard: EmojiSuggestion[] = searchEmoji(q)
      .slice(0, 8)
      .map((it) => ({ insert: it.e, label: `:${it.k.split(' ')[0]}:`, char: it.e }));
    return [...server, ...standard].slice(0, 10);
  });

  $effect(() => {
    if (!emojiAutoOpen || !spaceId) return;
    if ($emojisBySpace[spaceId] === undefined) {
      void loadEmojis(spaceId).catch(() => {});
      return;
    }
    for (const e of serverEmojiList) {
      if (emojiAutoThumbs[e.name]) continue;
      void emojiUrl(e.file_key).then((u) => {
        emojiAutoThumbs = { ...emojiAutoThumbs, [e.name]: u };
      });
    }
  });

  function refreshEmojiAuto() {
    if (!editor) {
      emojiAutoOpen = false;
      return;
    }
    const before = text.slice(0, caret());
    const m = before.match(/(?:^|\s):([a-z0-9_]{2,})$/i);
    if (m) {
      emojiAutoOpen = true;
      emojiAutoQuery = m[1].toLowerCase();
      emojiAutoIndex = 0;
    } else {
      emojiAutoOpen = false;
    }
  }

  function applyEmojiAuto(item: EmojiSuggestion) {
    if (!editor) return;
    const c = caret();
    const before = text.slice(0, c).replace(/:([a-z0-9_]{2,})$/i, `${item.insert} `);
    emojiAutoOpen = false;
    applyString(before + text.slice(c), before.length);
  }

  function grow() {}

  function syncFromEditor() {
    if (!editor) return;
    text = serializeEditor(editor);
    const c = caret();
    const m = text.slice(0, c).match(/:([a-z0-9_]{2,32}):$/);
    if (m && knownEmoji(m[1])) {
      applyString(text, c);
    }
  }

  function onInput() {
    syncFromEditor();
    notifyTyping();
    refreshMention();
    refreshEmojiAuto();
    persistDraft();
  }

  function insertEmoji(e: string) {
    const c = caret();
    emojiOpen = false;
    applyString(text.slice(0, c) + e + text.slice(c), c + e.length);
  }

  function wrapSelection(prefix: string, suffix = prefix) {
    if (!editor) return;
    const [start, end] = getSelectionOffsets(editor);
    const selected = text.slice(start, end);
    const next = text.slice(0, start) + prefix + selected + suffix + text.slice(end);
    text = next.slice(0, maxlength);
    renderEditor();
    persistDraft();
    queueMicrotask(() => {
      editor?.focus();
      if (!editor) return;
      if (selected) {
        setSelectionOffsets(editor, start + prefix.length, end + prefix.length);
      } else {
        setCaretOffset(editor, start + prefix.length);
      }
    });
  }

  const PAIRS: Record<string, string> = {
    '*': '*', _: '_', '`': '`', '~': '~', '(': ')', '[': ']', '{': '}', '"': '"'
  };

  function continueListOnEnter(): boolean {
    if (!editor) return false;
    const [s, e] = getSelectionOffsets(editor);
    if (s !== e) return false;
    const c = s;
    const lineStart = text.lastIndexOf('\n', c - 1) + 1;
    const line = text.slice(lineStart, c);
    const m = line.match(/^(\s*)(([-*+])|(\d+)\.|(>))\s+(.*)$/);
    if (!m) return false;
    const [, indent, , bullet, num, quote, content] = m;
    if (content.trim() === '') {
      applyString(text.slice(0, lineStart) + text.slice(c), lineStart);
      return true;
    }
    const marker = num ? `${parseInt(num, 10) + 1}.` : quote ? '>' : bullet;
    const insert = `\n${indent}${marker} `;
    applyString(text.slice(0, c) + insert + text.slice(c), c + insert.length);
    return true;
  }

  const SLASH: Record<string, string> = {
    shrug: '¯\\_(ツ)_/¯',
    tableflip: '(╯°□°)╯︵ ┻━┻',
    unflip: '┬─┬ ノ( ゜-゜ノ)',
    lenny: '( ͡° ͜ʖ ͡°)'
  };
  function expandSlash(input: string): string {
    const m = input.match(/^\/(\w+)(?:\s+([\s\S]*))?$/);
    if (!m) return input;
    const emote = SLASH[m[1].toLowerCase()];
    if (!emote) return input;
    const rest = (m[2] ?? '').trim();
    return rest ? `${rest} ${emote}` : emote;
  }

  async function submit() {
    if (disabled) return;
    const t = expandSlash(text.trim());
    const toSend = staged;
    const sticker = pendingSticker;
    if (!t && !toSend.length && !sticker) return;
    text = '';
    renderEditor();
    persistDraft();
    mentionOpen = false;
    emojiAutoOpen = false;
    if (sticker) {
      clearPendingSticker();
      if (t) await onsend(t);
      await onsticker?.(sticker);
      editor?.focus();
      return;
    }
    if (toSend.length && onattach) {
      staged = [];
      const files = toSend.map((s) => s.file);
      const captions = toSend.map((s) => s.caption.trim());
      const spoilers = toSend.map((s) => s.spoiler);
      toSend.forEach((s) => URL.revokeObjectURL(s.url));
      if (t) await onsend(t);
      await onattach(files, captions, spoilers);
    } else if (t) {
      await onsend(t);
    }
    editor?.focus();
  }

  function onKeydown(e: KeyboardEvent) {
    if (emojiAutoOpen && emojiAutoMatches.length) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        emojiAutoIndex = (emojiAutoIndex + 1) % emojiAutoMatches.length;
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        emojiAutoIndex = (emojiAutoIndex - 1 + emojiAutoMatches.length) % emojiAutoMatches.length;
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        applyEmojiAuto(emojiAutoMatches[emojiAutoIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        emojiAutoOpen = false;
        return;
      }
    }
    if (mentionOpen && mentionMatches.length) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        mentionIndex = (mentionIndex + 1) % mentionMatches.length;
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        mentionIndex = (mentionIndex - 1 + mentionMatches.length) % mentionMatches.length;
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        applyMention(mentionMatches[mentionIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        mentionOpen = false;
        return;
      }
    }
    if ((e.ctrlKey || e.metaKey) && !e.altKey) {
      const k = e.key.toLowerCase();
      if (e.shiftKey && k === 'x') return void (e.preventDefault(), wrapSelection('||'));
      if (e.shiftKey) {
      } else if (k === 'b') return void (e.preventDefault(), wrapSelection('**'));
      else if (k === 'i') return void (e.preventDefault(), wrapSelection('*'));
      else if (k === 'e') return void (e.preventDefault(), wrapSelection('`'));
      else if (k === 'u') return void (e.preventDefault(), wrapSelection('__'));
    }
    if (!e.ctrlKey && !e.metaKey && !e.altKey && e.key.length === 1 && PAIRS[e.key] && editor) {
      const [ss, se] = getSelectionOffsets(editor);
      if (ss !== se) {
        e.preventDefault();
        wrapSelection(e.key, PAIRS[e.key]);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (continueListOnEnter()) return;
      void submit();
    }
  }

  function onPickFile(e: Event) {
    emojiOpen = gifOpen = stickerOpen = false;
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = '';
    if (files.length) stageFiles(files);
  }

  function onPaste(e: ClipboardEvent) {
    const files = Array.from(e.clipboardData?.files ?? []);
    if (files.length === 0) {
      for (const it of Array.from(e.clipboardData?.items ?? [])) {
        if (it.kind === 'file' && it.type.startsWith('image/')) {
          const f = it.getAsFile();
          if (f) files.push(f);
        }
      }
    }
    if (files.length && onattach) {
      e.preventDefault();
      stageFiles(files);
      return;
    }
    const t = e.clipboardData?.getData('text/plain');
    if (t != null) {
      e.preventDefault();
      const [s, en] = editor ? getSelectionOffsets(editor) : [text.length, text.length];
      applyString(text.slice(0, s) + t + text.slice(en), s + t.length);
      notifyTyping();
      refreshMention();
      refreshEmojiAuto();
    }
  }

  const canSend = $derived(
    (text.trim().length > 0 || staged.length > 0 || !!pendingSticker) && !disabled
  );
  const nearLimit = $derived(text.length > maxlength * 0.85);
</script>

<svelte:window
  onpointerdowncapture={onWindowPointerDown}
  onkeydown={(e) => {
    if (e.key === 'Escape' && (emojiOpen || gifOpen || stickerOpen)) {
      emojiOpen = gifOpen = stickerOpen = false;
    }
  }}
/>

<div class="relative shrink-0 px-4 pb-[calc(1rem+var(--safe-bottom))] pt-1">
  {#if replyName}
    <div class="mb-1 flex items-center gap-2 rounded-t-lg border border-b-0 border-border bg-elevated/50 px-3 py-1.5 text-label text-muted">
      <Reply size={14} class="shrink-0" />
      <span class="truncate">
        Réponse à <span class="font-medium text-content">{replyName}</span>{#if replyPreview}<span class="text-muted/80"> · {replyPreview}</span>{/if}
      </span>
      <button
        type="button"
        onclick={() => oncancelreply?.()}
        title="Annuler la réponse"
        class="ml-auto grid size-5 shrink-0 place-items-center rounded text-muted hover:text-content"
      >
        <X size={14} />
      </button>
    </div>
  {/if}

  {#if emojiAutoOpen && emojiAutoMatches.length}
    <div class="absolute bottom-full left-4 z-20 mb-1 w-64 overflow-hidden rounded-lg border border-border bg-overlay shadow-xl">
      <p class="border-b border-border px-3 py-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted/70">
        Emoji correspondant à :{emojiAutoQuery}
      </p>
      {#each emojiAutoMatches as em, i (em.label + em.insert)}
        <button
          type="button"
          onmousedown={(e) => {
            e.preventDefault();
            applyEmojiAuto(em);
          }}
          class="flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors
                 {i === emojiAutoIndex ? 'bg-elevated text-content' : 'text-muted hover:bg-elevated/60'}"
        >
          {#if em.thumb}
            <img src={em.thumb} alt="" class="size-5 shrink-0 object-contain" />
          {:else if em.char}
            <span class="grid size-5 shrink-0 place-items-center text-base">{em.char}</span>
          {:else}
            <span class="size-5 shrink-0 animate-pulse rounded bg-elevated"></span>
          {/if}
          <span class="truncate text-label text-content">{em.label}</span>
        </button>
      {/each}
    </div>
  {/if}

  {#if mentionOpen && mentionMatches.length}
    <div class="absolute bottom-full left-4 z-20 mb-1 w-64 overflow-hidden rounded-lg border border-border bg-overlay shadow-xl">
      {#each mentionMatches as m, i (m.insert + m.sub)}
        <button
          type="button"
          onmousedown={(e) => {
            e.preventDefault();
            applyMention(m);
          }}
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-body transition-colors
                 {i === mentionIndex ? 'bg-elevated text-content' : 'text-muted hover:bg-elevated/60'}"
        >
          <AtSign size={14} class="shrink-0 text-muted" />
          <span class="truncate font-medium text-content">{m.label}</span>
          <span class="truncate text-label text-muted">{m.sub}</span>
        </button>
      {/each}
    </div>
  {/if}

  <div bind:this={pickerWrapEl} class="contents">
    {#if emojiOpen}
      <div class="absolute bottom-full right-4 z-20 mb-1">
        <EmojiPicker onpick={insertEmoji} {spaceId} />
      </div>
    {/if}

    {#if stickerOpen && onsticker}
      <div class="absolute bottom-full right-4 z-20 mb-1">
        <StickerPicker {spaceId} onpick={chooseSticker} />
      </div>
    {/if}

    {#if gifOpen && ongif}
      <div class="absolute bottom-full right-4 z-20 mb-1 rounded-lg border border-border bg-overlay p-2 shadow-xl">
        <GifPicker onpick={chooseGif} />
      </div>
    {/if}
  </div>

  {#if staged.length}
    <div class="mb-1.5 flex flex-wrap gap-2 rounded-lg border border-border bg-surface/60 p-2">
      {#each staged as s, i (s.id)}
        <div class="flex w-36 flex-col gap-1 rounded-md border border-border bg-base/60 p-1.5">
          <div class="relative">
            {#if s.isImage}
              <img src={s.url} alt={s.file.name} class="h-20 w-full rounded object-cover" />
            {:else}
              <div class="flex h-20 w-full flex-col items-center justify-center gap-1 rounded bg-elevated px-1">
                <FileText size={20} class="text-muted" />
                <span class="w-full truncate text-center text-[0.625rem] text-muted">{s.file.name}</span>
              </div>
            {/if}
            {#if s.spoiler}
              <div class="pointer-events-none absolute inset-0 grid place-items-center rounded bg-base/40 backdrop-blur-md">
                <span class="flex items-center gap-1 rounded bg-base/80 px-1.5 py-0.5 text-[0.625rem] font-semibold text-content">
                  <EyeOff size={11} /> Spoiler
                </span>
              </div>
            {/if}
            <button
              type="button"
              onclick={() => unstage(s.id)}
              title="Retirer"
              aria-label="Retirer"
              class="absolute -right-1.5 -top-1.5 z-10 grid size-5 place-items-center rounded-full border border-border bg-surface text-muted shadow hover:bg-elevated hover:text-content"
            >
              <X size={12} />
            </button>
          </div>
          <input
            bind:value={s.caption}
            placeholder="Légende…"
            class="w-full rounded bg-base px-1.5 py-1 text-label text-content outline-none placeholder:text-muted/50"
          />
          <div class="flex items-center justify-between">
            <button
              type="button"
              onclick={() => (s.spoiler = !s.spoiler)}
              aria-pressed={s.spoiler}
              title={s.spoiler ? 'Spoiler activé' : 'Marquer comme spoiler'}
              aria-label="Marquer comme spoiler"
              class="grid size-5 place-items-center rounded transition-colors hover:bg-elevated
                     {s.spoiler ? 'text-warning' : 'text-muted hover:text-content'}"
            >
              {#if s.spoiler}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
            </button>
            <div class="flex">
              <button
                type="button"
                onclick={() => moveStaged(s.id, -1)}
                disabled={i === 0}
                title="Déplacer à gauche"
                aria-label="Déplacer à gauche"
                class="grid size-5 place-items-center rounded text-muted hover:bg-elevated hover:text-content disabled:opacity-30"
              >
                <ChevronLeft size={14} />
              </button>
              <button
                type="button"
                onclick={() => moveStaged(s.id, 1)}
                disabled={i === staged.length - 1}
                title="Déplacer à droite"
                aria-label="Déplacer à droite"
                class="grid size-5 place-items-center rounded text-muted hover:bg-elevated hover:text-content disabled:opacity-30"
              >
                <ChevronRight size={14} />
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  {#if pendingSticker}
    <div class="relative z-20 mb-1.5 inline-flex items-center gap-2 rounded-lg border border-border bg-surface/60 p-2">
      {#if pendingStickerUrl}
        <img src={pendingStickerUrl} alt={pendingSticker.name} class="size-16 rounded object-contain" />
      {:else}
        <span class="size-16 animate-pulse rounded bg-elevated"></span>
      {/if}
      <span class="text-label text-muted">{pendingSticker.name}</span>
      <button
        type="button"
        onclick={clearPendingSticker}
        title="Retirer"
        aria-label="Retirer le sticker"
        class="grid size-6 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-danger"
      >
        <X size={14} />
      </button>
    </div>
  {/if}

  <div
    bind:this={toolbarEl}
    class="relative z-20 flex items-end gap-1.5 rounded-lg border border-border bg-surface px-2 py-2
           transition-[border-color,box-shadow] duration-150 ease-smooth
           focus-within:border-brand focus-within:shadow-[0_0_0_3px_rgba(122,115,152,0.25)]
           {disabled ? 'opacity-60' : ''}"
  >
    {#if onattach}
      <label
        title="Joindre un fichier"
        aria-label="Joindre un fichier"
        class="mb-0.5 grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content {disabled ? 'pointer-events-none opacity-40' : ''}"
      >
        <input bind:this={fileInput} type="file" multiple class="sr-only" disabled={disabled} onchange={onPickFile} />
        <Paperclip size={18} />
      </label>
    {/if}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      bind:this={editor}
      contenteditable={!disabled}
      role="textbox"
      tabindex="0"
      aria-multiline="true"
      aria-label={placeholder}
      data-placeholder={placeholder}
      oninput={onInput}
      onkeydown={onKeydown}
      onpaste={onPaste}
      onblur={() => {
        mentionOpen = false;
        emojiAutoOpen = false;
      }}
      spellcheck="true"
      lang="fr"
      autocapitalize="sentences"
      class="composer-editor max-h-40 min-h-[1.5rem] flex-1 self-center overflow-y-auto whitespace-pre-wrap break-words
             bg-transparent py-1.5 text-body text-content outline-none
             empty:before:text-muted/60 empty:before:content-[attr(data-placeholder)]"
    ></div>
    {#if ongif}
      <button
        type="button"
        onclick={() => {
          gifOpen = !gifOpen;
          emojiOpen = false;
          stickerOpen = false;
        }}
        disabled={disabled}
        title="GIF"
        aria-label="GIF"
        class="mb-0.5 grid size-8 shrink-0 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content disabled:opacity-40 {gifOpen
          ? 'bg-elevated text-content'
          : ''}"
      >
        <Film size={18} />
      </button>
    {/if}
    {#if onsticker && spaceId}
      <button
        type="button"
        onclick={() => {
          stickerOpen = !stickerOpen;
          emojiOpen = false;
          gifOpen = false;
        }}
        disabled={disabled}
        title="Sticker"
        aria-label="Sticker"
        class="mb-0.5 grid size-8 shrink-0 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content disabled:opacity-40 {stickerOpen
          ? 'bg-elevated text-content'
          : ''}"
      >
        <Sticker size={18} />
      </button>
    {/if}
    <button
      type="button"
      onclick={() => {
        emojiOpen = !emojiOpen;
        gifOpen = false;
        stickerOpen = false;
      }}
      disabled={disabled}
      title="Emoji"
      aria-label="Emoji"
      class="mb-0.5 grid size-8 shrink-0 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content disabled:opacity-40"
    >
      <Smile size={18} />
    </button>
    <button
      type="button"
      onclick={submit}
      disabled={!canSend}
      title="Envoyer"
      aria-label="Envoyer"
      class="mb-0.5 grid size-8 shrink-0 place-items-center rounded-md text-muted
             transition-colors duration-150
             enabled:hover:bg-primary enabled:hover:text-white
             {canSend ? 'text-content' : ''}
             disabled:cursor-default disabled:opacity-40"
    >
      <SendHorizontal size={18} />
    </button>
  </div>
  <div class="mt-1.5 flex items-center gap-2 px-1 text-[0.6875rem] text-muted/70">
    <p class="min-w-0 truncate">
      <kbd class="font-sans">Entrée</kbd> pour envoyer ·
      <kbd class="font-sans">Maj+Entrée</kbd> saut de ligne ·
      <kbd class="font-sans">Ctrl+B/I/E</kbd> mise en forme · <kbd class="font-sans">@</kbd> mentionner
    </p>
    {#if nearLimit}
      <span
        class="ml-auto shrink-0 tabular-nums {text.length >= maxlength ? 'text-danger' : 'text-muted'}"
      >
        {text.length}/{maxlength}
      </span>
    {/if}
  </div>
</div>

<style>
  .composer-editor :global(img.emoji-chip) {
    display: inline-block;
    width: 1.4em;
    height: 1.4em;
    vertical-align: -0.3em;
    object-fit: contain;
    margin: 0 0.05em;
    user-select: all;
  }
  .composer-editor:focus:empty::before,
  .composer-editor:empty::before {
    pointer-events: none;
  }
</style>
