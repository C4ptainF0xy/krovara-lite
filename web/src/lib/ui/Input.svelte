<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props extends HTMLInputAttributes {
    label?: string;
    error?: string | null;
    value?: string;
  }

  let {
    label,
    error = null,
    value = $bindable(''),
    class: klass = '',
    id,
    ...rest
  }: Props = $props();

  const fallbackId = `in-${Math.random().toString(36).slice(2, 8)}`;
  const uid = $derived(id ?? fallbackId);
</script>

<div class="space-y-1.5">
  {#if label}
    <label for={uid} class="block text-label font-medium text-muted">{label}</label>
  {/if}
  <input
    id={uid}
    bind:value
    class="h-10 w-full rounded border bg-base/50 px-3 text-body text-content
           outline-none transition-[box-shadow,border-color] duration-150 ease-smooth
           placeholder:text-muted/60
           {error
      ? 'border-danger focus:shadow-[0_0_0_3px_rgba(229,72,77,0.30)]'
      : 'border-border focus:border-primary focus:shadow-[0_0_0_3px_rgba(122,115,152,0.40)]'}
           {klass}"
    aria-invalid={!!error}
    {...rest}
  />
  {#if error}
    <p class="text-label text-danger">{error}</p>
  {/if}
</div>
