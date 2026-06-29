<script lang="ts">
  import { MailWarning, X } from '@lucide/svelte';
  import { api, loadMe } from '$lib/api';
  import { auth } from '$lib/stores/auth';

  const email = $derived($auth.user?.email ?? '');

  let open = $state(false);
  let code = $state('');
  let busy = $state(false);
  let msg = $state<string | null>(null);
  let err = $state<string | null>(null);
  let dismissed = $state(false);

  async function verify(e: Event) {
    e.preventDefault();
    const c = code.trim();
    if (c.length !== 6) {
      err = 'Entre le code à 6 chiffres.';
      return;
    }
    busy = true;
    err = null;
    msg = null;
    try {
      await api('/api/auth/verify-email', { method: 'POST', body: { email, code: c } });
      await loadMe().catch(() => {});
      msg = 'Email vérifié ✅';
    } catch {
      err = 'Code invalide ou expiré.';
    } finally {
      busy = false;
    }
  }

  async function resend() {
    busy = true;
    err = null;
    msg = null;
    try {
      await api('/api/auth/resend-verification', { method: 'POST', body: { email } });
      msg = 'Nouveau code envoyé.';
    } catch {
      err = "Impossible d'envoyer le code.";
    } finally {
      busy = false;
    }
  }
</script>

{#if !dismissed}
  <div class="shrink-0 border-b border-warning/30 bg-warning/10 px-4 py-2 text-label text-content"
       style="padding-top:calc(0.5rem + var(--safe-top))">
    <div class="mx-auto flex max-w-3xl flex-wrap items-center gap-x-3 gap-y-2">
      <MailWarning size={16} class="shrink-0 text-warning" />
      <span class="min-w-0 flex-1">
        Vérifie ton adresse email{email ? ` (${email})` : ''} pour sécuriser ton compte.
      </span>
      {#if open}
        <form onsubmit={verify} class="flex items-center gap-1.5">
          <input
            bind:value={code}
            inputmode="numeric"
            maxlength="6"
            placeholder="123456"
            class="h-7 w-24 rounded border border-border bg-base/60 px-2 text-center tracking-widest text-content outline-none focus:border-primary"
          />
          <button type="submit" disabled={busy} class="rounded bg-primary px-2.5 py-1 font-medium text-white hover:bg-primary-hover disabled:opacity-50">Valider</button>
          <button type="button" onclick={resend} disabled={busy} class="rounded border border-border px-2.5 py-1 text-muted hover:text-content disabled:opacity-50">Renvoyer</button>
        </form>
      {:else}
        <button type="button" onclick={() => (open = true)} class="rounded bg-primary px-2.5 py-1 font-medium text-white hover:bg-primary-hover">Entrer le code</button>
      {/if}
      {#if msg}<span class="text-success">{msg}</span>{/if}
      {#if err}<span class="text-danger">{err}</span>{/if}
      <button type="button" onclick={() => (dismissed = true)} title="Plus tard" class="grid size-6 place-items-center rounded text-muted hover:text-content"><X size={14} /></button>
    </div>
  </div>
{/if}
