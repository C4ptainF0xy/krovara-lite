<script lang="ts">
  import { publishPresence, clearPresence } from '$lib/xmpp/presence';
  import { xmppState } from '$lib/xmpp/client';
  import { Button, Input } from '$lib/ui';

  let game = $state('');
  let details = $state('');
  let stateText = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);
  let ok = $state(false);

  async function publish(e: Event) {
    e.preventDefault();
    err = null;
    ok = false;
    busy = true;
    try {
      await publishPresence({
        game: game.trim(),
        details: details.trim() || undefined,
        state: stateText.trim() || undefined,
        since: Math.floor(Date.now() / 1000)
      });
      ok = true;
    } catch (e) {
      err = e instanceof Error ? e.message : 'failed';
    } finally {
      busy = false;
    }
  }

  async function clear() {
    err = null;
    ok = false;
    busy = true;
    try {
      await clearPresence();
      game = '';
      details = '';
      stateText = '';
      ok = true;
    } catch (e) {
      err = e instanceof Error ? e.message : 'failed';
    } finally {
      busy = false;
    }
  }
</script>

<div class="mx-auto max-w-lg p-6 md:p-8">
  <h1 class="text-subtitle font-semibold text-content">Statut de jeu</h1>
  <p class="mt-2 text-body text-muted">
    Diffuse ce que tu fais à tous tes abonnés (façon Discord).
  </p>

  {#if $xmppState !== 'online'}
    <p class="mt-4 text-label text-warning">
      XMPP : {$xmppState}. La publication échouera tant que tu n'es pas reconnecté.
    </p>
  {/if}

  <form onsubmit={publish} class="mt-6 space-y-4">
    <Input label="Jeu" required maxlength={64} placeholder="Valorant" bind:value={game} />
    <Input label="Détails" maxlength={128} placeholder="Compétitif" bind:value={details} />
    <Input label="État" maxlength={128} placeholder="Round 3" bind:value={stateText} />

    {#if err}<p class="text-label text-danger">{err}</p>{/if}
    {#if ok}<p class="text-label text-success">Publié.</p>{/if}

    <div class="flex gap-2">
      <Button type="submit" loading={busy}>Publier</Button>
      <Button type="button" variant="secondary" disabled={busy} onclick={clear}>Effacer</Button>
    </div>
  </form>
</div>
