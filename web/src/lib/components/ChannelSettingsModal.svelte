<script lang="ts">
  import Modal from './Modal.svelte';
  import ChannelPermissionsEditor from './ChannelPermissionsEditor.svelte';
  import { Button, Input } from '$lib/ui';
  import { Hash, Megaphone, ShieldAlert, Timer } from '@lucide/svelte';
  import { updateChannel, type Channel } from '$lib/stores/spaces';
  import { emojisBySpace, loadEmojis, emojiUrl } from '$lib/stores/emojis';

  type Props = {
    open: boolean;
    channel: Channel | null;
    spaceId: string;
    onclose: () => void;
  };
  let { open, channel, spaceId, onclose }: Props = $props();

  const EMOJIS =
    '💬 📢 📌 🎮 🎉 🔥 ⭐ ✅ 📰 🛠️ 🎨 🎵 📷 🤝 🧠 🐛 💡 🚀 ❤️ 👀'.split(' ');

  const SLOWMODE = [
    { v: 0, label: 'Off' },
    { v: 5, label: '5 s' },
    { v: 15, label: '15 s' },
    { v: 30, label: '30 s' },
    { v: 60, label: '1 min' },
    { v: 300, label: '5 min' },
    { v: 900, label: '15 min' },
    { v: 3600, label: '1 h' }
  ];

  let name = $state('');
  let topic = $state('');
  let slowmode = $state(0);
  let nsfw = $state(false);
  let readOnly = $state(false);
  let iconEmoji = $state<string | null>(null);
  let busy = $state(false);
  let err = $state<string | null>(null);
  let tab = $state<'general' | 'permissions'>('general');

  $effect(() => {
    if (open && channel) {
      name = channel.name;
      topic = channel.topic ?? '';
      slowmode = channel.slowmode_seconds ?? 0;
      nsfw = channel.nsfw ?? false;
      readOnly = channel.read_only ?? false;
      iconEmoji = channel.icon_emoji ?? null;
      err = null;
      tab = 'general';
    }
  });

  const serverEmojis = $derived(spaceId ? ($emojisBySpace[spaceId] ?? []) : []);
  let emojiThumbs = $state<Record<string, string>>({});
  $effect(() => {
    if (!open || !spaceId) return;
    if ($emojisBySpace[spaceId] === undefined) {
      void loadEmojis(spaceId).catch(() => {});
      return;
    }
    let cancelled = false;
    void Promise.all(
      serverEmojis.map(async (e) => [e.name, await emojiUrl(e.file_key)] as const)
    ).then((entries) => {
      if (!cancelled) emojiThumbs = Object.fromEntries(entries);
    });
    return () => {
      cancelled = true;
    };
  });

  async function save(e: Event) {
    e.preventDefault();
    if (!channel) return;
    busy = true;
    err = null;
    try {
      await updateChannel(spaceId, channel.id, {
        name: name.trim(),
        topic: topic.trim(),
        slowmode_seconds: slowmode,
        nsfw,
        read_only: readOnly,
        icon_emoji: iconEmoji
      });
      onclose();
    } catch (e2) {
      err = e2 instanceof Error ? e2.message : 'Échec de l’enregistrement';
    } finally {
      busy = false;
    }
  }
</script>

<Modal {open} title="Paramètres du salon" {onclose}>
  {#if channel}
    <div class="mb-4 flex gap-1 border-b border-border">
      <button
        type="button"
        onclick={() => (tab = 'general')}
        class="-mb-px border-b-2 px-3 py-2 text-label font-medium transition-colors duration-150
               {tab === 'general'
          ? 'border-primary text-content'
          : 'border-transparent text-muted hover:text-content'}"
      >
        Général
      </button>
      <button
        type="button"
        onclick={() => (tab = 'permissions')}
        class="-mb-px border-b-2 px-3 py-2 text-label font-medium transition-colors duration-150
               {tab === 'permissions'
          ? 'border-primary text-content'
          : 'border-transparent text-muted hover:text-content'}"
      >
        Permissions
      </button>
    </div>

    {#if tab === 'permissions'}
      <ChannelPermissionsEditor channelId={channel.id} {spaceId} />
    {:else}
    <form onsubmit={save} class="space-y-5">
      <Input label="Nom du salon" bind:value={name} required minlength={1} maxlength={64} />

      <div class="space-y-1.5">
        <label for="ch-topic" class="block text-label font-medium text-muted">
          Sujet / description
        </label>
        <textarea
          id="ch-topic"
          bind:value={topic}
          maxlength={512}
          rows={2}
          placeholder="De quoi parle ce salon ?"
          class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body
                 text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth
                 placeholder:text-muted/60 focus:border-primary
                 focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
        ></textarea>
      </div>

      <div class="space-y-1.5">
        <span class="block text-label font-medium text-muted">Icône</span>
        <div class="flex flex-wrap gap-1">
          <button
            type="button"
            onclick={() => (iconEmoji = null)}
            title="Aucune (afficher #)"
            aria-pressed={iconEmoji === null}
            class="grid size-8 place-items-center rounded border transition-colors duration-150
                   {iconEmoji === null
              ? 'border-primary bg-primary/10 text-content'
              : 'border-border text-muted hover:border-border-strong'}"
          >
            <Hash size={16} />
          </button>
          {#each EMOJIS as e (e)}
            <button
              type="button"
              onclick={() => (iconEmoji = e)}
              aria-pressed={iconEmoji === e}
              class="grid size-8 place-items-center rounded border text-lg transition-colors duration-150
                     {iconEmoji === e
                ? 'border-primary bg-primary/10'
                : 'border-transparent hover:bg-elevated'}"
            >
              {e}
            </button>
          {/each}
        </div>
        {#if serverEmojis.length}
          <p class="mt-2 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted/70">Emojis du serveur</p>
          <div class="mt-1 flex flex-wrap gap-1">
            {#each serverEmojis as e (e.id)}
              {@const token = ':' + e.name + ':'}
              <button
                type="button"
                onclick={() => (iconEmoji = token)}
                title={token}
                aria-pressed={iconEmoji === token}
                class="grid size-8 place-items-center rounded border p-1 transition-colors duration-150
                       {iconEmoji === token
                  ? 'border-primary bg-primary/10'
                  : 'border-transparent hover:bg-elevated'}"
              >
                {#if emojiThumbs[e.name]}
                  <img src={emojiThumbs[e.name]} alt={e.name} class="size-full object-contain" />
                {:else}
                  <span class="size-full animate-pulse rounded bg-elevated"></span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <div class="space-y-1.5">
        <span class="flex items-center gap-1.5 text-label font-medium text-muted">
          <Timer size={14} /> Mode lent
        </span>
        <div class="flex flex-wrap gap-1">
          {#each SLOWMODE as s (s.v)}
            <button
              type="button"
              onclick={() => (slowmode = s.v)}
              aria-pressed={slowmode === s.v}
              class="rounded border px-2.5 py-1 text-label transition-colors duration-150
                     {slowmode === s.v
                ? 'border-primary bg-primary/10 text-content'
                : 'border-border text-muted hover:border-border-strong'}"
            >
              {s.label}
            </button>
          {/each}
        </div>
      </div>

      <div class="space-y-2">
        <label
          class="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3
                 transition-colors duration-150 hover:bg-elevated/40"
        >
          <input type="checkbox" bind:checked={readOnly} class="mt-0.5 accent-primary" />
          <span class="min-w-0">
            <span class="flex items-center gap-1.5 text-body text-content">
              <Megaphone size={15} /> Salon annonce (lecture seule)
            </span>
            <span class="mt-0.5 block text-label text-muted">
              Seuls les modérateurs peuvent y publier.
            </span>
          </span>
        </label>

        <label
          class="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3
                 transition-colors duration-150 hover:bg-elevated/40"
        >
          <input type="checkbox" bind:checked={nsfw} class="mt-0.5 accent-danger" />
          <span class="min-w-0">
            <span class="flex items-center gap-1.5 text-body text-content">
              <ShieldAlert size={15} /> Contenu sensible (NSFW)
            </span>
            <span class="mt-0.5 block text-label text-muted">
              Un avertissement s’affiche avant d’ouvrir le salon.
            </span>
          </span>
        </label>
      </div>

      {#if err}
        <p class="text-label text-danger">{err}</p>
      {/if}
      <Button type="submit" full loading={busy}>Enregistrer</Button>
    </form>
    {/if}
  {/if}
</Modal>
