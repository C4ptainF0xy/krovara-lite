<script lang="ts">
  import { tick } from 'svelte';
  import { ArrowLeft, Users, Settings, UserMinus, Crown, Link as LinkIcon, LogOut, Check, Copy, ImagePlus } from '@lucide/svelte';
  import { auth } from '$lib/stores/auth';
  import { api, authedObjectURL } from '$lib/api';
  import {
    groupById, groupMessages, openGroup, closeGroup, sendGroupMessage,
    renameGroup, setGroupIcon, leaveGroup, kickFromGroup, transferGroup,
    createGroupInvite, type DMGroup
  } from '$lib/stores/groups';
  import MessageInput from '$lib/components/MessageInput.svelte';
  import { Popover } from 'bits-ui';

  let { groupId, onback }: { groupId: string; onback?: () => void } = $props();

  const group = $derived<DMGroup | undefined>($groupById[groupId]);
  const messages = $derived($groupMessages[groupId] ?? []);
  const selfId = $derived($auth.user?.id ?? '');
  const isOwner = $derived(group?.owner_id === selfId);
  const title = $derived(group?.name || group?.members?.map((m) => m.display_name).join(', ') || 'Groupe');

  let thread = $state<HTMLDivElement | null>(null);
  let sending = $state(false);
  let showSettings = $state(false);
  let renameVal = $state('');
  let inviteCode = $state<string | null>(null);
  let inviteCopied = $state(false);

  $effect(() => {
    openGroup(groupId);
    return () => closeGroup();
  });
  $effect(() => {
    messages.length;
    if (thread) void tick().then(() => thread && (thread.scrollTop = thread.scrollHeight));
  });

  function initials(s: string) { return s.slice(0, 2).toUpperCase(); }
  function timeOf(d: string) { return new Date(d).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }); }
  function nameOf(uid: string) {
    return group?.members?.find((m) => m.id === uid)?.display_name ?? 'Utilisateur';
  }

  async function submit(body: string) {
    if (!body || sending) return;
    sending = true;
    try { await sendGroupMessage(groupId, body); } finally { sending = false; }
  }
  async function doRename() {
    const n = renameVal.trim();
    if (n) await renameGroup(groupId, n).catch(() => {});
  }
  let uploadingIcon = $state(false);
  async function onIconPick(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;
    uploadingIcon = true;
    try {
      const form = new FormData();
      form.append('file', file);
      const dto = await api<{ id: string }>('/api/files?kind=attachment', { method: 'POST', body: form });
      await setGroupIcon(groupId, dto.id);
    } catch (err) {
      console.error('group icon upload failed', err);
    } finally {
      uploadingIcon = false;
      (e.target as HTMLInputElement).value = '';
    }
  }
  async function makeInvite() {
    inviteCode = await createGroupInvite(groupId).catch(() => null);
  }
  function copyInvite() {
    if (!inviteCode) return;
    void navigator.clipboard?.writeText(`${location.origin}/g/${inviteCode}`);
    inviteCopied = true;
    setTimeout(() => (inviteCopied = false), 1500);
  }
</script>

<section class="flex h-full min-h-0 min-w-0 flex-1 flex-col">
  {#if !group}
    <div class="grid flex-1 place-items-center text-label text-muted">Chargement…</div>
  {:else}
    <header class="flex items-center gap-3 border-b border-border px-4 py-3 shrink-0">
      {#if onback}
        <button type="button" onclick={onback} class="grid size-8 shrink-0 place-items-center rounded-md text-muted hover:bg-elevated hover:text-content md:hidden"><ArrowLeft size={18} /></button>
      {/if}
      {#if group.icon_key}
        {#await authedObjectURL(`/api/files/${group.icon_key}`) then src}
          <img {src} alt="" class="size-8 rounded-full object-cover" />
        {/await}
      {:else}
        <span class="grid size-8 place-items-center rounded-full bg-elevated text-muted"><Users size={16} /></span>
      {/if}
      <div class="min-w-0 flex-1">
        <p class="truncate text-body font-semibold text-content">{title}</p>
        <p class="text-label text-muted">{group.members?.length ?? group.member_count ?? 0} membres</p>
      </div>
      <button type="button" onclick={() => (showSettings = !showSettings)} title="Paramètres du groupe" class="grid size-8 place-items-center rounded-md text-muted hover:bg-elevated hover:text-content"><Settings size={18} /></button>
    </header>

    <div class="flex min-h-0 flex-1">
      <div class="flex min-w-0 flex-1 flex-col">
        <div bind:this={thread} class="min-h-0 flex-1 overflow-y-auto py-4">
          {#if messages.length === 0}
            <div class="grid h-full place-items-center px-4 text-center text-body text-muted">C'est le début de ce groupe.</div>
          {:else}
            {#each messages as m (m.id)}
              <div class="flex gap-3 px-4 py-1 hover:bg-black/5">
                <span class="grid size-9 shrink-0 place-items-center rounded-full bg-elevated text-label font-medium text-muted">{initials(m.mine ? ($auth.user?.username ?? 'Moi') : nameOf(m.author_id))}</span>
                <div class="min-w-0 flex-1">
                  <div class="flex items-baseline gap-2">
                    <span class="text-body font-medium text-content">{m.mine ? ($auth.user?.username ?? 'Moi') : nameOf(m.author_id)}</span>
                    <span class="text-[0.6875rem] text-muted">{timeOf(m.at)}</span>
                  </div>
                  <div class="text-body text-content/90 break-words">{m.body}</div>
                </div>
              </div>
            {/each}
          {/if}
        </div>
        <MessageInput placeholder={`Message dans ${title}`} onsend={submit} />
      </div>

      {#if showSettings}
        <aside class="hidden w-64 shrink-0 overflow-y-auto border-l border-border bg-surface/40 p-3 md:block">
          <div class="mb-3 flex flex-col items-center">
            <label class="group/icon relative cursor-pointer" title="Changer l'image du groupe">
              {#if group.icon_key}
                {#await authedObjectURL(`/api/files/${group.icon_key}`) then src}
                  <img {src} alt="" class="size-16 rounded-full object-cover" />
                {/await}
              {:else}
                <span class="grid size-16 place-items-center rounded-full bg-elevated text-muted"><Users size={26} /></span>
              {/if}
              <span class="absolute inset-0 grid place-items-center rounded-full bg-black/45 opacity-0 transition group-hover/icon:opacity-100">
                {#if uploadingIcon}
                  <span class="size-4 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
                {:else}
                  <ImagePlus size={18} class="text-white" />
                {/if}
              </span>
              <input type="file" accept="image/*" class="hidden" onchange={onIconPick} />
            </label>
          </div>
          <div class="mb-3">
            <p class="mb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Nom du groupe</p>
            <div class="flex gap-1.5">
              <input bind:value={renameVal} placeholder={group.name ?? title} class="h-8 min-w-0 flex-1 rounded border border-border bg-base/50 px-2 text-label text-content outline-none focus:border-primary" />
              <button type="button" onclick={doRename} class="rounded bg-primary px-2 py-1 text-label text-white hover:bg-primary-hover"><Check size={14} /></button>
            </div>
          </div>

          {#if isOwner}
            <div class="mb-3">
              <p class="mb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Inviter</p>
              {#if inviteCode}
                <div class="flex items-center gap-1.5 rounded border border-border bg-base/50 px-2 py-1">
                  <code class="min-w-0 flex-1 truncate text-[0.6875rem] text-accent">/g/{inviteCode}</code>
                  <button type="button" onclick={copyInvite} class="text-muted hover:text-content">{#if inviteCopied}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}</button>
                </div>
              {:else}
                <button type="button" onclick={makeInvite} class="flex w-full items-center justify-center gap-1.5 rounded-md border border-border px-2 py-1.5 text-label text-muted hover:text-content"><LinkIcon size={14} /> Créer un lien</button>
              {/if}
            </div>
          {/if}

          <p class="mb-1 text-[0.6875rem] font-semibold uppercase tracking-wide text-muted">Membres ({group.members?.length ?? 0})</p>
          <div class="space-y-0.5">
            {#each group.members ?? [] as m (m.id)}
              <div class="group/m flex items-center gap-2 rounded px-1.5 py-1 hover:bg-elevated/60">
                {#if m.avatar_key}
                  {#await authedObjectURL(`/api/files/${m.avatar_key}`) then src}<img {src} alt="" class="size-7 rounded-full object-cover" />{/await}
                {:else}
                  <span class="grid size-7 place-items-center rounded-full bg-elevated text-[0.625rem] font-semibold text-content">{initials(m.username)}</span>
                {/if}
                <span class="min-w-0 flex-1 truncate text-label text-content">{m.display_name}{#if m.id === group.owner_id}<Crown size={11} class="ml-1 inline text-warning" />{/if}</span>
                {#if isOwner && m.id !== selfId}
                  <Popover.Root>
                    <Popover.Trigger class="opacity-0 transition group-hover/m:opacity-100 text-muted hover:text-content">⋯</Popover.Trigger>
                    <Popover.Content align="end" class="z-50 w-44 rounded-lg border border-border bg-overlay p-1 shadow-xl">
                      <button type="button" onclick={() => transferGroup(groupId, m.id)} class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-label text-content hover:bg-elevated"><Crown size={14} class="text-muted" /> Transférer la propriété</button>
                      <button type="button" onclick={() => kickFromGroup(groupId, m.id)} class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-label text-danger hover:bg-danger/10"><UserMinus size={14} /> Expulser</button>
                    </Popover.Content>
                  </Popover.Root>
                {/if}
              </div>
            {/each}
          </div>

          <button type="button" onclick={() => { void leaveGroup(groupId); onback?.(); }} class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-md border border-border px-2 py-1.5 text-label text-danger hover:border-danger/50"><LogOut size={14} /> Quitter le groupe</button>
        </aside>
      {/if}
    </div>
  {/if}
</section>
