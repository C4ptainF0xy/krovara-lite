<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { snapshot } from '$lib/stores/auth';

  onMount(() => {
    goto(snapshot().accessToken ? '/app' : '/login');
  });
</script>
