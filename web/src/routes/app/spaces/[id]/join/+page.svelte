<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { Check, ArrowRight, Award } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import {
    getJoinForm,
    submitJoinRequest,
    type JoinForm,
    type JoinAnswer
  } from '$lib/stores/joingate';
  import { Button } from '$lib/ui';

  const spaceId = $derived(page.params.id ?? '');

  let form = $state<JoinForm | null>(null);
  let answers = $state<Record<string, string>>({});
  let loading = $state(true);
  let busy = $state(false);
  let err = $state<string | null>(null);
  let submitted = $state(false);

  let loaded = false;
  $effect(() => {
    if (spaceId && !loaded) {
      loaded = true;
      void getJoinForm(spaceId)
        .then((f) => (form = f))
        .catch(() => (err = 'Impossible de charger le formulaire.'))
        .finally(() => (loading = false));
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (!form) return;
    busy = true;
    err = null;
    try {
      const payload: JoinAnswer[] = form.questions.map((q) => ({
        question_id: q.id,
        answer: (answers[q.id] ?? '').trim()
      }));
      await submitJoinRequest(spaceId, payload);
      submitted = true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        err = e.message.includes('member')
          ? 'Tu es déjà membre de cet espace.'
          : 'Tu as déjà une demande en attente.';
      } else if (e instanceof ApiError && e.status === 403 && e.message.includes('karma')) {
        err = `Karma insuffisant : il faut au moins ${form.min_karma} de karma global.`;
      } else if (e instanceof ApiError) err = e.message;
      else err = 'Envoi impossible.';
    } finally {
      busy = false;
    }
  }
</script>

<div class="mx-auto max-w-lg px-6 py-10">
  {#if loading}
    <div class="space-y-3">
      <div class="h-6 w-40 animate-pulse rounded bg-elevated"></div>
      <div class="h-24 animate-pulse rounded-lg bg-elevated"></div>
    </div>
  {:else if submitted}
    <div class="rounded-lg border border-success/40 bg-success/5 p-6 text-center">
      <Check size={28} class="mx-auto text-success" />
      <h1 class="mt-3 text-subtitle font-semibold text-content">Demande envoyée</h1>
      <p class="mt-1.5 text-body text-muted">
        Un modérateur examinera ta demande. Tu rejoindras l'espace une fois approuvé.
      </p>
      <Button type="button" variant="ghost" class="mt-4" onclick={() => goto('/app')}>
        Retour
      </Button>
    </div>
  {:else if form && !form.enabled}
    <div class="rounded-lg border border-border bg-surface p-6 text-center">
      <h1 class="text-subtitle font-semibold text-content">Adhésion sur invitation</h1>
      <p class="mt-1.5 text-body text-muted">
        Cet espace n'accepte pas de demandes d'entrée. Il te faut une invitation.
      </p>
    </div>
  {:else if form}
    <h1 class="text-title font-bold">Demande d'adhésion</h1>
    <p class="mt-1.5 text-body text-muted">Réponds au formulaire pour rejoindre cet espace.</p>
    {#if form.min_karma > 0}
      <p class="mt-2 inline-flex items-center gap-1.5 rounded-full bg-elevated px-3 py-1 text-label text-muted">
        <Award size={13} class="text-accent" /> Karma global requis : {form.min_karma}
      </p>
    {/if}
    <form onsubmit={submit} class="mt-6 space-y-5">
      {#each form.questions as q (q.id)}
        <div class="space-y-1.5">
          <label for={`jq-${q.id}`} class="block text-label font-medium text-muted">
            {q.label}{#if q.required}<span class="text-danger"> *</span>{/if}
          </label>
          <textarea
            id={`jq-${q.id}`}
            bind:value={answers[q.id]}
            required={q.required}
            maxlength={2000}
            rows={2}
            class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none transition-[box-shadow,border-color] duration-150 ease-smooth placeholder:text-muted/60 focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]"
          ></textarea>
        </div>
      {/each}
      {#if err}<p class="text-label text-danger">{err}</p>{/if}
      <Button type="submit" loading={busy}>
        {#if !busy}Envoyer la demande <ArrowRight size={16} />{/if}
      </Button>
    </form>
  {:else}
    <p class="text-body text-danger">{err ?? 'Erreur.'}</p>
  {/if}
</div>
