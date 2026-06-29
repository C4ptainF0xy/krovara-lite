<script lang="ts">
  import { Copy, Check } from '@lucide/svelte';

  type Props = { id: string; label?: string };
  let { id, label = 'ID' }: Props = $props();

  let copied = $state(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(id);
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } catch {
    }
  }
</script>

<button
  type="button"
  onclick={copy}
  title="Copier l'identifiant"
  class="inline-flex items-center gap-1.5 rounded border border-border bg-base/50 px-2 py-1
         font-mono text-label text-muted transition-colors duration-150 hover:border-border-strong hover:text-content"
>
  {#if copied}<Check size={13} class="text-success" />{:else}<Copy size={13} />{/if}
  <span class="opacity-60">{label}:</span>
  <span class="max-w-[12rem] truncate">{id}</span>
</button>
