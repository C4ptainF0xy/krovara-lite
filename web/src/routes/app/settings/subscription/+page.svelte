<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { Sparkles, Check, ExternalLink, AlertCircle } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import type { AuthUser } from '$lib/stores/auth';
  import { Button } from '$lib/ui';

  const tier = $derived($auth.user?.tier ?? 'free');
  const isPlus = $derived(tier === 'plus');

  let busy = $state(false);
  let err = $state<string | null>(null);
  let banner = $state<'success' | 'canceled' | null>(null);

  const PERKS = [
    '50 Go de média conservé (au lieu de 2 Go)',
    'Vidéo 1080p60 et bitrate supérieur',
    'Badge supporter décoratif',
    'Soutiens un projet indépendant, sans pub ni revente de données'
  ];

  onMount(async () => {
    const sp = page.url.searchParams;
    if (sp.get('success') === '1') banner = 'success';
    else if (sp.get('canceled') === '1') banner = 'canceled';
    if (banner) await refreshMe();
  });

  async function refreshMe() {
    try {
      const me = await api<AuthUser>('/api/me');
      auth.update((s) => ({ ...s, user: me }));
    } catch {
    }
  }

  let billingReady = $state(true);

  async function subscribe() {
    err = null;
    busy = true;
    try {
      const { url } = await api<{ url: string }>('/api/billing/checkout', { method: 'POST' });
      window.location.href = url;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        billingReady = false;
      } else {
        err = e instanceof Error ? e.message : 'Indisponible';
      }
      busy = false;
    }
  }

  async function manage() {
    err = null;
    busy = true;
    try {
      const { url } = await api<{ url: string }>('/api/billing/portal', { method: 'POST' });
      window.location.href = url;
    } catch (e) {
      err = e instanceof Error ? e.message : 'Indisponible';
      busy = false;
    }
  }
</script>

<div class="space-y-8">
  <section>
    <h2 class="flex items-center gap-2 text-subtitle font-semibold text-content">
      <Sparkles size={18} class="text-accent" /> Krovara+
    </h2>
    <p class="mt-1 text-body text-muted">
      L'abonnement qui finance le projet. Cosmétique et confort, jamais de pouvoir ni d'avantage de jeu.
    </p>
  </section>

  {#if banner === 'success'}
    <div class="flex items-center gap-2 rounded-lg border border-success/40 bg-success/10 px-4 py-3 text-body text-success">
      <Check size={16} class="shrink-0" /> Merci ! Ton abonnement Krovara+ est actif.
    </div>
  {:else if banner === 'canceled'}
    <div class="flex items-center gap-2 rounded-lg border border-border bg-elevated/50 px-4 py-3 text-body text-muted">
      <AlertCircle size={16} class="shrink-0" /> Paiement annulé, aucun montant débité.
    </div>
  {/if}

  <section class="max-w-md overflow-hidden rounded-xl border border-border">
    <div class="border-b border-border bg-gradient-to-r from-primary/20 to-brand/10 px-5 py-4">
      <div class="flex items-center justify-between">
        <div>
          <p class="text-label uppercase tracking-wide text-muted">Ton offre</p>
          <p class="mt-0.5 text-subtitle font-bold text-content">
            {isPlus ? 'Krovara+' : 'Gratuit'}
          </p>
        </div>
        {#if isPlus}
          <span class="flex items-center gap-1 rounded-full bg-accent/15 px-2.5 py-1 text-label font-semibold text-accent">
            <Sparkles size={12} /> Actif
          </span>
        {:else}
          <span class="text-subtitle font-bold text-content">4,99 €<span class="text-label font-normal text-muted">/mois</span></span>
        {/if}
      </div>
    </div>

    <div class="space-y-2.5 px-5 py-4">
      {#each PERKS as perk (perk)}
        <div class="flex items-start gap-2.5">
          <Check size={15} class="mt-0.5 shrink-0 {isPlus ? 'text-success' : 'text-accent'}" />
          <span class="text-body text-content">{perk}</span>
        </div>
      {/each}
    </div>

    <div class="border-t border-border px-5 py-4">
      {#if err}<p class="mb-3 text-label text-danger">{err}</p>{/if}
      {#if isPlus}
        <Button type="button" variant="secondary" loading={busy} onclick={manage}>
          <ExternalLink size={15} /> Gérer l'abonnement
        </Button>
        <p class="mt-2 text-label text-muted">
          Résiliation, moyen de paiement et factures sur le portail sécurisé Stripe.
        </p>
      {:else if !billingReady}
        <Button type="button" disabled>
          <Sparkles size={15} /> Arrive bientôt
        </Button>
        <p class="mt-2 text-label text-muted">
          Les paiements ne sont pas encore ouverts, Krovara+ arrive bientôt.
        </p>
      {:else}
        <Button type="button" loading={busy} onclick={subscribe}>
          <Sparkles size={15} /> Passer à Krovara+
        </Button>
        <p class="mt-2 text-label text-muted">
          Paiement sécurisé par Stripe. Résiliable à tout moment.
        </p>
      {/if}
    </div>
  </section>

  <p class="max-w-md text-label text-muted">
    Le gratuit reste un produit fini : tout le social, le texte et les emojis custom de tes
    serveurs y sont inclus. Krovara+ ajoute du quota média, de la vidéo HD et du cosmétique.
  </p>
</div>
