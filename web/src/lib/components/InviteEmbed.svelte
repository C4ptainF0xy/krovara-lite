<script lang="ts">
  import { onMount } from 'svelte';
  import { api, authedObjectURL } from '$lib/api';

  type Props = { code: string };
  let { code }: Props = $props();

  type InvitePreview = {
    code: string;
    space_id: string;
    space_name: string;
    space_icon?: string;
    space_banner?: string;
    space_description?: string;
    member_count?: number;
    created_at?: string;
    expires_at?: string;
    max_uses: number;
    uses: number;
  };

  let invite = $state<InvitePreview | null>(null);
  let error = $state<string | null>(null);
  let iconUrl = $state<string | null>(null);
  let bannerUrl = $state<string | null>(null);

  onMount(load);

  async function load() {
    try {
      invite = await api<InvitePreview>(`/api/invites/${code}`);
      if (invite.space_icon && !invite.space_icon.startsWith(':')) {
        authedObjectURL(`/api/files/${invite.space_icon}`)
          .then((u) => (iconUrl = u))
          .catch(() => {});
      }
      if (invite.space_banner) {
        authedObjectURL(`/api/files/${invite.space_banner}`)
          .then((u) => (bannerUrl = u))
          .catch(() => {});
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Invitation invalide ou expirée';
    }
  }

  const initials = $derived((invite?.space_name ?? '').trim().slice(0, 2).toUpperCase() || '?');
  const memberLabel = $derived(
    invite?.member_count != null ? `${invite.member_count} membre${invite.member_count > 1 ? 's' : ''}` : ''
  );
  const sinceLabel = $derived(
    invite?.created_at
      ? `depuis ${new Date(invite.created_at).toLocaleDateString('fr-FR', { month: 'long', year: 'numeric' })}`
      : ''
  );
</script>

<div class="my-2 w-full max-w-sm overflow-hidden rounded-lg border border-border bg-surface shadow-sm">
  {#if error}
    <div class="flex flex-col gap-1 p-4">
      <div class="text-label font-semibold uppercase tracking-wide text-muted">Invitation</div>
      <div class="text-body font-medium text-danger">{error}</div>
    </div>
  {:else if !invite}
    <div class="flex items-center gap-3 p-4">
      <div class="size-12 animate-pulse rounded-2xl bg-elevated"></div>
      <div class="flex flex-1 flex-col gap-2">
        <div class="h-4 w-24 animate-pulse rounded bg-elevated"></div>
        <div class="h-3 w-32 animate-pulse rounded bg-elevated"></div>
      </div>
    </div>
  {:else}
    {#if bannerUrl}
      <div class="h-20 w-full overflow-hidden">
        <img src={bannerUrl} alt="" class="size-full object-cover" />
      </div>
    {:else}
      <div class="h-16 w-full bg-gradient-to-r from-primary/40 to-brand/30"></div>
    {/if}

    <div class="p-4">
      <div class="-mt-10 mb-3 flex items-end justify-between gap-3">
        <div class="grid size-16 place-items-center overflow-hidden rounded-2xl bg-elevated text-subtitle font-bold text-muted ring-4 ring-surface">
          {#if iconUrl}
            <img src={iconUrl} alt="" class="size-full object-cover" />
          {:else}
            {initials}
          {/if}
        </div>
      </div>

      <div class="mb-1 text-label font-semibold uppercase tracking-wide text-muted">
        Vous avez été invité(e) à rejoindre un serveur
      </div>
      <h3 class="truncate text-subtitle font-bold text-content">{invite.space_name}</h3>

      {#if memberLabel || sinceLabel}
        <div class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-label text-muted">
          {#if memberLabel}
            <span class="flex items-center gap-1.5">
              <span class="size-2 rounded-full bg-success"></span>{memberLabel}
            </span>
          {/if}
          {#if memberLabel && sinceLabel}<span class="text-border-strong">·</span>{/if}
          {#if sinceLabel}<span>{sinceLabel}</span>{/if}
        </div>
      {/if}

      {#if invite.space_description}
        <p class="mt-2 line-clamp-2 text-body text-muted">{invite.space_description}</p>
      {/if}

      <a
        href="/join/{code}"
        class="mt-4 flex w-full items-center justify-center rounded-md bg-primary px-4 py-2.5 text-body font-semibold text-white transition-colors duration-150 hover:bg-primary-hover"
      >
        Rejoindre le serveur
      </a>
    </div>
  {/if}
</div>
