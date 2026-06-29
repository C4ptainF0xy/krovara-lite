<script lang="ts">
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { Dialog } from 'bits-ui';
  import { Sparkles, Hash, Compass, UserCircle, ArrowRight, Check } from '@lucide/svelte';

  const SHOW_KEY = 'krovara.show_onboarding';

  type Step = {
    icon: typeof Sparkles;
    title: string;
    body: string;
    cta?: { label: string; href: string };
  };
  const STEPS: Step[] = [
    {
      icon: Sparkles,
      title: 'Bienvenue sur Krovara',
      body: 'Ta messagerie temps réel, gaming et pro réunis. Voici l’essentiel pour démarrer en moins d’une minute.'
    },
    {
      icon: Hash,
      title: 'Crée ou rejoins un espace',
      body: 'Un espace regroupe tes salons texte et vocaux. Crée le tien avec le bouton + à gauche, ou découvre des communautés publiques.',
      cta: { label: 'Explorer les espaces', href: '/app/discover' }
    },
    {
      icon: UserCircle,
      title: 'Personnalise ton profil',
      body: 'Avatar, bio, statut, et même une activité de jeu : règle tout ça dans tes réglages quand tu veux.',
      cta: { label: 'Ouvrir mes réglages', href: '/app/settings/profile' }
    }
  ];

  let open = $state(browser && localStorage.getItem(SHOW_KEY) === '1');
  let step = $state(0);

  const current = $derived(STEPS[step]);
  const isLast = $derived(step === STEPS.length - 1);

  function finish() {
    if (browser) localStorage.removeItem(SHOW_KEY);
    open = false;
  }
  function next() {
    if (isLast) finish();
    else step += 1;
  }
  async function followCta(href: string) {
    finish();
    await goto(href);
  }
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) finish(); }}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-fade-in" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 z-50 w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2
             rounded-xl border border-border bg-surface p-6 shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
    >
      <div class="grid size-12 place-items-center rounded-2xl bg-primary/15 text-accent">
        {#key step}
          <current.icon size={24} />
        {/key}
      </div>

      <Dialog.Title class="mt-4 text-subtitle font-bold text-content">{current.title}</Dialog.Title>
      <p class="mt-2 text-body text-muted">{current.body}</p>

      {#if current.cta}
        <button
          type="button"
          onclick={() => followCta(current.cta!.href)}
          class="mt-4 inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-label text-content transition-colors hover:border-border-strong hover:bg-elevated"
        >
          <Compass size={15} /> {current.cta.label} <ArrowRight size={14} />
        </button>
      {/if}

      <div class="mt-6 flex items-center justify-between">
        <div class="flex gap-1.5" aria-hidden="true">
          {#each STEPS as _, i (i)}
            <span class="size-1.5 rounded-full transition-colors {i === step ? 'bg-primary' : 'bg-border-strong'}"></span>
          {/each}
        </div>
        <div class="flex items-center gap-2">
          {#if !isLast}
            <button type="button" onclick={finish} class="px-2 py-1.5 text-label text-muted transition-colors hover:text-content">
              Passer
            </button>
          {/if}
          <button
            type="button"
            onclick={next}
            class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-1.5 text-label font-medium text-white transition-[filter] hover:brightness-110"
          >
            {#if isLast}<Check size={15} /> Commencer{:else}Suivant <ArrowRight size={14} />{/if}
          </button>
        </div>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
