<script lang="ts">
  import { page } from '$app/state';
  import { ArrowLeft, Upload, Check } from '@lucide/svelte';
  import { api, authedObjectURL } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import { loadMembers, updateMySpaceProfile, type Member } from '$lib/stores/members';
  import { spaces, loadSpaces, type Space } from '$lib/stores/spaces';
  import { Button, Input } from '$lib/ui';

  const spaceId = $derived(page.params.id ?? '');
  const space = $derived<Space | undefined>($spaces.data.find((s) => s.id === spaceId));

  let nickname = $state('');
  let bio = $state('');
  let avatarKey = $state<string | null>(null);
  let avatarUrl = $state<string | null>(null);
  let busy = $state(false);
  let saved = $state(false);
  let err = $state<string | null>(null);

  let loaded = false;
  $effect(() => {
    if (!spaceId || loaded) return;
    loaded = true;
    if (!$spaces.data.length) void loadSpaces();
    void loadMembers(spaceId).then((list) => {
      const me = list.find((m: Member) => m.user_id === $auth.user?.id);
      if (me) {
        nickname = me.nickname ?? '';
        bio = me.bio ?? '';
        avatarKey = me.avatar_key ?? null;
        if (me.avatar_key) {
          void authedObjectURL(`/api/files/${me.avatar_key}`)
            .then((u) => (avatarUrl = u))
            .catch(() => {});
        }
      }
    });
  });

  async function onAvatarPick(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    err = null;
    try {
      const form = new FormData();
      form.append('file', file);
      const dto = await api<{ id: string }>(`/api/files?kind=avatar`, { method: 'POST', body: form });
      avatarKey = dto.id;
      if (avatarUrl) URL.revokeObjectURL(avatarUrl);
      avatarUrl = await authedObjectURL(`/api/files/${avatarKey}`);
    } catch {
      err = 'Échec de l’upload de l’avatar';
    }
  }

  async function save(e: Event) {
    e.preventDefault();
    busy = true;
    err = null;
    saved = false;
    try {
      await updateMySpaceProfile(spaceId, {
        nickname: nickname.trim() || null,
        avatar_key: avatarKey,
        bio: bio.trim() || null
      });
      saved = true;
      setTimeout(() => (saved = false), 1500);
    } catch (e2) {
      err = e2 instanceof Error ? e2.message : 'Échec';
    } finally {
      busy = false;
    }
  }

  function clearAvatar() {
    if (avatarUrl) URL.revokeObjectURL(avatarUrl);
    avatarUrl = null;
    avatarKey = null;
  }
</script>

<div class="mx-auto max-w-xl px-6 py-8">
  <a
    href={`/app/spaces/${spaceId}`}
    class="mb-4 inline-flex items-center gap-1.5 text-label text-muted transition-colors duration-150 hover:text-content"
  >
    <ArrowLeft size={15} /> Retour à l'espace
  </a>
  <h1 class="text-title font-bold">Mon profil dans cet espace</h1>
  <p class="mt-1 text-body text-muted">
    Personnalise ton identité {#if space}dans <strong class="text-content">{space.name}</strong>{/if}.
    Laisse vide pour reprendre ton profil global.
  </p>

  <form onsubmit={save} class="mt-6 space-y-5">
    <div class="flex items-center gap-4">
      <div class="grid size-16 shrink-0 place-items-center overflow-hidden rounded-full bg-elevated text-subtitle font-semibold text-muted">
        {#if avatarUrl}
          <img src={avatarUrl} alt="" class="size-full object-cover" />
        {:else}
          {(nickname || $auth.user?.username || '?').slice(0, 2).toUpperCase()}
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <label
          class="inline-flex cursor-pointer items-center gap-1.5 rounded border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content"
        >
          <Upload size={14} /> Changer l'avatar
          <input type="file" accept="image/*" class="sr-only" onchange={onAvatarPick} />
        </label>
        {#if avatarKey}
          <Button type="button" variant="ghost" onclick={clearAvatar}>Retirer</Button>
        {/if}
      </div>
    </div>

    <Input label="Pseudo dans l'espace" bind:value={nickname} maxlength={32} placeholder="Ton pseudo ici" />

    <div class="space-y-1.5">
      <label for="space-bio" class="block text-label font-medium text-muted">Bio dans l'espace</label>
      <textarea
        id="space-bio"
        bind:value={bio}
        maxlength={300}
        rows={3}
        placeholder="Quelques mots sur toi, pour cet espace…"
        class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
      ></textarea>
    </div>

    {#if err}<p class="text-label text-danger">{err}</p>{/if}
    <Button type="submit" loading={busy}>
      {#if saved}<Check size={16} /> Enregistré{:else}Enregistrer{/if}
    </Button>
  </form>
</div>
