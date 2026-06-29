<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Check, Camera, Trash2, Plus } from '@lucide/svelte';
  import { api, authedObjectURL } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import { t } from '$lib/i18n';
  import { Button, Input } from '$lib/ui';
  import ImageCropper from '$lib/components/ImageCropper.svelte';

  type ProfileLink = { label: string; url: string };
  type Me = {
    id: string;
    username: string;
    email: string;
    display_name: string | null;
    status: string | null;
    avatar_key: string | null;
    banner_key?: string | null;
    bio?: string | null;
    pronouns?: string | null;
    links?: ProfileLink[];
    is_admin?: boolean;
  };

  type FileDTO = { id: string };

  let me = $state<Me | null>(null);
  let displayName = $state('');
  let status = $state('');
  let bio = $state('');
  let pronouns = $state('');
  let links = $state<ProfileLink[]>([]);
  let loading = $state(true);
  let busy = $state(false);
  let saved = $state(false);
  let err = $state<string | null>(null);

  let avatarUrl = $state<string | null>(null);
  let avatarBusy = $state(false);
  let avatarErr = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let cropFile = $state<File | null>(null);
  let cropOpen = $state(false);

  let bannerUrl = $state<string | null>(null);
  let bannerBusy = $state(false);
  let bannerErr = $state<string | null>(null);
  let bannerInput = $state<HTMLInputElement | null>(null);
  let bannerCropFile = $state<File | null>(null);
  let bannerCropOpen = $state(false);

  function onPickBanner(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    bannerErr = null;
    if (!file.type.startsWith('image/')) {
      bannerErr = $t('profile.err.notImage');
      return;
    }
    bannerCropFile = file;
    bannerCropOpen = true;
  }

  async function uploadBanner(file: File) {
    bannerErr = null;
    bannerBusy = true;
    try {
      const form = new FormData();
      form.append('file', file);
      const up = await api<FileDTO>('/api/files?kind=banner', { method: 'POST', body: form });
      await api<Me>('/api/me', { method: 'PATCH', body: { banner_key: up.id } });
      if (me) me.banner_key = up.id;
      if (bannerUrl) URL.revokeObjectURL(bannerUrl);
      bannerUrl = await authedObjectURL(`/api/files/${up.id}`);
      bannerCropOpen = false;
    } catch (err) {
      bannerErr = err instanceof Error ? err.message : $t('profile.err.upload');
    } finally {
      bannerBusy = false;
    }
  }

  async function onBannerCropped(blob: Blob) {
    const gif = blob.type === 'image/gif';
    await uploadBanner(new File([blob], gif ? 'banner.gif' : 'banner.png', { type: blob.type || 'image/png' }));
  }

  async function removeBanner() {
    bannerBusy = true;
    try {
      await api<Me>('/api/me', { method: 'PATCH', body: { banner_key: null } });
      if (me) me.banner_key = null;
      if (bannerUrl) URL.revokeObjectURL(bannerUrl);
      bannerUrl = null;
    } catch (err) {
      bannerErr = err instanceof Error ? err.message : $t('profile.err.save');
    } finally {
      bannerBusy = false;
    }
  }

  function setAvatarUrl(next: string | null) {
    if (avatarUrl) URL.revokeObjectURL(avatarUrl);
    avatarUrl = next;
  }

  onMount(async () => {
    try {
      me = await api<Me>('/api/me');
      displayName = me.display_name ?? '';
      status = me.status ?? '';
      bio = me.bio ?? '';
      pronouns = me.pronouns ?? '';
      links = me.links ?? [];
      if (me.avatar_key) {
        try {
          setAvatarUrl(await authedObjectURL(`/api/files/${me.avatar_key}`));
        } catch {
        }
      }
      if (me.banner_key) {
        try {
          bannerUrl = await authedObjectURL(`/api/files/${me.banner_key}`);
        } catch {
        }
      }
    } catch (e) {
      err = e instanceof Error ? e.message : $t('profile.err.load');
    } finally {
      loading = false;
    }
  });

  onDestroy(() => {
    if (avatarUrl) URL.revokeObjectURL(avatarUrl);
  });

  function onPickAvatar(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    avatarErr = null;
    if (!file.type.startsWith('image/')) {
      avatarErr = $t('profile.err.notImage');
      return;
    }
    cropFile = file;
    cropOpen = true;
  }

  async function uploadAvatar(file: File) {
    avatarErr = null;
    avatarBusy = true;
    setAvatarUrl(URL.createObjectURL(file));
    try {
      const form = new FormData();
      form.append('file', file);
      const dto = await api<FileDTO>('/api/me/avatar', { method: 'POST', body: form });
      if (me) me.avatar_key = dto.id;
      cropOpen = false;
    } catch (e) {
      avatarErr = e instanceof Error ? e.message : $t('profile.err.upload');
    } finally {
      avatarBusy = false;
    }
  }

  async function onCropped(blob: Blob) {
    const gif = blob.type === 'image/gif';
    await uploadAvatar(new File([blob], gif ? 'avatar.gif' : 'avatar.png', { type: blob.type || 'image/png' }));
  }

  async function save(e: Event) {
    e.preventDefault();
    err = null;
    saved = false;
    busy = true;
    try {
      const cleanLinks = links
        .map((l) => ({ label: l.label.trim(), url: l.url.trim() }))
        .filter((l) => l.url);
      const updated = await api<Me>('/api/me', {
        method: 'PATCH',
        body: {
          display_name: displayName.trim() || null,
          status: status.trim() || null,
          bio: bio.trim() || null,
          pronouns: pronouns.trim() || null,
          links: cleanLinks
        }
      });
      me = updated;
      auth.update((s) => ({
        ...s,
        user: s.user
          ? { ...s.user, display_name: updated.display_name, status: updated.status }
          : s.user
      }));
      saved = true;
      setTimeout(() => (saved = false), 2000);
    } catch (e) {
      err = e instanceof Error ? e.message : $t('profile.err.save');
    } finally {
      busy = false;
    }
  }

  const shownName = $derived(displayName.trim() || me?.username || '');
</script>

{#if loading}
  <div class="space-y-3">
    {#each [0, 1, 2] as i (i)}
      <div class="h-10 animate-pulse rounded bg-surface"></div>
    {/each}
  </div>
{:else if me}
  <div class="mb-4 overflow-hidden rounded-xl border border-border">
    <div
      class="relative h-28 bg-gradient-to-r from-primary/30 to-brand/20 bg-cover bg-center"
      style={bannerUrl ? `background-image:url(${bannerUrl})` : ''}
    >
      <div class="absolute bottom-2 right-2 flex gap-1.5">
        <label
          class="cursor-pointer rounded-md bg-base/70 px-2.5 py-1 text-label text-content backdrop-blur transition-colors hover:bg-base {bannerBusy ? 'pointer-events-none opacity-50' : ''}"
        >
          <input bind:this={bannerInput} type="file" accept="image/*" class="sr-only" disabled={bannerBusy} onchange={onPickBanner} />
          {bannerBusy ? '…' : bannerUrl ? 'Changer la bannière' : 'Ajouter une bannière'}
        </label>
        {#if bannerUrl && !bannerBusy}
          <button
            type="button"
            onclick={removeBanner}
            class="rounded-md bg-base/70 px-2.5 py-1 text-label text-muted backdrop-blur transition-colors hover:bg-base hover:text-danger"
          >
            Retirer
          </button>
        {/if}
      </div>
    </div>
  </div>
  {#if bannerErr}<p class="mb-3 text-label text-danger">{bannerErr}</p>{/if}

  <div class="mb-8 flex items-center gap-4">
    <label
      aria-label={$t('profile.changeAvatar')}
      class="group relative grid size-16 shrink-0 cursor-pointer place-items-center overflow-hidden rounded-full
             bg-elevated text-subtitle font-semibold text-muted ring-1 ring-border
             transition-[box-shadow,filter] duration-150 ease-smooth
             hover:ring-brand focus-within:outline-none focus-within:ring-2 focus-within:ring-brand
             {avatarBusy ? 'pointer-events-none' : ''}"
    >
      <input
        bind:this={fileInput}
        type="file"
        accept="image/*"
        class="sr-only"
        disabled={avatarBusy}
        onchange={onPickAvatar}
      />
      {#if avatarUrl}
        <img src={avatarUrl} alt="" class="size-full object-cover" />
      {:else}
        {shownName.slice(0, 2).toUpperCase()}
      {/if}
      <span
        class="absolute inset-0 grid place-items-center bg-base/60 opacity-0 transition-opacity
               duration-150 ease-smooth group-hover:opacity-100 {avatarBusy ? 'opacity-100' : ''}"
      >
        {#if avatarBusy}
          <span class="size-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
        {:else}
          <Camera size={20} strokeWidth={2} class="text-content" />
        {/if}
      </span>
    </label>
    <div class="min-w-0">
      <p class="truncate text-subtitle font-bold">{shownName}</p>
      <p class="truncate text-body text-muted">@{me.username} · {me.email}</p>
      {#if avatarErr}
        <p class="mt-1 text-label text-danger">{avatarErr}</p>
      {:else}
        <label
          class="mt-1 inline-block cursor-pointer text-label text-accent transition-colors duration-150 ease-smooth hover:underline"
        >
          <input type="file" accept="image/*" class="sr-only" disabled={avatarBusy} onchange={onPickAvatar} />
          {$t('profile.changeAvatar')}
        </label>
      {/if}
    </div>
  </div>

  <form onsubmit={save} class="max-w-md space-y-4">
    <Input
      label={$t('profile.displayName')}
      placeholder={me.username}
      maxlength={64}
      bind:value={displayName}
    />
    <p class="-mt-2 text-label text-muted">
      {$t('profile.displayNameHint', { username: me.username })}
    </p>

    <Input
      label={$t('profile.status')}
      placeholder={$t('profile.statusPlaceholder')}
      maxlength={128}
      bind:value={status}
    />

    <Input label={$t('profile.pronouns')} placeholder={$t('profile.pronounsPlaceholder')} maxlength={32} bind:value={pronouns} />

    <div class="space-y-1.5">
      <label for="bio" class="block text-label font-medium text-muted">{$t('profile.bio')}</label>
      <textarea
        id="bio"
        bind:value={bio}
        maxlength={512}
        rows={3}
        placeholder={$t('profile.bioPlaceholder')}
        class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
      ></textarea>
    </div>

    <div class="space-y-1.5">
      <span class="block text-label font-medium text-muted">{$t('profile.links')}</span>
      {#each links as link, i (i)}
        <div class="flex items-center gap-2">
          <input
            bind:value={link.label}
            placeholder={$t('profile.linkLabel')}
            maxlength={48}
            class="h-9 w-32 shrink-0 rounded border border-border bg-base/50 px-2.5 text-label text-content outline-none focus:border-primary"
          />
          <input
            bind:value={link.url}
            placeholder="https://…"
            maxlength={256}
            class="h-9 min-w-0 flex-1 rounded border border-border bg-base/50 px-2.5 text-label text-content outline-none focus:border-primary"
          />
          <button
            type="button"
            title={$t('profile.removeLink')}
            onclick={() => (links = links.filter((_, j) => j !== i))}
            class="grid size-8 shrink-0 place-items-center rounded text-muted transition-colors hover:text-danger"
          >
            <Trash2 size={15} />
          </button>
        </div>
      {/each}
      {#if links.length < 5}
        <button
          type="button"
          onclick={() => (links = [...links, { label: '', url: '' }])}
          class="flex items-center gap-1.5 rounded px-2 py-1 text-label font-medium text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          <Plus size={14} /> {$t('profile.addLink')}
        </button>
      {/if}
    </div>

    {#if err}
      <p class="text-label text-danger">{err}</p>
    {/if}

    <div class="flex items-center gap-3 pt-1">
      <Button type="submit" loading={busy}>{$t('common.save')}</Button>
      {#if saved}
        <span class="flex items-center gap-1.5 text-label text-success">
          <Check size={16} /> {$t('common.saved')}
        </span>
      {/if}
    </div>
  </form>

  <p class="mt-8 text-label text-muted">
    Nom d'utilisateur et email ne sont pas modifiables (identifiant de connexion).
  </p>
{:else}
  <p class="text-body text-danger">{err}</p>
{/if}

<ImageCropper
  open={cropOpen}
  file={cropFile}
  busy={avatarBusy}
  title="Recadrer l'avatar"
  aspect={1}
  shape="circle"
  onclose={() => (cropOpen = false)}
  oncropped={onCropped}
/>

<ImageCropper
  open={bannerCropOpen}
  file={bannerCropFile}
  busy={bannerBusy}
  title="Recadrer la bannière"
  aspect={3}
  shape="rect"
  outWidth={1200}
  onclose={() => (bannerCropOpen = false)}
  oncropped={onBannerCropped}
/>
