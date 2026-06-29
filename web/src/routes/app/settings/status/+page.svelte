<script lang="ts">
  import { Check, Smile, X } from '@lucide/svelte';
  import { Button, Input } from '$lib/ui';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import {
    selfStatus,
    customStatus,
    customStatusEmoji,
    customStatusExpiresAt,
    setStatus,
    setCustomStatus,
    clearCustomStatus,
    autoDndGame,
    setAutoDndGame,
    setGaming,
    STATUS_META,
    type SelfStatus
  } from '$lib/stores/status';
  import { publishPresence, clearPresence } from '$lib/xmpp/presence';
  import { xmppState } from '$lib/xmpp/client';
  import { t } from '$lib/i18n';

  const OPTIONS = $derived<{ value: SelfStatus; label: string; desc: string }[]>([
    { value: 'online', label: $t('status.name.online'), desc: $t('status.desc.online') },
    { value: 'idle', label: $t('status.name.idle'), desc: $t('status.desc.idle') },
    { value: 'dnd', label: $t('status.name.dnd'), desc: $t('status.desc.dnd') },
    { value: 'invisible', label: $t('status.name.invisible'), desc: $t('status.desc.invisible') }
  ]);

  let draft = $state($customStatus);
  let emoji = $state($customStatusEmoji);
  let expiry = $state(0);
  let savedNote = $state(false);

  let emojiOpen = $state(false);
  let emojiWrapEl = $state<HTMLElement | null>(null);
  function pickEmoji(e: string) {
    emoji = e;
    emojiOpen = false;
  }
  function onEmojiWinPointerDown(ev: PointerEvent) {
    if (!emojiOpen) return;
    const target = ev.target as Node | null;
    if (target && emojiWrapEl?.contains(target)) return;
    emojiOpen = false;
  }

  const EXPIRIES = $derived([
    { v: 0, label: $t('status.expiry.never') },
    { v: 30, label: $t('status.expiry.30m') },
    { v: 60, label: $t('status.expiry.1h') },
    { v: 240, label: $t('status.expiry.4h') },
    { v: 1440, label: $t('status.expiry.today') }
  ]);

  function saveNote() {
    setCustomStatus(draft.trim(), emoji.trim(), expiry);
    savedNote = true;
    setTimeout(() => (savedNote = false), 1500);
  }
  function clearNote() {
    clearCustomStatus();
    draft = '';
    emoji = '';
    expiry = 0;
  }

  let game = $state('');
  let details = $state('');
  let stateText = $state('');
  let gameBusy = $state(false);
  let gameErr = $state<string | null>(null);
  let gameOk = $state(false);

  async function publishGame(e: Event) {
    e.preventDefault();
    gameErr = null;
    gameOk = false;
    gameBusy = true;
    try {
      await publishPresence({
        game: game.trim(),
        details: details.trim() || undefined,
        state: stateText.trim() || undefined,
        since: Math.floor(Date.now() / 1000)
      });
      setGaming(true);
      gameOk = true;
    } catch (e) {
      gameErr = e instanceof Error ? e.message : 'failed';
    } finally {
      gameBusy = false;
    }
  }

  async function clearGame() {
    gameErr = null;
    gameBusy = true;
    try {
      await clearPresence();
      setGaming(false);
      game = '';
      details = '';
      stateText = '';
    } catch (e) {
      gameErr = e instanceof Error ? e.message : 'failed';
    } finally {
      gameBusy = false;
    }
  }
</script>

<svelte:window
  onpointerdowncapture={onEmojiWinPointerDown}
  onkeydown={(e) => {
    if (e.key === 'Escape' && emojiOpen) emojiOpen = false;
  }}
/>

<div class="space-y-10">
  <section>
    <h2 class="text-subtitle font-semibold text-content">{$t('status.availability.title')}</h2>
    <p class="mt-1 text-body text-muted">{$t('status.availability.hint')}</p>

    <div class="mt-4 grid gap-2 sm:grid-cols-2">
      {#each OPTIONS as opt (opt.value)}
        {@const active = $selfStatus === opt.value}
        <button
          type="button"
          onclick={() => setStatus(opt.value)}
          class="flex items-start gap-3 rounded-lg border p-3 text-left transition-colors duration-150 ease-smooth
                 {active
            ? 'border-primary bg-primary/10'
            : 'border-border hover:border-border-strong hover:bg-elevated/50'}"
        >
          <span class="mt-1 size-2.5 shrink-0 rounded-full {STATUS_META[opt.value].dot}"></span>
          <span class="min-w-0">
            <span class="block text-body font-medium text-content">{opt.label}</span>
            <span class="block text-label text-muted">{opt.desc}</span>
          </span>
          {#if active}
            <Check size={16} class="ml-auto shrink-0 text-accent" />
          {/if}
        </button>
      {/each}
    </div>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$t('status.custom.title')}</h2>
    <p class="mt-1 text-body text-muted">{$t('status.custom.hint')}</p>

    <div class="mt-3 flex items-end gap-2">
      <div bind:this={emojiWrapEl} class="relative shrink-0">
        <button
          type="button"
          onclick={() => (emojiOpen = !emojiOpen)}
          aria-expanded={emojiOpen}
          aria-label={$t('status.emoji')}
          class="grid size-10 place-items-center rounded-lg border border-border text-xl transition-colors duration-150 hover:border-border-strong"
        >
          {#if emoji}{emoji}{:else}<Smile size={18} class="text-muted" />{/if}
        </button>
        {#if emoji}
          <button
            type="button"
            onclick={() => (emoji = '')}
            title="Retirer l'emoji"
            class="absolute -right-1.5 -top-1.5 grid size-4 place-items-center rounded-full bg-base text-muted ring-1 ring-border transition-colors hover:text-danger"
          >
            <X size={10} />
          </button>
        {/if}
        {#if emojiOpen}
          <div class="absolute left-0 top-full z-20 mt-1">
            <EmojiPicker onpick={pickEmoji} scope="krovara" />
          </div>
        {/if}
      </div>
      <div class="flex-1">
        <Input placeholder={$t('status.whatsup')} maxlength={128} bind:value={draft} />
      </div>
    </div>

    <div class="mt-3">
      <span class="block text-label font-medium text-muted">{$t('status.clearAfter')}</span>
      <div class="mt-1.5 flex flex-wrap gap-1">
        {#each EXPIRIES as o (o.v)}
          <button
            type="button"
            onclick={() => (expiry = o.v)}
            aria-pressed={expiry === o.v}
            class="rounded border px-2.5 py-1 text-label transition-colors duration-150
                   {expiry === o.v ? 'border-primary bg-primary/10 text-content' : 'border-border text-muted hover:border-border-strong'}"
          >
            {o.label}
          </button>
        {/each}
      </div>
    </div>

    <div class="mt-4 flex items-center gap-2">
      <Button type="button" onclick={saveNote}>{$t('status.save')}</Button>
      {#if $customStatus || $customStatusEmoji}
        <Button type="button" variant="ghost" onclick={clearNote}>{$t('status.clear')}</Button>
      {/if}
      {#if savedNote}
        <span class="flex items-center gap-1.5 text-label text-success"><Check size={14} /> {$t('status.updated')}</span>
      {/if}
    </div>
    {#if $customStatusExpiresAt}
      <p class="mt-2 text-label text-muted">
        {$t('status.expiresAt', {
          time: new Date($customStatusExpiresAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        })}
      </p>
    {/if}
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$t('status.game.title')}</h2>
    <p class="mt-1 text-body text-muted">{$t('status.game.hint')}</p>

    {#if $xmppState !== 'online'}
      <p class="mt-3 text-label text-warning">
        {$t('status.game.xmppWarn', { state: $xmppState })}
      </p>
    {/if}

    <form onsubmit={publishGame} class="mt-4 space-y-4">
      <Input label={$t('status.game.game')} required maxlength={64} placeholder="Valorant" bind:value={game} />
      <Input label={$t('status.game.details')} maxlength={128} placeholder="Compétitif" bind:value={details} />
      <Input label={$t('status.game.state')} maxlength={128} placeholder="Round 3" bind:value={stateText} />

      {#if gameErr}<p class="text-label text-danger">{gameErr}</p>{/if}
      {#if gameOk}<p class="text-label text-success">{$t('status.game.published')}</p>{/if}

      <div class="flex gap-2">
        <Button type="submit" loading={gameBusy}>{$t('status.game.publish')}</Button>
        <Button type="button" variant="secondary" disabled={gameBusy} onclick={clearGame}>{$t('status.clear')}</Button>
      </div>
    </form>

    <label class="mt-5 flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3">
      <input
        type="checkbox"
        checked={$autoDndGame}
        onchange={(e) => setAutoDndGame(e.currentTarget.checked)}
        class="mt-0.5 accent-primary"
      />
      <span class="min-w-0">
        <span class="block text-body text-content">{$t('status.game.autoDnd')}</span>
        <span class="block text-label text-muted">{$t('status.game.autoDndHint')}</span>
      </span>
    </label>
  </section>
</div>
