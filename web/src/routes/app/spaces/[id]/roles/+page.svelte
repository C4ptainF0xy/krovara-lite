<script lang="ts">
  import { page } from '$app/state';
  import { ArrowLeft, Plus, Trash2, Check, GripVertical, Hash, X } from '@lucide/svelte';
  import {
    rolesBySpace,
    loadRoles,
    createRole,
    updateRole,
    deleteRole,
    listRoleMembers,
    bulkAssign,
    reorderRoles,
    type Role,
    type RoleMember
  } from '$lib/stores/roles';
  import { loadMembers, displayName, type Member } from '$lib/stores/members';
  import { spaces, loadSpaces } from '$lib/stores/spaces';
  import { PERMISSION_GROUPS, hasBit } from '$lib/permissions';
  import { Button } from '$lib/ui';

  const spaceId = $derived(page.params.id ?? '');
  const roles = $derived<Role[]>($rolesBySpace[spaceId] ?? []);

  let loaded = false;
  $effect(() => {
    if (spaceId && !loaded) {
      loaded = true;
      void loadRoles(spaceId);
      if (!$spaces.data.length) void loadSpaces();
    }
  });

  let selectedId = $state<string | null>(null);
  const selected = $derived(roles.find((r) => r.id === selectedId) ?? null);
  $effect(() => {
    if (!selectedId && roles.length) selectedId = roles[0].id;
  });

  const COLORS = ['#A79FCB', '#5865F2', '#3BA55D', '#FAA61A', '#ED4245', '#EB459E', '#1ABC9C', '#E67E22'];

  let saving = $state(false);
  let saved = $state(false);
  async function patch(p: Parameters<typeof updateRole>[2]) {
    if (!selected) return;
    saving = true;
    try {
      await updateRole(spaceId, selected.id, p);
      saved = true;
      setTimeout(() => (saved = false), 1200);
    } finally {
      saving = false;
    }
  }

  function togglePerm(bit: number) {
    if (!selected) return;
    const cur = selected.permissions ?? 0;
    void patch({ permissions: hasBit(cur, bit) ? cur & ~bit : cur | bit });
  }

  let creating = $state(false);
  let newName = $state('');
  async function doCreate() {
    const name = newName.trim();
    if (!name) {
      creating = false;
      return;
    }
    const r = await createRole(spaceId, name);
    newName = '';
    creating = false;
    selectedId = r.id;
  }
  async function doDelete(r: Role) {
    if (!confirm(`Supprimer le rôle « ${r.name} » ?`)) return;
    await deleteRole(spaceId, r.id);
    if (selectedId === r.id) selectedId = null;
  }

  let dragId = $state<string | null>(null);
  function onDragStart(id: string) {
    dragId = id;
  }
  async function onDropBefore(targetId: string) {
    if (!dragId || dragId === targetId) return;
    const draggable = roles.filter((r) => !r.is_everyone);
    const ids = draggable.map((r) => r.id);
    const from = ids.indexOf(dragId);
    const to = ids.indexOf(targetId);
    if (from < 0 || to < 0) return;
    ids.splice(to, 0, ids.splice(from, 1)[0]);
    dragId = null;
    await reorderRoles(spaceId, ids);
  }

  let editTab = $state<'display' | 'perms' | 'members'>('display');

  let roleMembers = $state<RoleMember[]>([]);
  let allMembers = $state<Member[]>([]);
  let memberPicker = $state('');
  let memberDuration = $state(0);
  async function loadRoleMembersTab() {
    if (!selected) return;
    roleMembers = await listRoleMembers(selected.id);
    if (!allMembers.length) allMembers = await loadMembers(spaceId);
  }
  $effect(() => {
    if (editTab === 'members' && selected && !selected.is_everyone) void loadRoleMembersTab();
  });
  const assignable = $derived(
    allMembers.filter((m) => !roleMembers.some((rm) => rm.member_id === m.id))
  );
  async function addMemberToRole() {
    if (!selected || !memberPicker) return;
    await bulkAssign(selected.id, 'add', {
      memberIds: [memberPicker],
      expiresInSeconds: memberDuration
    });
    memberPicker = '';
    await loadRoleMembersTab();
  }
  async function removeMemberFromRole(memberId: string) {
    if (!selected) return;
    await bulkAssign(selected.id, 'remove', { memberIds: [memberId] });
    await loadRoleMembersTab();
  }
</script>

<div class="mx-auto max-w-4xl px-6 py-8">
  <a
    href={`/app/spaces/${spaceId}`}
    class="mb-4 inline-flex items-center gap-1.5 text-label text-muted transition-colors duration-150 hover:text-content"
  >
    <ArrowLeft size={15} /> Retour à l'espace
  </a>
  <h1 class="text-title font-bold">Rôles</h1>
  <p class="mt-1 text-body text-muted">
    Glisse pour réordonner la hiérarchie (le haut prime). @everyone s'applique à tous.
  </p>

  <div class="mt-6 flex gap-6">
    <div class="w-56 shrink-0 space-y-1">
      {#each roles as r (r.id)}
        <div
          role="presentation"
          draggable={!r.is_everyone}
          ondragstart={() => onDragStart(r.id)}
          ondragover={(e) => e.preventDefault()}
          ondrop={() => onDropBefore(r.id)}
          class="group flex items-center gap-1.5 rounded px-2 py-1.5 transition-colors duration-150
                 {selectedId === r.id ? 'bg-elevated' : 'hover:bg-surface'}
                 {dragId === r.id ? 'opacity-40' : ''}"
        >
          {#if !r.is_everyone}
            <GripVertical size={14} class="shrink-0 cursor-grab text-muted opacity-0 group-hover:opacity-100" />
          {:else}
            <span class="w-3.5 shrink-0"></span>
          {/if}
          <button
            type="button"
            onclick={() => (selectedId = r.id)}
            class="flex min-w-0 flex-1 items-center gap-2 text-left"
          >
            <span
              class="size-3 shrink-0 rounded-full"
              style="background:{r.color ?? '#A79FCB'}"
            ></span>
            {#if r.icon_emoji}<span class="shrink-0 text-sm leading-none">{r.icon_emoji}</span>{/if}
            <span class="truncate text-body {selectedId === r.id ? 'text-content' : 'text-muted'}">
              {r.is_everyone ? '@everyone' : r.name}
            </span>
          </button>
          {#if !r.is_everyone}
            <button
              type="button"
              title="Supprimer"
              onclick={() => doDelete(r)}
              class="grid size-5 shrink-0 place-items-center rounded text-muted opacity-0 transition-opacity hover:text-danger group-hover:opacity-100"
            >
              <Trash2 size={13} />
            </button>
          {/if}
        </div>
      {/each}

      {#if creating}
        <!-- svelte-ignore a11y_autofocus -->
        <input
          autofocus
          bind:value={newName}
          onkeydown={(e) => {
            if (e.key === 'Enter') void doCreate();
            if (e.key === 'Escape') creating = false;
          }}
          onblur={() => void doCreate()}
          maxlength={64}
          placeholder="Nom du rôle"
          class="w-full rounded border border-border-strong bg-base px-2 py-1.5 text-body text-content outline-none"
        />
      {:else}
        <button
          type="button"
          onclick={() => (creating = true)}
          class="flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-label font-medium text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          <Plus size={14} /> Nouveau rôle
        </button>
      {/if}
    </div>

    <div class="min-w-0 flex-1">
      {#if !selected}
        <p class="text-body text-muted">Choisis un rôle à gauche.</p>
      {:else}
        <div class="mb-4 flex items-center gap-2 border-b border-border">
          {#each [{ k: 'display', l: 'Affichage' }, { k: 'perms', l: 'Permissions' }, { k: 'members', l: 'Membres' }] as t (t.k)}
            {#if !(selected.is_everyone && t.k !== 'perms')}
              <button
                type="button"
                onclick={() => (editTab = t.k as typeof editTab)}
                class="-mb-px border-b-2 px-3 py-2 text-label font-medium transition-colors duration-150
                       {editTab === t.k ? 'border-brand text-content' : 'border-transparent text-muted hover:text-content'}"
              >
                {t.l}
              </button>
            {/if}
          {/each}
          {#if saved}<span class="ml-auto flex items-center gap-1 text-label text-success"><Check size={14} /> Enregistré</span>{/if}
        </div>

        {#if editTab === 'display' && !selected.is_everyone}
          <div class="space-y-5">
            <div class="space-y-1.5">
              <span class="block text-label font-medium text-muted">Nom</span>
              <input
                value={selected.name}
                onchange={(e) => patch({ name: e.currentTarget.value.trim() })}
                maxlength={64}
                class="h-10 w-full max-w-xs rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              />
            </div>

            <div class="space-y-1.5">
              <span class="block text-label font-medium text-muted">Couleur</span>
              <div class="flex flex-wrap items-center gap-1.5">
                {#each COLORS as c (c)}
                  <button
                    type="button"
                    onclick={() => patch({ color: c })}
                    aria-label={c}
                    class="size-7 rounded-full ring-2 transition-transform duration-150 hover:scale-110
                           {selected.color === c ? 'ring-content' : 'ring-transparent'}"
                    style="background:{c}"
                  ></button>
                {/each}
                <input
                  type="color"
                  value={selected.color ?? '#A79FCB'}
                  onchange={(e) => patch({ color: e.currentTarget.value })}
                  class="size-7 cursor-pointer rounded-full border border-border bg-transparent"
                  title="Couleur personnalisée"
                />
              </div>
            </div>

            <div class="space-y-1.5">
              <span class="block text-label font-medium text-muted">Icône (emoji)</span>
              <div class="flex flex-wrap gap-1">
                <button
                  type="button"
                  onclick={() => patch({ icon_emoji: null })}
                  class="grid size-8 place-items-center rounded border transition-colors duration-150
                         {!selected.icon_emoji ? 'border-primary bg-primary/10 text-content' : 'border-border text-muted hover:border-border-strong'}"
                  title="Aucune"
                >
                  <Hash size={15} />
                </button>
                {#each '⭐ 🛡️ 👑 🎮 🔧 🎨 💎 🚀 🔥 ❤️'.split(' ') as e (e)}
                  <button
                    type="button"
                    onclick={() => patch({ icon_emoji: e })}
                    class="grid size-8 place-items-center rounded border text-lg transition-colors duration-150
                           {selected.icon_emoji === e ? 'border-primary bg-primary/10' : 'border-transparent hover:bg-elevated'}"
                  >
                    {e}
                  </button>
                {/each}
              </div>
            </div>

            <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 transition-colors duration-150 hover:bg-elevated/40">
              <input
                type="checkbox"
                checked={selected.hoist ?? false}
                onchange={(e) => patch({ hoist: e.currentTarget.checked })}
                class="mt-0.5 accent-primary"
              />
              <span>
                <span class="block text-body text-content">Afficher séparément</span>
                <span class="block text-label text-muted">Les membres de ce rôle ont leur propre section dans le roster.</span>
              </span>
            </label>

            <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 transition-colors duration-150 hover:bg-elevated/40">
              <input
                type="checkbox"
                checked={selected.mentionable ?? false}
                onchange={(e) => patch({ mentionable: e.currentTarget.checked })}
                class="mt-0.5 accent-primary"
              />
              <span>
                <span class="block text-body text-content">Mentionnable</span>
                <span class="block text-label text-muted">Tout le monde peut mentionner @{selected.name}.</span>
              </span>
            </label>
          </div>
        {:else if editTab === 'perms'}
          <div class="space-y-6">
            {#each PERMISSION_GROUPS as group (group.title)}
              <div>
                <h3 class="mb-2 text-label font-semibold uppercase tracking-wide text-muted">{group.title}</h3>
                <div class="space-y-1.5">
                  {#each group.perms as p (p.bit)}
                    <label
                      class="flex cursor-pointer items-center justify-between gap-3 rounded-lg border p-3 transition-colors duration-150
                             {p.danger ? 'border-danger/40 bg-danger/5' : 'border-border hover:bg-elevated/40'}"
                    >
                      <span class="min-w-0">
                        <span class="block text-body {p.danger ? 'text-danger' : 'text-content'}">{p.label}</span>
                        {#if p.desc}<span class="block text-label text-muted">{p.desc}</span>{/if}
                      </span>
                      <input
                        type="checkbox"
                        checked={hasBit(selected.permissions ?? 0, p.bit)}
                        onchange={() => togglePerm(p.bit)}
                        disabled={saving}
                        class="size-5 shrink-0 cursor-pointer {p.danger ? 'accent-danger' : 'accent-primary'}"
                      />
                    </label>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        {:else if editTab === 'members' && !selected.is_everyone}
          <div class="space-y-4">
            <div class="flex flex-wrap items-center gap-2">
              <select
                bind:value={memberPicker}
                class="h-10 rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              >
                <option value="">Ajouter un membre…</option>
                {#each assignable as m (m.id)}
                  <option value={m.id}>{displayName(m)}</option>
                {/each}
              </select>
              <select
                bind:value={memberDuration}
                title="Durée de l'attribution"
                class="h-10 rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
              >
                <option value={0}>Permanent</option>
                <option value={3600}>1 heure</option>
                <option value={86400}>1 jour</option>
                <option value={604800}>7 jours</option>
              </select>
              <Button type="button" variant="ghost" disabled={!memberPicker} onclick={addMemberToRole}>
                Ajouter
              </Button>
            </div>
            <div class="divide-y divide-border rounded-lg border border-border">
              {#each roleMembers as rm (rm.member_id)}
                <div class="flex items-center justify-between gap-2 px-3 py-2">
                  <span class="truncate text-body text-content">{rm.nickname || rm.username}</span>
                  <button
                    type="button"
                    title="Retirer"
                    onclick={() => removeMemberFromRole(rm.member_id)}
                    class="grid size-6 place-items-center rounded text-muted transition-colors hover:text-danger"
                  >
                    <X size={14} />
                  </button>
                </div>
              {:else}
                <p class="px-3 py-4 text-center text-label text-muted">Aucun membre n'a ce rôle.</p>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>
