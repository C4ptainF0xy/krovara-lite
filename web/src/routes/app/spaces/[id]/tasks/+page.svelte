<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { ArrowLeft, ListChecks, Plus, Trash2, Check } from '@lucide/svelte';
  import { listTasks, createTask, setTaskStatus, deleteTask, type Task } from '$lib/stores/tasks';
  import { Button, Input } from '$lib/ui';

  const spaceId = $derived(page.params.id ?? '');

  let tasks = $state<Task[]>([]);
  let loading = $state(true);
  let newTitle = $state('');
  let busy = $state(false);

  onMount(load);
  async function load() {
    loading = true;
    try {
      tasks = await listTasks(spaceId);
    } finally {
      loading = false;
    }
  }

  const open = $derived(tasks.filter((t) => t.status === 'open'));
  const done = $derived(tasks.filter((t) => t.status === 'done'));

  async function add(e: Event) {
    e.preventDefault();
    const title = newTitle.trim();
    if (!title) return;
    busy = true;
    try {
      const t = await createTask(spaceId, title);
      tasks = [t, ...tasks];
      newTitle = '';
    } finally {
      busy = false;
    }
  }

  async function toggle(t: Task) {
    const updated = await setTaskStatus(t.id, t.status === 'open' ? 'done' : 'open');
    tasks = tasks.map((x) => (x.id === t.id ? updated : x));
  }

  async function remove(t: Task) {
    await deleteTask(t.id);
    tasks = tasks.filter((x) => x.id !== t.id);
  }
</script>

<div class="mx-auto max-w-2xl px-6 py-8">
  <a
    href={`/app/spaces/${spaceId}`}
    class="mb-4 inline-flex items-center gap-1.5 text-label text-muted transition-colors duration-150 hover:text-content"
  >
    <ArrowLeft size={15} /> Retour à l'espace
  </a>
  <h1 class="flex items-center gap-2 text-title font-bold"><ListChecks size={26} /> Tâches</h1>

  <form onsubmit={add} class="mt-6 flex items-end gap-2">
    <div class="flex-1">
      <Input bind:value={newTitle} placeholder="Nouvelle tâche…" maxlength={280} />
    </div>
    <Button type="submit" loading={busy}><Plus size={16} /> Ajouter</Button>
  </form>

  <div class="mt-6 space-y-1">
    {#if loading}
      {#each [0, 1, 2] as i (i)}<div class="h-11 animate-pulse rounded-lg bg-elevated/50"></div>{/each}
    {:else if tasks.length === 0}
      <div class="grid place-items-center gap-3 py-16 text-center">
        <div class="grid size-14 place-items-center rounded-full bg-elevated text-muted"><ListChecks size={26} /></div>
        <p class="text-body text-muted">Aucune tâche. Ajoute la première !</p>
      </div>
    {:else}
      {#each open as t (t.id)}
        {@render row(t)}
      {/each}
      {#if done.length}
        <p class="px-1 pt-4 text-label font-semibold uppercase tracking-wide text-muted">Terminées ({done.length})</p>
        {#each done as t (t.id)}
          {@render row(t)}
        {/each}
      {/if}
    {/if}
  </div>
</div>

{#snippet row(t: Task)}
  <div class="group flex items-center gap-3 rounded-lg border border-border p-3 transition-colors duration-150 hover:bg-surface/50">
    <button
      type="button"
      onclick={() => toggle(t)}
      aria-pressed={t.status === 'done'}
      title={t.status === 'done' ? 'Rouvrir' : 'Terminer'}
      class="grid size-5 shrink-0 place-items-center rounded-full border transition-colors duration-150
             {t.status === 'done' ? 'border-success bg-success text-white' : 'border-border-strong text-transparent hover:border-success'}"
    >
      <Check size={13} />
    </button>
    <span class="min-w-0 flex-1 truncate text-body {t.status === 'done' ? 'text-muted line-through' : 'text-content'}">
      {t.title}
    </span>
    <button
      type="button"
      title="Supprimer"
      onclick={() => remove(t)}
      class="grid size-7 shrink-0 place-items-center rounded text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"
    >
      <Trash2 size={15} />
    </button>
  </div>
{/snippet}
