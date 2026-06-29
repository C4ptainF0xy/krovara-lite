<script lang="ts">
  import { BarChart3, X, Plus, Trash2, Check, Lock } from '@lucide/svelte';
  import {
    pollsByChannel,
    loadPolls,
    createPoll,
    votePoll,
    closePoll,
    totalVotes,
    type Poll
  } from '$lib/stores/polls';
  import { Button } from '$lib/ui';

  type Props = {
    open: boolean;
    channelId: string;
    selfId?: string;
    onclose: () => void;
  };
  let { open, channelId, selfId, onclose }: Props = $props();

  const polls = $derived<Poll[]>($pollsByChannel[channelId] ?? []);

  let loadedFor = '';
  $effect(() => {
    if (open && channelId && loadedFor !== channelId) {
      loadedFor = channelId;
      void loadPolls(channelId).catch(() => {});
    }
    if (!open) loadedFor = '';
  });

  let creating = $state(false);
  let question = $state('');
  let options = $state<string[]>(['', '']);
  let busy = $state(false);
  let err = $state<string | null>(null);

  function addOption() {
    if (options.length < 10) options = [...options, ''];
  }
  function removeOption(i: number) {
    if (options.length > 2) options = options.filter((_, idx) => idx !== i);
  }
  function resetForm() {
    creating = false;
    question = '';
    options = ['', ''];
    err = null;
  }
  async function submit() {
    const q = question.trim();
    const opts = options.map((o) => o.trim()).filter(Boolean);
    if (!q || opts.length < 2) {
      err = 'Une question et au moins 2 options.';
      return;
    }
    busy = true;
    err = null;
    try {
      await createPoll(channelId, q, opts);
      resetForm();
    } catch (e) {
      err = e instanceof Error ? e.message : 'Échec';
    } finally {
      busy = false;
    }
  }

  function pct(p: Poll, votes: number): number {
    const t = totalVotes(p);
    return t === 0 ? 0 : Math.round((votes / t) * 100);
  }
</script>

{#if open}
  <button
    type="button"
    aria-label="Fermer les sondages"
    class="absolute inset-0 z-20 bg-base/40 backdrop-blur-[1px]"
    onclick={onclose}
  ></button>
  <aside
    class="absolute right-0 top-0 z-30 flex h-full w-80 max-w-[88%] flex-col border-l border-border bg-surface shadow-2xl shadow-black/40 animate-slide-in"
  >
    <header class="flex items-center gap-2 border-b border-border px-4 py-3">
      <BarChart3 size={16} class="text-accent" />
      <h2 class="text-body font-semibold text-content">Sondages</h2>
      <span class="text-label text-muted">{polls.length}</span>
      <button
        type="button"
        title="Fermer"
        onclick={onclose}
        class="ml-auto grid size-7 place-items-center rounded text-muted transition-colors hover:bg-elevated hover:text-content"
      >
        <X size={16} />
      </button>
    </header>

    <div class="flex-1 space-y-3 overflow-y-auto p-3">
      {#if creating}
        <div class="space-y-2 rounded-lg border border-border bg-base/40 p-3">
          <input
            bind:value={question}
            placeholder="Ta question…"
            maxlength={300}
            class="h-9 w-full rounded border border-border bg-base/50 px-2.5 text-body text-content outline-none focus:border-primary"
          />
          {#each options as _, i (i)}
            <div class="flex items-center gap-1.5">
              <input
                bind:value={options[i]}
                placeholder={`Option ${i + 1}`}
                maxlength={120}
                class="h-8 flex-1 rounded border border-border bg-base/50 px-2.5 text-label text-content outline-none focus:border-primary"
              />
              {#if options.length > 2}
                <button
                  type="button"
                  onclick={() => removeOption(i)}
                  aria-label="Retirer l'option"
                  class="shrink-0 rounded p-1 text-muted transition-colors hover:text-danger"
                >
                  <Trash2 size={14} />
                </button>
              {/if}
            </div>
          {/each}
          {#if options.length < 10}
            <button
              type="button"
              onclick={addOption}
              class="inline-flex items-center gap-1 text-label text-muted transition-colors hover:text-content"
            >
              <Plus size={13} /> Ajouter une option
            </button>
          {/if}
          {#if err}<p class="text-label text-danger">{err}</p>{/if}
          <div class="flex justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" onclick={resetForm}>Annuler</Button>
            <Button type="button" loading={busy} onclick={submit}>Créer</Button>
          </div>
        </div>
      {:else}
        <Button type="button" variant="ghost" class="w-full" onclick={() => (creating = true)}>
          <Plus size={15} /> Nouveau sondage
        </Button>
      {/if}

      {#if polls.length === 0 && !creating}
        <p class="px-1 py-6 text-center text-label text-muted">Aucun sondage dans ce salon.</p>
      {/if}

      {#each polls as p (p.id)}
        <article class="rounded-lg border border-border bg-base/40 p-3">
          <div class="flex items-start justify-between gap-2">
            <p class="text-body font-medium text-content">{p.question}</p>
            {#if p.closed}
              <span class="inline-flex shrink-0 items-center gap-1 text-label text-muted"><Lock size={12} /> clos</span>
            {/if}
          </div>
          <div class="mt-2 space-y-1.5">
            {#each p.options as o (o.id)}
              {@const mine = p.my_option === o.id}
              <button
                type="button"
                disabled={p.closed}
                onclick={() => votePoll(channelId, p.id, o.id)}
                class="relative block w-full overflow-hidden rounded border border-border px-2.5 py-1.5 text-left text-label transition-colors duration-150 enabled:hover:border-border-strong disabled:cursor-default
                       {mine ? 'border-primary' : ''}"
              >
                <span
                  class="absolute inset-y-0 left-0 bg-primary/15"
                  style={`width:${pct(p, o.votes)}%`}
                ></span>
                <span class="relative flex items-center justify-between gap-2">
                  <span class="flex min-w-0 items-center gap-1.5 truncate text-content">
                    {#if mine}<Check size={13} class="shrink-0 text-accent" />{/if}
                    {o.label}
                  </span>
                  <span class="shrink-0 tabular-nums text-muted">{pct(p, o.votes)}% · {o.votes}</span>
                </span>
              </button>
            {/each}
          </div>
          <div class="mt-2 flex items-center justify-between text-label text-muted">
            <span>{totalVotes(p)} vote{totalVotes(p) > 1 ? 's' : ''}</span>
            {#if !p.closed && p.created_by === selfId}
              <button type="button" onclick={() => closePoll(channelId, p.id)} class="transition-colors hover:text-content">
                Clore
              </button>
            {/if}
          </div>
        </article>
      {/each}
    </div>
  </aside>
{/if}
