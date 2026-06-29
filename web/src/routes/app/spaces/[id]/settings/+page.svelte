<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { ArrowLeft, Upload, Check, AlertTriangle, Shield, Plus, Trash2, ImagePlus } from '@lucide/svelte';
  import { api, authedObjectURL, ApiError } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import {
    spaces,
    loadSpaces,
    updateSpaceSettings,
    setVanity,
    transferOwnership,
    deleteSpaceSecure,
    type Space
  } from '$lib/stores/spaces';
  import { loadMembers, displayName, type Member } from '$lib/stores/members';
  import {
    getListing,
    listSpace,
    delistSpace,
    CATEGORIES as DISCOVERY_CATEGORIES
  } from '$lib/stores/discovery';
  import { loadRoles, type Role } from '$lib/stores/roles';
  import {
    getJoinForm,
    saveJoinForm,
    listJoinRequests,
    reviewJoinRequest,
    type JoinQuestion,
    type JoinRequest
  } from '$lib/stores/joingate';
  import {
    emojisBySpace,
    loadEmojis,
    uploadEmoji,
    deleteEmoji,
    emojiUrl,
    type CustomEmoji
  } from '$lib/stores/emojis';
  import {
    stickersBySpace,
    loadStickers,
    uploadSticker,
    deleteSticker,
    stickerUrl,
    type CustomSticker
  } from '$lib/stores/stickers';
  import { Button, Input, CopyId } from '$lib/ui';
  import { prefs } from '$lib/stores/prefs';
  import Modal from '$lib/components/Modal.svelte';
  import ImageCropper from '$lib/components/ImageCropper.svelte';

  const spaceId = $derived(page.params.id ?? '');
  const space = $derived<Space | undefined>($spaces.data.find((s) => s.id === spaceId));
  const isOwner = $derived(!!space && space.owner_id === $auth.user?.id);

  let tab = $state<'general' | 'vanity' | 'entry' | 'emojis' | 'stickers' | 'danger'>('general');

  let name = $state('');
  let description = $state('');
  let rules = $state('');
  let language = $state('');
  let tagsInput = $state('');
  let bannerKey = $state<string | null>(null);
  let bannerUrl = $state<string | null>(null);
  let iconKey = $state<string | null>(null);
  let genBusy = $state(false);
  let genSaved = $state(false);
  let genErr = $state<string | null>(null);

  let hydrated = $state(false);
  $effect(() => {
    if (space && !hydrated) {
      hydrated = true;
      name = space.name;
      description = space.description ?? '';
      rules = space.rules ?? '';
      language = space.language ?? '';
      tagsInput = (space.tags ?? []).join(', ');
      bannerKey = space.banner_key ?? null;
      iconKey = space.icon_key ?? null;
      vanity = space.vanity_slug ?? '';
      if (space.banner_key) {
        void authedObjectURL(`/api/files/${space.banner_key}`)
          .then((u) => (bannerUrl = u))
          .catch(() => {});
      }
    }
  });

  let triedLoad = false;
  $effect(() => {
    if (!triedLoad && !$spaces.data.length) {
      triedLoad = true;
      void loadSpaces();
    }
  });

  async function uploadFile(file: Blob, kind: 'banner' | 'icon', name: string): Promise<string> {
    const form = new FormData();
    form.append('file', new File([file], name, { type: file.type || 'image/png' }));
    const dto = await api<{ id: string }>(`/api/files?kind=${kind}`, { method: 'POST', body: form });
    return dto.id;
  }

  let bannerCropFile = $state<File | null>(null);
  let bannerCropOpen = $state(false);
  let bannerBusy = $state(false);
  function onBannerPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !file.type.startsWith('image/')) return;
    bannerCropFile = file;
    bannerCropOpen = true;
  }
  async function onBannerCropped(blob: Blob) {
    genErr = null;
    bannerBusy = true;
    try {
      bannerKey = await uploadFile(blob, 'banner', 'banner.png');
      if (bannerUrl) URL.revokeObjectURL(bannerUrl);
      bannerUrl = await authedObjectURL(`/api/files/${bannerKey}`);
      bannerCropOpen = false;
    } catch {
      genErr = 'Échec de l’upload de la bannière';
    } finally {
      bannerBusy = false;
    }
  }

  const iconIsEmoji = $derived(!!iconKey && /^:[a-z0-9_]{2,32}:$/.test(iconKey));
  const iconIsImage = $derived(!!iconKey && !iconIsEmoji);
  let iconImgUrl = $state<string | null>(null);
  $effect(() => {
    const k = iconKey;
    if (!k || /^:[a-z0-9_]{2,32}:$/.test(k) || /^https?:\/\//.test(k)) {
      iconImgUrl = null;
      return;
    }
    let cancelled = false;
    let created: string | null = null;
    void authedObjectURL(`/api/files/${k}`)
      .then((u) => {
        if (cancelled) {
          URL.revokeObjectURL(u);
          return;
        }
        created = u;
        iconImgUrl = u;
      })
      .catch(() => {});
    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
    };
  });
  let iconCropFile = $state<File | null>(null);
  let iconCropOpen = $state(false);
  let iconBusy = $state(false);
  function onIconPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !file.type.startsWith('image/')) return;
    iconCropFile = file;
    iconCropOpen = true;
  }
  async function setIcon(key: string | null) {
    iconKey = key;
    genErr = null;
    iconBusy = true;
    try {
      await updateSpaceSettings(spaceId, { icon_key: key });
    } catch {
      genErr = "Échec de l'enregistrement de l'icône";
    } finally {
      iconBusy = false;
    }
  }

  async function onIconCropped(blob: Blob) {
    genErr = null;
    iconBusy = true;
    try {
      const key = await uploadFile(blob, 'icon', 'icon.png');
      iconCropOpen = false;
      await setIcon(key);
    } catch {
      genErr = 'Échec de l’upload de l’icône';
    } finally {
      iconBusy = false;
    }
  }

  async function saveGeneral(e: Event) {
    e.preventDefault();
    genBusy = true;
    genErr = null;
    genSaved = false;
    try {
      const tags = tagsInput
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
        .slice(0, 8);
      await updateSpaceSettings(spaceId, {
        name: name.trim(),
        description: description.trim() || null,
        rules: rules.trim() || null,
        language: language.trim() || null,
        tags,
        banner_key: bannerKey,
        icon_key: iconKey
      });
      genSaved = true;
      setTimeout(() => (genSaved = false), 1500);
    } catch (err) {
      genErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      genBusy = false;
    }
  }

  let vanity = $state('');
  let vanityBusy = $state(false);
  let vanityErr = $state<string | null>(null);
  let vanitySaved = $state(false);

  async function saveVanity(e: Event) {
    e.preventDefault();
    vanityBusy = true;
    vanityErr = null;
    vanitySaved = false;
    try {
      await setVanity(spaceId, vanity.trim() || null);
      vanitySaved = true;
      setTimeout(() => (vanitySaved = false), 1500);
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) vanityErr = 'Cette adresse est déjà prise.';
      else vanityErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      vanityBusy = false;
    }
  }

  let listingActive = $state(false);
  let listCategory = $state('gaming');
  let listBusy = $state(false);
  let listingLoaded = false;
  $effect(() => {
    if (tab === 'vanity' && spaceId && !listingLoaded) {
      listingLoaded = true;
      void getListing(spaceId).then((s) => {
        listingActive = s.listed;
        if (s.category) listCategory = s.category;
      });
    }
  });
  async function doList() {
    listBusy = true;
    try {
      const s = await listSpace(spaceId, listCategory);
      listingActive = s.listed;
    } finally {
      listBusy = false;
    }
  }
  async function doDelist() {
    await delistSpace(spaceId);
    listingActive = false;
  }

  let gateEnabled = $state(false);
  let gateQuestions = $state<JoinQuestion[]>([]);
  let gateAutoRole = $state<string>('');
  let gateMinKarma = $state(0);
  let gateRoles = $state<Role[]>([]);
  let gateBusy = $state(false);
  let gateSaved = $state(false);
  let gateErr = $state<string | null>(null);
  let queue = $state<JoinRequest[]>([]);
  let queueBusy = $state(false);
  let gateLoaded = false;

  $effect(() => {
    if (tab === 'entry' && spaceId && !gateLoaded) {
      gateLoaded = true;
      void getJoinForm(spaceId).then((f) => {
        gateEnabled = f.enabled;
        gateQuestions = f.questions ?? [];
        gateAutoRole = f.auto_role_id ?? '';
        gateMinKarma = f.min_karma ?? 0;
      });
      void loadRoles(spaceId).then((r) => (gateRoles = r.filter((x) => !x.is_everyone)));
      void refreshQueue();
    }
  });

  async function refreshQueue() {
    queueBusy = true;
    try {
      queue = await listJoinRequests(spaceId, 'pending');
    } finally {
      queueBusy = false;
    }
  }

  function addQuestion() {
    gateQuestions = [...gateQuestions, { id: `q${gateQuestions.length + 1}`, label: '', required: true }];
  }
  function removeQuestion(i: number) {
    gateQuestions = gateQuestions.filter((_, idx) => idx !== i);
  }

  let emojisLoaded = false;
  let emojiName = $state('');
  let emojiFile = $state<File | null>(null);
  let emojiInput = $state<HTMLInputElement | null>(null);
  let emojiBusy = $state(false);
  let emojiErr = $state<string | null>(null);
  let emojiThumbs = $state<Record<string, string>>({});

  $effect(() => {
    if ((tab === 'emojis' || tab === 'general') && spaceId && !emojisLoaded) {
      emojisLoaded = true;
      void loadEmojis(spaceId).then((list) => {
        for (const e of list) void thumb(e);
      });
    }
  });

  const iconEmojis = $derived(spaceId ? ($emojisBySpace[spaceId] ?? []) : []);

  async function thumb(e: CustomEmoji) {
    try {
      const url = await emojiUrl(e.file_key);
      emojiThumbs = { ...emojiThumbs, [e.file_key]: url };
    } catch {
    }
  }

  function onPickEmoji(ev: Event) {
    const input = ev.currentTarget as HTMLInputElement;
    const f = input.files?.[0] ?? null;
    emojiErr = null;
    if (f && f.size > 5 * 1024 * 1024) {
      emojiErr = 'Image trop lourde (max 5 Mo).';
      input.value = '';
      return;
    }
    emojiFile = f;
    if (f && !emojiName) {
      emojiName = f.name.replace(/\.[^.]+$/, '').toLowerCase().replace(/[^a-z0-9_]/g, '_').slice(0, 32);
    }
  }

  async function submitEmoji() {
    const name = emojiName.trim().toLowerCase();
    if (!emojiFile || !/^[a-z0-9_]{2,32}$/.test(name)) {
      emojiErr = 'Nom 2-32 caractères (a-z, 0-9, _) et une image requis.';
      return;
    }
    emojiBusy = true;
    emojiErr = null;
    try {
      const e = await uploadEmoji(spaceId, name, emojiFile);
      void thumb(e);
      emojiName = '';
      emojiFile = null;
      if (emojiInput) emojiInput.value = '';
    } catch (err) {
      emojiErr = err instanceof ApiError ? err.message : 'Ajout impossible';
    } finally {
      emojiBusy = false;
    }
  }

  async function removeEmoji(e: CustomEmoji) {
    try {
      await deleteEmoji(spaceId, e.id);
    } catch {
    }
  }

  let stickersLoaded = false;
  let stickerName = $state('');
  let stickerFile = $state<File | null>(null);
  let stickerInput = $state<HTMLInputElement | null>(null);
  let stickerBusy = $state(false);
  let stickerErr = $state<string | null>(null);
  let stickerThumbs = $state<Record<string, string>>({});

  $effect(() => {
    if (tab === 'stickers' && spaceId && !stickersLoaded) {
      stickersLoaded = true;
      void loadStickers(spaceId).then((list) => {
        for (const e of list) void stickerThumb(e);
      });
    }
  });

  async function stickerThumb(e: CustomSticker) {
    try {
      const url = await stickerUrl(e.file_key);
      stickerThumbs = { ...stickerThumbs, [e.file_key]: url };
    } catch {
    }
  }

  function onPickSticker(ev: Event) {
    const input = ev.currentTarget as HTMLInputElement;
    const f = input.files?.[0] ?? null;
    stickerErr = null;
    if (f && f.size > 10 * 1024 * 1024) {
      stickerErr = 'Image trop lourde (max 10 Mo).';
      input.value = '';
      return;
    }
    stickerFile = f;
    if (f && !stickerName) {
      stickerName = f.name.replace(/\.[^.]+$/, '').slice(0, 40);
    }
  }

  async function submitSticker() {
    const name = stickerName.trim();
    if (!stickerFile || !/^[\p{L}\p{N} _-]{1,40}$/u.test(name)) {
      stickerErr = 'Nom 1-40 caractères et une image requis.';
      return;
    }
    stickerBusy = true;
    stickerErr = null;
    try {
      const e = await uploadSticker(spaceId, name, stickerFile);
      void stickerThumb(e);
      stickerName = '';
      stickerFile = null;
      if (stickerInput) stickerInput.value = '';
    } catch (err) {
      stickerErr = err instanceof ApiError ? err.message : 'Ajout impossible';
    } finally {
      stickerBusy = false;
    }
  }

  async function removeSticker(e: CustomSticker) {
    try {
      await deleteSticker(spaceId, e.id);
    } catch {
    }
  }

  async function saveGate() {
    gateBusy = true;
    gateErr = null;
    gateSaved = false;
    try {
      const cleaned = gateQuestions
        .map((q) => ({ ...q, label: q.label.trim() }))
        .filter((q) => q.label);
      await saveJoinForm(spaceId, {
        enabled: gateEnabled,
        questions: cleaned,
        auto_role_id: gateAutoRole || null,
        min_karma: Math.max(0, Math.floor(gateMinKarma || 0))
      });
      gateQuestions = cleaned;
      gateSaved = true;
      setTimeout(() => (gateSaved = false), 1500);
    } catch (err) {
      gateErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      gateBusy = false;
    }
  }

  async function decide(req: JoinRequest, action: 'approve' | 'reject') {
    await reviewJoinRequest(req.id, action);
    queue = queue.filter((q) => q.id !== req.id);
  }

  let members = $state<Member[]>([]);
  let transferTo = $state('');
  let transferBusy = $state(false);
  let transferErr = $state<string | null>(null);
  let confirmTransfer = $state(false);

  $effect(() => {
    if (tab === 'danger' && spaceId && members.length === 0) {
      void loadMembers(spaceId).then((m) => (members = m.filter((x) => x.user_id !== space?.owner_id)));
    }
  });

  async function doTransfer() {
    transferBusy = true;
    transferErr = null;
    try {
      await transferOwnership(spaceId, members.find((m) => m.id === transferTo)!.user_id);
      confirmTransfer = false;
    } catch (err) {
      transferErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      transferBusy = false;
    }
  }

  let deleteOpen = $state(false);
  let deletePassword = $state('');
  let deleteBusy = $state(false);
  let deleteErr = $state<string | null>(null);

  async function doDelete() {
    deleteBusy = true;
    deleteErr = null;
    try {
      await deleteSpaceSecure(spaceId, deletePassword);
      deleteOpen = false;
      await goto('/app');
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) deleteErr = 'Mot de passe incorrect.';
      else deleteErr = err instanceof Error ? err.message : 'Échec';
    } finally {
      deleteBusy = false;
    }
  }
</script>

<div class="mx-auto max-w-3xl px-6 py-8">
  <a
    href={`/app/spaces/${spaceId}`}
    class="mb-4 inline-flex items-center gap-1.5 text-label text-muted transition-colors duration-150 hover:text-content"
  >
    <ArrowLeft size={15} /> Retour à l'espace
  </a>
  <div class="flex flex-wrap items-center justify-between gap-2">
    <h1 class="text-title font-bold">Paramètres de l'espace</h1>
    <a
      href={`/app/spaces/${spaceId}/roles`}
      class="inline-flex items-center gap-1.5 rounded border border-border px-3 py-1.5 text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content"
    >
      <Shield size={15} /> Gérer les rôles
    </a>
  </div>
  {#if $prefs.developerMode}
    <div class="mt-2"><CopyId id={spaceId} label="Space ID" /></div>
  {/if}

  {#if !space}
    <p class="mt-6 text-body text-muted">Chargement…</p>
  {:else}
    <nav class="no-scrollbar mt-6 flex gap-1 overflow-x-auto border-b border-border">
      {#each [{ k: 'general', l: 'Général' }, { k: 'vanity', l: 'Adresse vanity' }, { k: 'entry', l: 'Entrée' }, { k: 'emojis', l: 'Emojis' }, { k: 'stickers', l: 'Stickers' }, { k: 'danger', l: 'Zone de danger' }] as t (t.k)}
        <button
          type="button"
          onclick={() => (tab = t.k as typeof tab)}
          class="-mb-px shrink-0 whitespace-nowrap border-b-2 px-3 py-2.5 text-body transition-colors duration-150
                 {tab === t.k
            ? 'border-brand text-content'
            : 'border-transparent text-muted hover:text-content'}"
        >
          {t.l}
        </button>
      {/each}
    </nav>

    <div class="mt-6">
      {#if tab === 'general'}
        <form onsubmit={saveGeneral} class="space-y-5">
          <div class="space-y-1.5">
            <span class="block text-label font-medium text-muted">Bannière</span>
            <div
              class="relative flex h-32 items-end overflow-hidden rounded-lg border border-border bg-elevated/40"
              style={bannerUrl ? `background-image:url(${bannerUrl});background-size:cover;background-position:center` : ''}
            >
              <label
                class="m-3 flex cursor-pointer items-center gap-1.5 rounded-md bg-base/80 px-3 py-1.5 text-label text-content backdrop-blur transition-colors duration-150 hover:bg-base"
              >
                <Upload size={14} /> Changer
                <input type="file" accept="image/*" class="sr-only" onchange={onBannerPick} />
              </label>
            </div>
          </div>

          <Input label="Nom de l'espace" bind:value={name} required minlength={1} maxlength={64} />

          <div class="space-y-1.5">
            <span class="block text-label font-medium text-muted">Icône</span>
            <div class="flex flex-wrap items-center gap-1">
              <button
                type="button"
                onclick={() => setIcon(null)}
                disabled={iconBusy}
                title="Aucune (initiales)"
                aria-pressed={!iconKey}
                class="grid size-9 place-items-center rounded-lg border text-label font-semibold transition-colors duration-150
                       {!iconKey
                  ? 'border-primary bg-primary/10 text-content'
                  : 'border-border text-muted hover:border-border-strong'}"
              >
                {(name || 'E').trim().slice(0, 2).toUpperCase()}
              </button>

              <label
                title="Image personnalisée"
                class="grid size-9 cursor-pointer place-items-center overflow-hidden rounded-lg border transition-colors duration-150
                       {iconIsImage
                  ? 'border-primary bg-primary/10'
                  : 'border-dashed border-border text-muted hover:border-border-strong hover:text-content'}"
              >
                <input type="file" accept="image/*" class="sr-only" onchange={onIconPick} />
                {#if iconIsImage && iconImgUrl}
                  <img src={iconImgUrl} alt="" class="size-full object-cover" />
                {:else}
                  <ImagePlus size={16} />
                {/if}
              </label>

              {#each iconEmojis as e (e.id)}
                {@const token = ':' + e.name + ':'}
                <button
                  type="button"
                  onclick={() => setIcon(token)}
                  disabled={iconBusy}
                  title={token}
                  aria-pressed={iconKey === token}
                  class="grid size-9 place-items-center rounded-lg border p-1.5 transition-colors duration-150
                         {iconKey === token
                    ? 'border-primary bg-primary/10'
                    : 'border-transparent hover:bg-elevated'}"
                >
                  {#if emojiThumbs[e.file_key]}
                    <img src={emojiThumbs[e.file_key]} alt={e.name} class="size-full object-contain" />
                  {:else}
                    <span class="size-full animate-pulse rounded bg-elevated"></span>
                  {/if}
                </button>
              {/each}
            </div>
            {#if !iconEmojis.length}
              <p class="text-label text-muted">
                Ajoute des emojis dans l'onglet <strong>Emojis</strong> pour les utiliser aussi comme icône.
              </p>
            {/if}
          </div>

          <div class="space-y-1.5">
            <label for="sp-desc" class="block text-label font-medium text-muted">Description</label>
            <textarea
              id="sp-desc"
              bind:value={description}
              maxlength={2000}
              rows={2}
              placeholder="Présente ton espace…"
              class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
            ></textarea>
          </div>

          <div class="grid gap-4 sm:grid-cols-2">
            <Input label="Tags (séparés par des virgules)" bind:value={tagsInput} placeholder="gaming, fr, chill" />
            <Input label="Langue" bind:value={language} placeholder="fr" maxlength={8} />
          </div>

          <div class="space-y-1.5">
            <label for="sp-rules" class="block text-label font-medium text-muted">Règles</label>
            <textarea
              id="sp-rules"
              bind:value={rules}
              maxlength={4000}
              rows={3}
              placeholder="Les règles de la communauté…"
              class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
            ></textarea>
          </div>

          {#if genErr}<p class="text-label text-danger">{genErr}</p>{/if}
          <Button type="submit" loading={genBusy}>
            {#if genSaved}<Check size={16} /> Enregistré{:else}Enregistrer{/if}
          </Button>
        </form>
      {:else if tab === 'vanity'}
        <form onsubmit={saveVanity} class="max-w-md space-y-4">
          <p class="text-body text-muted">
            Donne à ton espace une adresse mémorable, partageable publiquement.
          </p>
          <div class="space-y-1.5">
            <span class="block text-label font-medium text-muted">Adresse</span>
            <div class="flex items-center gap-1.5">
              <span class="shrink-0 text-body text-muted">krovara.space/</span>
              <input
                bind:value={vanity}
                placeholder="mon-espace"
                maxlength={32}
                class="h-10 flex-1 rounded border border-border bg-base/50 px-3 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
              />
            </div>
            <p class="text-label text-muted">3 à 32 caractères : minuscules, chiffres, tirets.</p>
          </div>
          {#if vanityErr}<p class="text-label text-danger">{vanityErr}</p>{/if}
          <Button type="submit" loading={vanityBusy}>
            {#if vanitySaved}<Check size={16} /> Enregistré{:else}Réserver{/if}
          </Button>
        </form>

        <div class="mt-8 max-w-md border-t border-border pt-6">
          <h2 class="text-body font-semibold text-content">Visibilité dans l'Explorer</h2>
          <p class="mt-1 text-label text-muted">
            Liste publiquement ton espace pour qu'il soit découvrable dans l'Explorer.
          </p>
          <div class="mt-3 flex flex-wrap items-end gap-2">
            <div class="space-y-1">
              <span class="block text-label font-medium text-muted">Catégorie</span>
              <select
                bind:value={listCategory}
                class="h-10 rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              >
                {#each DISCOVERY_CATEGORIES as c (c)}
                  <option value={c}>{c}</option>
                {/each}
              </select>
            </div>
            {#if listingActive}
              <Button type="button" variant="ghost" onclick={doDelist}>Retirer de l'Explorer</Button>
            {/if}
            <Button type="button" loading={listBusy} onclick={doList}>
              {listingActive ? 'Mettre à jour' : 'Lister'}
            </Button>
          </div>
          {#if listingActive}
            <p class="mt-2 flex items-center gap-1.5 text-label text-success"><Check size={14} /> Listé publiquement</p>
          {/if}
        </div>
      {:else if tab === 'entry'}
        <div class="space-y-8">
          <section class="max-w-xl space-y-4">
            <div>
              <h2 class="text-body font-semibold text-content">Porte d'entrée</h2>
              <p class="mt-1 text-label text-muted">
                Exige une demande d'adhésion validée par un modérateur. Les invitations
                directes continuent de fonctionner.
              </p>
            </div>

            <label class="flex items-center gap-2.5">
              <input
                type="checkbox"
                bind:checked={gateEnabled}
                class="h-4 w-4 rounded border-border bg-base/50 accent-primary"
              />
              <span class="text-body text-content">Activer la porte d'entrée</span>
            </label>

            <div class="space-y-2.5">
              <span class="block text-label font-medium text-muted">Questions</span>
              {#each gateQuestions as q, i (i)}
                <div class="flex items-center gap-2">
                  <Input bind:value={q.label} placeholder="Pourquoi veux-tu rejoindre ?" maxlength={200} />
                  <label class="flex shrink-0 items-center gap-1.5 text-label text-muted">
                    <input type="checkbox" bind:checked={q.required} class="h-4 w-4 rounded border-border bg-base/50 accent-primary" />
                    requis
                  </label>
                  <button
                    type="button"
                    onclick={() => removeQuestion(i)}
                    aria-label="Supprimer la question"
                    class="shrink-0 rounded p-1.5 text-muted transition-colors duration-150 hover:bg-elevated hover:text-danger"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              {/each}
              <Button type="button" variant="ghost" onclick={addQuestion}>
                <Plus size={15} /> Ajouter une question
              </Button>
            </div>

            <div class="space-y-1.5">
              <span class="block text-label font-medium text-muted">Rôle attribué à l'approbation</span>
              <select
                bind:value={gateAutoRole}
                class="h-10 w-full rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              >
                <option value="">Aucun</option>
                {#each gateRoles as r (r.id)}
                  <option value={r.id}>{r.name}</option>
                {/each}
              </select>
            </div>

            <div class="space-y-1.5">
              <label for="gate-karma" class="block text-label font-medium text-muted">
                Karma minimum requis
              </label>
              <input
                id="gate-karma"
                type="number"
                min="0"
                bind:value={gateMinKarma}
                class="h-10 w-32 rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              />
              <p class="text-label text-muted">
                0 = aucun seuil. Basé sur le karma global du candidat (tous espaces confondus).
              </p>
            </div>

            {#if gateErr}<p class="text-label text-danger">{gateErr}</p>{/if}
            <Button type="button" loading={gateBusy} onclick={saveGate}>
              {#if gateSaved}<Check size={16} /> Enregistré{:else}Enregistrer{/if}
            </Button>
          </section>

          <section class="border-t border-border pt-6">
            <h2 class="text-body font-semibold text-content">
              Demandes en attente
              {#if queue.length}<span class="ml-1 text-muted">({queue.length})</span>{/if}
            </h2>
            {#if queueBusy}
              <p class="mt-3 text-label text-muted">Chargement…</p>
            {:else if queue.length === 0}
              <p class="mt-3 text-label text-muted">Aucune demande en attente.</p>
            {:else}
              <ul class="mt-3 space-y-2.5">
                {#each queue as req (req.id)}
                  <li class="rounded-lg border border-border bg-surface p-3.5">
                    <div class="flex items-start justify-between gap-3">
                      <div class="min-w-0">
                        <p class="text-body font-medium text-content">{req.display_name || req.username}</p>
                        <p class="text-label text-muted">@{req.username}</p>
                      </div>
                      <div class="flex shrink-0 gap-2">
                        <Button type="button" variant="ghost" onclick={() => decide(req, 'reject')}>Refuser</Button>
                        <Button type="button" onclick={() => decide(req, 'approve')}>Approuver</Button>
                      </div>
                    </div>
                    {#if req.answers?.length}
                      <dl class="mt-2.5 space-y-1.5 border-t border-border pt-2.5">
                        {#each req.answers as a (a.question_id)}
                          <div>
                            <dt class="text-label text-muted">
                              {gateQuestions.find((q) => q.id === a.question_id)?.label ?? a.question_id}
                            </dt>
                            <dd class="text-body text-content">{a.answer}</dd>
                          </div>
                        {/each}
                      </dl>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        </div>
      {:else if tab === 'emojis'}
        <div class="space-y-5">
          <p class="text-body text-muted">
            Emojis personnalisés de l'espace, utilisables avec <code class="rounded bg-base px-1 py-0.5 text-accent">:nom:</code>.
            PNG, JPG, GIF ou WebP, 5 Mo max.
          </p>
          <div class="flex flex-wrap items-end gap-2 rounded-lg border border-border p-3">
            <label
              class="grid size-12 shrink-0 cursor-pointer place-items-center rounded-lg border border-dashed border-border-strong text-muted transition-colors hover:border-primary hover:text-content"
              title="Choisir une image"
            >
              <input bind:this={emojiInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" class="sr-only" onchange={onPickEmoji} />
              {#if emojiFile}<Check size={18} class="text-success" />{:else}<Upload size={18} />{/if}
            </label>
            <label class="flex-1">
              <span class="mb-1 block text-label text-muted">Nom (a-z, 0-9, _)</span>
              <Input bind:value={emojiName} placeholder="blobcat" maxlength={32} />
            </label>
            <Button type="button" loading={emojiBusy} disabled={!emojiFile || !emojiName.trim()} onclick={submitEmoji}>
              <Plus size={15} /> Ajouter
            </Button>
          </div>
          {#if emojiErr}<p class="text-label text-danger">{emojiErr}</p>{/if}

          {#if ($emojisBySpace[spaceId] ?? []).length === 0}
            <p class="rounded-lg border border-border py-10 text-center text-body text-muted">Aucun emoji. Ajoute le premier !</p>
          {:else}
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {#each $emojisBySpace[spaceId] as e (e.id)}
                <div class="group flex items-center gap-2 rounded-lg border border-border p-2">
                  {#if emojiThumbs[e.file_key]}
                    <img src={emojiThumbs[e.file_key]} alt={e.name} class="size-8 shrink-0 rounded object-contain" />
                  {:else}
                    <span class="size-8 shrink-0 animate-pulse rounded bg-elevated"></span>
                  {/if}
                  <code class="min-w-0 flex-1 truncate text-label text-content">:{e.name}:</code>
                  <button
                    type="button"
                    title="Supprimer"
                    onclick={() => removeEmoji(e)}
                    class="grid size-7 shrink-0 place-items-center rounded text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {:else if tab === 'stickers'}
        <div class="space-y-5">
          <p class="text-body text-muted">
            Stickers personnalisés de l'espace, envoyés en un clic dans le chat.
            PNG, JPG, GIF ou WebP, 10 Mo max.
          </p>
          <div class="flex flex-wrap items-end gap-2 rounded-lg border border-border p-3">
            <label
              class="grid size-12 shrink-0 cursor-pointer place-items-center rounded-lg border border-dashed border-border-strong text-muted transition-colors hover:border-primary hover:text-content"
              title="Choisir une image"
            >
              <input bind:this={stickerInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" class="sr-only" onchange={onPickSticker} />
              {#if stickerFile}<Check size={18} class="text-success" />{:else}<Upload size={18} />{/if}
            </label>
            <label class="flex-1">
              <span class="mb-1 block text-label text-muted">Nom</span>
              <Input bind:value={stickerName} placeholder="Salut" maxlength={40} />
            </label>
            <Button type="button" loading={stickerBusy} disabled={!stickerFile || !stickerName.trim()} onclick={submitSticker}>
              <Plus size={15} /> Ajouter
            </Button>
          </div>
          {#if stickerErr}<p class="text-label text-danger">{stickerErr}</p>{/if}

          {#if ($stickersBySpace[spaceId] ?? []).length === 0}
            <p class="rounded-lg border border-border py-10 text-center text-body text-muted">Aucun sticker. Ajoute le premier !</p>
          {:else}
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {#each $stickersBySpace[spaceId] as e (e.id)}
                <div class="group flex items-center gap-2 rounded-lg border border-border p-2">
                  {#if stickerThumbs[e.file_key]}
                    <img src={stickerThumbs[e.file_key]} alt={e.name} class="size-12 shrink-0 rounded object-contain" />
                  {:else}
                    <span class="size-12 shrink-0 animate-pulse rounded bg-elevated"></span>
                  {/if}
                  <span class="min-w-0 flex-1 truncate text-label text-content">{e.name}</span>
                  <button
                    type="button"
                    title="Supprimer"
                    onclick={() => removeSticker(e)}
                    class="grid size-7 shrink-0 place-items-center rounded text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {:else}
        <div class="space-y-6">
          {#if isOwner}
            <section class="rounded-lg border border-border p-4">
              <h2 class="text-body font-semibold text-content">Transférer la propriété</h2>
              <p class="mt-1 text-label text-muted">
                Le nouveau propriétaire aura le contrôle total. Tu perdras tes droits de propriétaire.
              </p>
              <div class="mt-3 flex flex-wrap items-center gap-2">
                <select
                  bind:value={transferTo}
                  class="h-10 rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
                >
                  <option value="">Choisir un membre…</option>
                  {#each members as m (m.id)}
                    <option value={m.id}>{displayName(m)}</option>
                  {/each}
                </select>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={!transferTo}
                  onclick={() => (confirmTransfer = true)}
                >
                  Transférer
                </Button>
              </div>
            </section>
          {/if}

          <section class="rounded-lg border border-danger/40 bg-danger/5 p-4">
            <h2 class="flex items-center gap-1.5 text-body font-semibold text-danger">
              <AlertTriangle size={16} /> Supprimer l'espace
            </h2>
            <p class="mt-1 text-label text-muted">
              Action définitive : salons, messages, rôles et membres seront supprimés. Ton mot de
              passe sera demandé pour confirmer.
            </p>
            <Button type="button" variant="danger" class="mt-3" onclick={() => (deleteOpen = true)}>
              Supprimer cet espace
            </Button>
          </section>
        </div>
      {/if}
    </div>
  {/if}
</div>

<Modal open={confirmTransfer} title="Confirmer le transfert" onclose={() => (confirmTransfer = false)}>
  <p class="text-body text-muted">
    Confirmer le transfert de propriété ? Cette action est immédiate.
  </p>
  {#if transferErr}<p class="mt-2 text-label text-danger">{transferErr}</p>{/if}
  <div class="mt-4 flex justify-end gap-2">
    <Button type="button" variant="ghost" onclick={() => (confirmTransfer = false)}>Annuler</Button>
    <Button type="button" loading={transferBusy} onclick={doTransfer}>Transférer</Button>
  </div>
</Modal>

<Modal open={deleteOpen} title="Supprimer l'espace" onclose={() => (deleteOpen = false)}>
  <p class="text-body text-muted">
    Entre ton mot de passe pour confirmer la suppression définitive de
    <strong class="text-content">{space?.name}</strong>.
  </p>
  <form
    onsubmit={(e) => {
      e.preventDefault();
      void doDelete();
    }}
    class="mt-4 space-y-3"
  >
    <Input type="password" placeholder="Mot de passe" bind:value={deletePassword} required />
    {#if deleteErr}<p class="text-label text-danger">{deleteErr}</p>{/if}
    <div class="flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={() => (deleteOpen = false)}>Annuler</Button>
      <Button type="submit" variant="danger" loading={deleteBusy} disabled={!deletePassword}>
        Supprimer définitivement
      </Button>
    </div>
  </form>
</Modal>

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

<ImageCropper
  open={iconCropOpen}
  file={iconCropFile}
  busy={iconBusy}
  title="Recadrer l'icône"
  aspect={1}
  shape="rounded"
  outWidth={256}
  onclose={() => (iconCropOpen = false)}
  oncropped={onIconCropped}
/>
