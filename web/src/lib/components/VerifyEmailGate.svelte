<script lang="ts">
  import { MailWarning, LogOut } from '@lucide/svelte';
  import { api, loadMe } from '$lib/api';
  import { auth, clearSession } from '$lib/stores/auth';
  import { goto } from '$app/navigation';

  const email = $derived($auth.user?.email ?? '');

  let code = $state('');
  let busy = $state(false);
  let msg = $state<string | null>(null);
  let err = $state<string | null>(null);

  async function verify(e: Event) {
    e.preventDefault();
    const c = code.trim();
    if (c.length !== 6) { err = 'Entre le code à 6 chiffres.'; return; }
    busy = true; err = null; msg = null;
    try {
      await api('/api/auth/verify-email', { method: 'POST', body: { email, code: c } });
      await loadMe().catch(() => {});
    } catch {
      err = 'Code invalide ou expiré.';
    } finally { busy = false; }
  }

  async function resend() {
    busy = true; err = null; msg = null;
    try {
      await api('/api/auth/resend-verification', { method: 'POST', body: { email } });
      msg = 'Nouveau code envoyé. Vérifie ta boîte (et les spams).';
    } catch {
      err = "Impossible d'envoyer le code.";
    } finally { busy = false; }
  }

  async function logout() {
    try { await api('/api/auth/logout', { method: 'POST' }); } catch {}
    clearSession();
    await goto('/login');
  }
</script>

<div class="grid h-[100dvh] place-items-center px-4" style="padding-top:var(--safe-top);padding-bottom:var(--safe-bottom)">
  <div class="w-full max-w-sm text-center">
    <div class="mx-auto mb-5 grid size-14 place-items-center rounded-2xl bg-warning/15 text-warning">
      <MailWarning size={28} />
    </div>
    <h1 class="text-subtitle font-bold text-content">Vérifie ton adresse email</h1>
    <p class="mt-2 text-body text-muted">
      On a envoyé un code à 6 chiffres à <span class="font-medium text-content">{email}</span>.
      Entre-le pour accéder à Krovara.
    </p>

    <form onsubmit={verify} class="mt-6 space-y-3">
      <input
        bind:value={code}
        inputmode="numeric"
        maxlength="6"
        placeholder="123456"
        class="h-12 w-full rounded-lg border border-border bg-base/50 text-center text-title tracking-[0.4em] text-content outline-none focus:border-primary"
      />
      {#if err}<p class="text-label text-danger">{err}</p>{/if}
      {#if msg}<p class="text-label text-success">{msg}</p>{/if}
      <button type="submit" disabled={busy} class="h-11 w-full rounded-lg bg-primary font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50">
        Valider
      </button>
    </form>

    <div class="mt-4 flex items-center justify-center gap-4 text-label">
      <button type="button" onclick={resend} disabled={busy} class="text-accent hover:text-content disabled:opacity-50">Renvoyer le code</button>
      <button type="button" onclick={logout} class="flex items-center gap-1 text-muted hover:text-content"><LogOut size={13} /> Se déconnecter</button>
    </div>
  </div>
</div>
