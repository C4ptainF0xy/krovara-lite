<script lang="ts">
  import { Check, Minus, X, Plus, Shield, User } from '@lucide/svelte';
  import {
    loadOverwrites,
    setOverwrite,
    clearOverwrite,
    type Overwrite
  } from '$lib/stores/spaces';
  import { loadMembers, displayName, type Member } from '$lib/stores/members';
  import { api } from '$lib/api';
  import { CHANNEL_OVERWRITE_PERMS, triStateOf, type TriState } from '$lib/permissions';

  type Props = { channelId: string; spaceId: string };
  let { channelId, spaceId }: Props = $props();

  type Role = { id: string; name: string; is_everyone: boolean; position: number };

  let overwrites = $state<Overwrite[]>([]);
  let roles = $state<Role[]>([]);
  let members = $state<Member[]>([]);
  let loading = $state(true);
  let selected = $state<{ type: 'role' | 'member'; id: string } | null>(null);
  let adding = $state(false);

  let draft = $state<Record<number, TriState>>({});

  $effect(() => {
    void load();
  });

  async function load() {
    loading = true;
    try {
      const [ows, rs, ms] = await Promise.all([
        loadOverwrites(channelId),
        api<Role[]>(`/api/spaces/${spaceId}/roles`),
        loadMembers(spaceId)
      ]);
      overwrites = ows;
      roles = rs.sort((a, b) => b.position - a.position);
      members = ms;
      if (!selected && ows.length) {
        select(ows[0].target_type, ows[0].target_id);
      }
    } finally {
      loading = false;
    }
  }

  function overwriteFor(type: 'role' | 'member', id: string): Overwrite | undefined {
    return overwrites.find((o) => o.target_type === type && o.target_id === id);
  }

  function select(type: 'role' | 'member', id: string) {
    selected = { type, id };
    adding = false;
    const ow = overwriteFor(type, id);
    const next: Record<number, TriState> = {};
    for (const p of CHANNEL_OVERWRITE_PERMS) {
      next[p.bit] = ow ? triStateOf(p.bit, ow.allow, ow.deny) : 'inherit';
    }
    draft = next;
  }

  function labelFor(o: Overwrite): string {
    if (o.target_type === 'role') {
      const r = roles.find((x) => x.id === o.target_id);
      return r ? (r.is_everyone ? '@everyone' : r.name) : 'rôle inconnu';
    }
    const m = members.find((x) => x.id === o.target_id);
    return m ? displayName(m) : 'membre inconnu';
  }

  const roleCandidates = $derived(
    roles.filter((r) => !overwriteFor('role', r.id))
  );
  const memberCandidates = $derived(
    members.filter((m) => !overwriteFor('member', m.id))
  );

  async function save() {
    if (!selected) return;
    let allow = 0;
    let deny = 0;
    for (const p of CHANNEL_OVERWRITE_PERMS) {
      const s = draft[p.bit];
      if (s === 'allow') allow |= p.bit;
      else if (s === 'deny') deny |= p.bit;
    }
    if (allow === 0 && deny === 0) {
      await clearOverwrite(channelId, selected.type, selected.id);
    } else {
      await setOverwrite(channelId, selected.type, selected.id, allow, deny);
    }
    await load();
  }

  async function remove(o: Overwrite) {
    await clearOverwrite(channelId, o.target_type, o.target_id);
    if (selected?.type === o.target_type && selected.id === o.target_id) selected = null;
    await load();
  }
</script>

{#if loading}
  <div class="space-y-2">
    {#each [0, 1, 2] as i (i)}
      <div class="h-8 animate-pulse rounded bg-elevated/60"></div>
    {/each}
  </div>
{:else}
  <div class="flex min-h-[16rem] gap-3">
    <div class="flex w-40 shrink-0 flex-col gap-1 border-r border-border pr-3">
      <div class="space-y-0.5 overflow-y-auto">
        {#each overwrites as o (o.target_type + o.target_id)}
          <div class="group/ow flex items-center gap-1">
            <button
              type="button"
              onclick={() => select(o.target_type, o.target_id)}
              class="flex min-w-0 flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-label
                     transition-colors duration-150
                     {selected?.type === o.target_type && selected.id === o.target_id
                ? 'bg-elevated text-content'
                : 'text-muted hover:bg-surface hover:text-content'}"
            >
              {#if o.target_type === 'role'}
                <Shield size={13} class="shrink-0 opacity-70" />
              {:else}
                <User size={13} class="shrink-0 opacity-70" />
              {/if}
              <span class="truncate">{labelFor(o)}</span>
            </button>
            <button
              type="button"
              title="Retirer"
              onclick={() => remove(o)}
              class="grid size-5 shrink-0 place-items-center rounded text-muted opacity-0
                     transition-opacity duration-150 hover:text-danger group-hover/ow:opacity-100"
            >
              <X size={13} />
            </button>
          </div>
        {/each}
      </div>

      {#if adding}
        <div class="mt-1 max-h-40 space-y-0.5 overflow-y-auto rounded border border-border p-1">
          {#each roleCandidates as r (r.id)}
            <button
              type="button"
              onclick={() => {
                select('role', r.id);
              }}
              class="flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-label
                     text-muted transition-colors hover:bg-elevated hover:text-content"
            >
              <Shield size={12} /> {r.is_everyone ? '@everyone' : r.name}
            </button>
          {/each}
          {#each memberCandidates as m (m.id)}
            <button
              type="button"
              onclick={() => {
                select('member', m.id);
              }}
              class="flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-label
                     text-muted transition-colors hover:bg-elevated hover:text-content"
            >
              <User size={12} /> {displayName(m)}
            </button>
          {/each}
          {#if roleCandidates.length === 0 && memberCandidates.length === 0}
            <p class="px-2 py-1 text-label text-muted">Tout le monde a déjà une règle.</p>
          {/if}
        </div>
      {:else}
        <button
          type="button"
          onclick={() => (adding = true)}
          class="mt-1 flex items-center gap-1.5 rounded px-2 py-1.5 text-label font-medium
                 text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          <Plus size={14} /> Ajouter
        </button>
      {/if}
    </div>

    <div class="min-w-0 flex-1">
      {#if !selected}
        <p class="grid h-full place-items-center text-center text-label text-muted">
          Choisis ou ajoute un rôle / membre pour éditer ses permissions ici.
        </p>
      {:else}
        <div class="space-y-1.5">
          {#each CHANNEL_OVERWRITE_PERMS as p (p.bit)}
            {@const state = draft[p.bit] ?? 'inherit'}
            <div class="flex items-center justify-between gap-2 rounded px-2 py-1.5 hover:bg-surface/60">
              <span class="min-w-0 truncate text-body text-content">{p.label}</span>
              <div class="flex shrink-0 overflow-hidden rounded border border-border">
                <button
                  type="button"
                  title="Refuser"
                  aria-pressed={state === 'deny'}
                  onclick={() => (draft = { ...draft, [p.bit]: 'deny' })}
                  class="grid size-7 place-items-center transition-colors duration-150
                         {state === 'deny'
                    ? 'bg-danger text-white'
                    : 'text-muted hover:bg-elevated'}"
                >
                  <X size={14} />
                </button>
                <button
                  type="button"
                  title="Hériter"
                  aria-pressed={state === 'inherit'}
                  onclick={() => (draft = { ...draft, [p.bit]: 'inherit' })}
                  class="grid size-7 place-items-center border-x border-border transition-colors duration-150
                         {state === 'inherit'
                    ? 'bg-elevated text-content'
                    : 'text-muted hover:bg-elevated'}"
                >
                  <Minus size={14} />
                </button>
                <button
                  type="button"
                  title="Autoriser"
                  aria-pressed={state === 'allow'}
                  onclick={() => (draft = { ...draft, [p.bit]: 'allow' })}
                  class="grid size-7 place-items-center transition-colors duration-150
                         {state === 'allow'
                    ? 'bg-success text-white'
                    : 'text-muted hover:bg-elevated'}"
                >
                  <Check size={14} />
                </button>
              </div>
            </div>
          {/each}
          <div class="pt-2">
            <button
              type="button"
              onclick={save}
              class="w-full rounded bg-primary px-3 py-2 text-body font-medium text-white
                     transition-colors duration-150 hover:bg-primary-hover"
            >
              Enregistrer les permissions
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
