<script lang="ts">
  import { announcement } from '$lib/stores/announce';

  let polite = $state('');
  let assertive = $state('');
  let lastN = 0;

  $effect(() => {
    const a = $announcement;
    if (a.n === lastN || !a.text) return;
    lastN = a.n;
    if (a.assertive) {
      assertive = '';
      queueMicrotask(() => (assertive = a.text));
    } else {
      polite = '';
      queueMicrotask(() => (polite = a.text));
    }
  });
</script>

<div class="sr-only" aria-live="polite" aria-atomic="true">{polite}</div>
<div class="sr-only" aria-live="assertive" aria-atomic="true" role="alert">{assertive}</div>
