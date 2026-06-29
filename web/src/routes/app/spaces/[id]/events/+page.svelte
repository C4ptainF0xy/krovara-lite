<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { ArrowLeft, CalendarDays, MapPin, Plus, Trash2, Check } from '@lucide/svelte';
  import { auth } from '$lib/stores/auth';
  import {
    eventsBySpace,
    loadEvents,
    createEvent,
    rsvpEvent,
    deleteEvent,
    type SpaceEvent,
    type RsvpStatus
  } from '$lib/stores/events';
  import { Button, Input } from '$lib/ui';

  const spaceId = $derived(page.params.id ?? '');
  const events = $derived<SpaceEvent[]>($eventsBySpace[spaceId] ?? []);

  let loading = $state(true);
  onMount(async () => {
    try {
      await loadEvents(spaceId);
    } finally {
      loading = false;
    }
  });

  let creating = $state(false);
  let title = $state('');
  let when = $state('');
  let location = $state('');
  let description = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  function reset() {
    creating = false;
    title = '';
    when = '';
    location = '';
    description = '';
    err = null;
  }

  async function submit(e: Event) {
    e.preventDefault();
    const t = title.trim();
    if (!t || !when) {
      err = 'Un titre et une date sont requis.';
      return;
    }
    busy = true;
    err = null;
    try {
      await createEvent(spaceId, {
        title: t,
        location: location.trim() || null,
        description: description.trim() || null,
        starts_at: new Date(when).toISOString()
      });
      reset();
    } catch (e2) {
      err = e2 instanceof Error ? e2.message : 'Échec';
    } finally {
      busy = false;
    }
  }

  const RSVPS: { v: RsvpStatus; label: string }[] = [
    { v: 'going', label: 'Présent' },
    { v: 'maybe', label: 'Peut-être' },
    { v: 'no', label: 'Absent' }
  ];

  function fmtDate(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'long' }) +
      ' · ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  function isPast(iso: string): boolean {
    return new Date(iso).getTime() < Date.now();
  }
</script>

<div class="mx-auto max-w-2xl px-6 py-8">
  <a
    href={`/app/spaces/${spaceId}`}
    class="mb-4 inline-flex items-center gap-1.5 text-label text-muted transition-colors duration-150 hover:text-content"
  >
    <ArrowLeft size={15} /> Retour à l'espace
  </a>
  <div class="flex items-center justify-between gap-3">
    <h1 class="flex items-center gap-2 text-title font-bold"><CalendarDays size={24} /> Événements</h1>
    {#if !creating}
      <Button type="button" onclick={() => (creating = true)}><Plus size={15} /> Nouvel événement</Button>
    {/if}
  </div>

  {#if creating}
    <form onsubmit={submit} class="mt-5 space-y-3 rounded-lg border border-border bg-surface p-4">
      <Input label="Titre" bind:value={title} maxlength={200} placeholder="Soirée jeux, AG, stream…" />
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="space-y-1.5">
          <label for="ev-when" class="block text-label font-medium text-muted">Date et heure</label>
          <input
            id="ev-when"
            type="datetime-local"
            bind:value={when}
            class="h-10 w-full rounded border border-border bg-base/50 px-3 text-body text-content outline-none focus:border-primary"
          />
        </div>
        <Input label="Lieu (optionnel)" bind:value={location} maxlength={200} placeholder="Discord, IRL…" />
      </div>
      <div class="space-y-1.5">
        <label for="ev-desc" class="block text-label font-medium text-muted">Description (optionnel)</label>
        <textarea
          id="ev-desc"
          bind:value={description}
          maxlength={2000}
          rows={2}
          class="w-full resize-none rounded border border-border bg-base/50 px-3 py-2 text-body text-content outline-none focus:border-primary"
        ></textarea>
      </div>
      {#if err}<p class="text-label text-danger">{err}</p>{/if}
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" onclick={reset}>Annuler</Button>
        <Button type="submit" loading={busy}>Créer</Button>
      </div>
    </form>
  {/if}

  <div class="mt-6 space-y-3">
    {#if loading}
      <div class="h-24 animate-pulse rounded-lg bg-elevated/50"></div>
    {:else if events.length === 0}
      <div class="grid place-items-center gap-2 py-16 text-center">
        <CalendarDays size={26} class="text-muted" />
        <p class="text-body text-muted">Aucun événement à venir.</p>
      </div>
    {:else}
      {#each events as ev (ev.id)}
        <article class="rounded-lg border border-border bg-surface p-4 {isPast(ev.starts_at) ? 'opacity-60' : ''}">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="text-body font-semibold text-content">{ev.title}</p>
              <p class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-label text-muted">
                <span class="flex items-center gap-1"><CalendarDays size={13} /> {fmtDate(ev.starts_at)}</span>
                {#if ev.location}<span class="flex items-center gap-1"><MapPin size={13} /> {ev.location}</span>{/if}
              </p>
            </div>
            {#if ev.created_by === $auth.user?.id}
              <button
                type="button"
                onclick={() => deleteEvent(spaceId, ev.id)}
                aria-label="Supprimer l'événement"
                class="shrink-0 rounded p-1.5 text-muted transition-colors duration-150 hover:bg-elevated hover:text-danger"
              >
                <Trash2 size={16} />
              </button>
            {/if}
          </div>
          {#if ev.description}
            <p class="mt-2 whitespace-pre-wrap text-label text-content/80">{ev.description}</p>
          {/if}
          <div class="mt-3 flex flex-wrap items-center gap-1.5">
            {#each RSVPS as r (r.v)}
              {@const active = ev.my_rsvp === r.v}
              <button
                type="button"
                onclick={() => rsvpEvent(spaceId, ev.id, r.v)}
                class="inline-flex items-center gap-1 rounded-full border px-3 py-1 text-label transition-colors duration-150
                       {active ? 'border-primary bg-primary/10 text-content' : 'border-border text-muted hover:border-border-strong'}"
              >
                {#if active}<Check size={13} class="text-accent" />{/if}
                {r.label}
                <span class="tabular-nums text-muted">{ev.rsvp[r.v]}</span>
              </button>
            {/each}
          </div>
        </article>
      {/each}
    {/if}
  </div>
</div>
