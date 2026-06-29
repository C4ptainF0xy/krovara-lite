<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    class: klass = '',
    children
  }: { class?: string; children: Snippet } = $props();
</script>

<div
  class="rounded-lg border border-border bg-surface/80 backdrop-blur-sm {klass}"
>
  {@render children()}
</div>
