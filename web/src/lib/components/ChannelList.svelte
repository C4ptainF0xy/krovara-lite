<script lang="ts">
  import { ChevronRight, Plus, Pencil, Trash2, MoreVertical, Settings, Hash, UserPlus, Columns2, Bell, BellOff, BellRing, Link2, Check } from '@lucide/svelte';
  import { Popover, ContextMenu } from 'bits-ui';
  import { longpress } from '$lib/actions/longpress';
  import ChannelItem from './ChannelItem.svelte';
  import type { Channel, Category } from '$lib/stores/spaces';
  import { moveChannel, updateCategory, deleteCategory } from '$lib/stores/spaces';
  import { setNotifSetting, type NotifLevel } from '$lib/stores/inbox';

  let copiedChannel = $state<string | null>(null);
  async function copyChannelLink(channel: Channel) {
    const base = typeof location !== 'undefined' ? location.origin : '';
    try {
      await navigator.clipboard.writeText(`${base}/app/spaces/${spaceId}/channels/${channel.id}`);
      copiedChannel = channel.id;
      setTimeout(() => {
        if (copiedChannel === channel.id) copiedChannel = null;
      }, 1200);
    } catch {
    }
  }

  async function setChannelNotif(channelId: string, level: NotifLevel) {
    try {
      await setNotifSetting('channel', channelId, { level });
    } catch {
    }
  }

  type Props = {
    spaceId: string;
    channels: Channel[];
    categories: Category[];
    activeChannelId: string | null;
    unread: Record<string, number>;
    mentions?: Record<string, number>;
    canManage?: boolean;
    onSelect: (channel: Channel) => void;
    onActivate?: (channel: Channel) => void;
    onAddChannel: (categoryId: string | null) => void;
    onAddCategory?: () => void;
    onSettings: (channel: Channel) => void;
    oninvite?: () => void;
    onsplit?: (channel: Channel) => void;
  };
  let {
    spaceId,
    channels,
    categories,
    activeChannelId,
    unread,
    mentions = {},
    canManage = false,
    onSelect,
    onActivate,
    onAddChannel,
    onAddCategory,
    onSettings,
    oninvite,
    onsplit
  }: Props = $props();

  const collapseKey = $derived(`krovara:cats-collapsed:${spaceId}`);
  let collapsed = $state<Set<string>>(new Set());
  $effect(() => {
    try {
      const raw = localStorage.getItem(collapseKey);
      collapsed = new Set(raw ? (JSON.parse(raw) as string[]) : []);
    } catch {
      collapsed = new Set();
    }
  });
  function toggle(id: string) {
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
    try {
      localStorage.setItem(collapseKey, JSON.stringify([...next]));
    } catch {
    }
  }

  const sortedCategories = $derived(
    [...categories].sort((a, b) => a.position - b.position || a.created_at.localeCompare(b.created_at))
  );
  function channelsIn(categoryId: string | null): Channel[] {
    return channels
      .filter((c) => (c.category_id ?? null) === categoryId)
      .sort(
        (a, b) =>
          (a.position ?? 0) - (b.position ?? 0) || a.created_at.localeCompare(b.created_at)
      );
  }
  const rootChannels = $derived(channelsIn(null));

  let dragId = $state<string | null>(null);
  let dropHint = $state<string | null>(null);

  function onDragStart(e: DragEvent, channelId: string) {
    if (!canManage) return;
    dragId = channelId;
    e.dataTransfer?.setData('text/plain', channelId);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  }
  function onDragEnd() {
    dragId = null;
    dropHint = null;
  }
  function allowDrop(e: DragEvent, hint: string) {
    if (!dragId) return;
    e.preventDefault();
    dropHint = hint;
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  }

  async function drop(categoryId: string | null, beforeId: string | null) {
    const id = dragId;
    dropHint = null;
    dragId = null;
    if (!id) return;

    const target = channelsIn(categoryId).filter((c) => c.id !== id);
    const idx = beforeId ? target.findIndex((c) => c.id === beforeId) : target.length;
    const moved = channels.find((c) => c.id === id);
    if (!moved) return;
    const ordered = [...target];
    ordered.splice(idx < 0 ? ordered.length : idx, 0, moved);

    await Promise.all(
      ordered.map((c, i) => {
        const sameCat = (c.category_id ?? null) === categoryId;
        if (sameCat && (c.position ?? 0) === i) return Promise.resolve();
        return moveChannel(spaceId, c.id, categoryId, i).catch(() => {});
      })
    );
  }

  let renaming = $state<string | null>(null);
  let renameValue = $state('');
  function startRename(cat: Category) {
    renaming = cat.id;
    renameValue = cat.name;
  }
  async function commitRename(cat: Category) {
    const name = renameValue.trim();
    renaming = null;
    if (name && name !== cat.name) await updateCategory(spaceId, cat.id, { name });
  }
  async function removeCategory(cat: Category) {
    if (!confirm(`Supprimer la catégorie « ${cat.name} » ? Les salons reviendront à la racine.`))
      return;
    await deleteCategory(spaceId, cat.id);
  }
</script>

{#snippet channelRow(channel: Channel, categoryId: string | null)}
  <div
    role="presentation"
    draggable={canManage}
    ondragstart={(e) => onDragStart(e, channel.id)}
    ondragend={onDragEnd}
    ondragover={(e) => allowDrop(e, `${categoryId ?? 'root'}:${channel.id}`)}
    ondrop={() => drop(categoryId, channel.id)}
    class="group/ch relative {dragId === channel.id ? 'opacity-40' : ''}"
  >
    {#if dropHint === `${categoryId ?? 'root'}:${channel.id}`}
      <span class="absolute -top-px left-2 right-2 h-0.5 rounded-full bg-accent" aria-hidden="true"
      ></span>
    {/if}
    <ContextMenu.Root>
      <ContextMenu.Trigger>
        {#snippet child({ props })}
          <div {...props} use:longpress>
            <ChannelItem
              {channel}
              {spaceId}
              active={channel.id === activeChannelId}
              unread={unread[channel.id] ?? 0}
              mentions={mentions[channel.id] ?? 0}
              onclick={() => onSelect(channel)}
              ondblclick={onActivate ? () => onActivate(channel) : undefined}
            />
          </div>
        {/snippet}
      </ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content class="z-50 w-56 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in">
          {#if onsplit && channel.type === 'text'}
            <ContextMenu.Item onSelect={() => onsplit?.(channel)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
              <Columns2 size={14} class="text-muted" /> Vue divisée
            </ContextMenu.Item>
          {/if}
          <ContextMenu.Sub>
            <ContextMenu.SubTrigger class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
              <Bell size={14} class="text-muted" /> Notifications
              <ChevronRight size={13} class="ml-auto text-muted" />
            </ContextMenu.SubTrigger>
            <ContextMenu.SubContent class="z-50 w-48 rounded-lg border border-border bg-overlay p-1 shadow-xl animate-fade-in">
              <ContextMenu.Item onSelect={() => setChannelNotif(channel.id, 'all')} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <BellRing size={14} class="text-muted" /> Tous les messages
              </ContextMenu.Item>
              <ContextMenu.Item onSelect={() => setChannelNotif(channel.id, 'mentions')} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <Bell size={14} class="text-muted" /> Mentions seulement
              </ContextMenu.Item>
              <ContextMenu.Item onSelect={() => setChannelNotif(channel.id, 'nothing')} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <BellOff size={14} class="text-muted" /> Rien
              </ContextMenu.Item>
            </ContextMenu.SubContent>
          </ContextMenu.Sub>
          <ContextMenu.Item onSelect={() => copyChannelLink(channel)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
            {#if copiedChannel === channel.id}<Check size={14} class="text-success" /> Lien copié{:else}<Link2 size={14} class="text-muted" /> Copier le lien{/if}
          </ContextMenu.Item>
          {#if canManage}
            <div class="my-1 h-px bg-border/60"></div>
            <ContextMenu.Item onSelect={() => onSettings(channel)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
              <Settings size={14} class="text-muted" /> Paramètres du salon
            </ContextMenu.Item>
            <ContextMenu.Item onSelect={() => onAddChannel(channel.category_id ?? null)} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
              <Hash size={14} class="text-muted" /> Créer un salon ici
            </ContextMenu.Item>
            {#if onAddCategory}
              <ContextMenu.Item onSelect={() => onAddCategory?.()} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <Plus size={14} class="text-muted" /> Créer une catégorie
              </ContextMenu.Item>
            {/if}
            {#if oninvite}
              <ContextMenu.Item onSelect={() => oninvite?.()} class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-label text-content transition-colors data-[highlighted]:bg-elevated">
                <UserPlus size={14} class="text-muted" /> Inviter
              </ContextMenu.Item>
            {/if}
          {/if}
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
    {#if canManage}
      <button
        type="button"
        title="Paramètres du salon"
        onclick={(e) => {
          e.stopPropagation();
          onSettings(channel);
        }}
        class="absolute right-1.5 top-1/2 grid size-6 -translate-y-1/2 place-items-center rounded
               text-muted opacity-0 transition-[opacity,color] duration-150
               hover:text-content focus-visible:opacity-100 group-hover/ch:opacity-100"
      >
        <Settings size={14} />
      </button>
    {/if}
  </div>
{/snippet}

<div class="space-y-0.5">
  {#each rootChannels as channel (channel.id)}
    {@render channelRow(channel, null)}
  {/each}
  {#if dragId}
    <div
      role="presentation"
      ondragover={(e) => allowDrop(e, 'root:end')}
      ondrop={() => drop(null, null)}
      class="h-2 {dropHint === 'root:end' ? 'border-t-2 border-accent' : ''}"
    ></div>
  {/if}

  {#each sortedCategories as cat (cat.id)}
    {@const open = !collapsed.has(cat.id)}
    <div class="pt-3">
      <div
        class="group/cat flex items-center gap-1 px-1"
        ondragover={(e) => allowDrop(e, `${cat.id}:end`)}
        ondrop={() => drop(cat.id, null)}
        role="presentation"
      >
        <button
          type="button"
          onclick={() => toggle(cat.id)}
          class="flex min-w-0 flex-1 items-center gap-1 rounded px-1 py-0.5 text-label font-semibold
                 uppercase tracking-wide text-muted transition-colors duration-150 hover:text-content"
        >
          <ChevronRight
            size={12}
            class="shrink-0 transition-transform duration-150 ease-smooth {open ? 'rotate-90' : ''}"
          />
          {#if renaming === cat.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              autofocus
              bind:value={renameValue}
              onclick={(e) => e.stopPropagation()}
              onkeydown={(e) => {
                if (e.key === 'Enter') commitRename(cat);
                if (e.key === 'Escape') renaming = null;
              }}
              onblur={() => commitRename(cat)}
              maxlength={64}
              class="min-w-0 flex-1 rounded bg-base px-1 py-0.5 text-label font-semibold uppercase
                     tracking-wide text-content outline-none ring-1 ring-border-strong"
            />
          {:else}
            <span class="truncate">{cat.name}</span>
          {/if}
        </button>
        {#if canManage && renaming !== cat.id}
          <button
            type="button"
            title="Ajouter un salon"
            onclick={() => onAddChannel(cat.id)}
            class="grid size-5 shrink-0 place-items-center rounded text-muted opacity-0
                   transition-[opacity,color] duration-150 hover:text-content group-hover/cat:opacity-100"
          >
            <Plus size={14} />
          </button>
          <Popover.Root>
            <Popover.Trigger
              title="Options de catégorie"
              class="grid size-5 shrink-0 place-items-center rounded text-muted opacity-0
                     transition-[opacity,color] duration-150 hover:text-content group-hover/cat:opacity-100"
            >
              <MoreVertical size={14} />
            </Popover.Trigger>
            <Popover.Portal>
              <Popover.Content
                align="end"
                sideOffset={4}
                class="z-50 w-44 rounded-lg border border-border bg-overlay p-1 shadow-lg
                       animate-fade-in"
              >
                <Popover.Close
                  onclick={() => startRename(cat)}
                  class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-body
                         text-content transition-colors duration-150 hover:bg-elevated"
                >
                  <Pencil size={15} /> Renommer
                </Popover.Close>
                <Popover.Close
                  onclick={() => removeCategory(cat)}
                  class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-body
                         text-danger transition-colors duration-150 hover:bg-elevated"
                >
                  <Trash2 size={15} /> Supprimer
                </Popover.Close>
              </Popover.Content>
            </Popover.Portal>
          </Popover.Root>
        {/if}
      </div>

      {#if open}
        <div class="mt-0.5 space-y-0.5">
          {#each channelsIn(cat.id) as channel (channel.id)}
            {@render channelRow(channel, cat.id)}
          {/each}
          {#if channelsIn(cat.id).length === 0}
            <div
              role="presentation"
              ondragover={(e) => allowDrop(e, `${cat.id}:end`)}
              ondrop={() => drop(cat.id, null)}
              class="px-2 py-1 text-label text-muted/70 {dropHint === `${cat.id}:end`
                ? 'rounded bg-elevated/60'
                : ''}"
            >
              Vide
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/each}
</div>
