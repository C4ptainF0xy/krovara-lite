<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  onMount(() => {
    void goto('/app?tab=messages', { replaceState: true });
  });
</script>

<div class="grid h-full place-items-center text-label text-muted">Redirection…</div>
