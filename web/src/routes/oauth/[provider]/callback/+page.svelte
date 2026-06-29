<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { setSession } from '$lib/stores/auth';
  import { api, loadMe, ApiError } from '$lib/api';
  import { Button, Input } from '$lib/ui';

  let err = $state<string | null>(null);
  let signupToken = $state<string | null>(null);
  let username = $state('');
  let submitting = $state(false);
  let formErr = $state<string | null>(null);
  let twoFAToken = $state<string | null>(null);
  let code2fa = $state('');

  const USERNAME_RE = /^[a-zA-Z0-9._-]+$/;

  async function applySession(t: { access_token: string; refresh_token: string; access_expires_at: string }) {
    setSession({
      accessToken: t.access_token,
      refreshToken: t.refresh_token,
      accessExpiresAt: t.access_expires_at
    });
    await loadMe().catch(() => {});
    await goto('/app');
  }

  async function submitUsername(e: Event) {
    e.preventDefault();
    const u = username.trim().toLowerCase();
    if (u.length < 3 || u.length > 32 || !USERNAME_RE.test(u)) {
      formErr = 'Pseudo invalide : 3-32 caractères, lettres, chiffres, . _ - (sans espaces).';
      return;
    }
    submitting = true;
    formErr = null;
    try {
      const t = await api<{ access_token: string; refresh_token: string; access_expires_at: string }>(
        '/api/auth/complete',
        { method: 'POST', body: { signup_token: signupToken, username: u } }
      );
      try {
        localStorage.setItem('krovara.show_onboarding', '1');
      } catch {
      }
      await applySession(t);
    } catch (e2) {
      if (e2 instanceof ApiError && e2.status === 409) formErr = 'Ce pseudo est déjà pris.';
      else if (e2 instanceof ApiError && e2.status === 401) formErr = 'Session expirée, relance la connexion.';
      else formErr = e2 instanceof Error ? e2.message : 'Échec de la création du compte.';
    } finally {
      submitting = false;
    }
  }

  onMount(async () => {
    const params = new URLSearchParams(page.url.search || page.url.hash.replace(/^#/, '?'));

    const st = params.get('signup_token');
    if (st) {
      signupToken = st;
      username = params.get('suggested') ?? '';
      return;
    }

    if (params.get('requires_2fa') === '1') {
      twoFAToken = params.get('temp_token');
      return;
    }

    const access = params.get('access_token');
    const refresh = params.get('refresh_token');
    const exp = params.get('access_expires_at');
    if (!access || !refresh || !exp) {
      err = 'missing tokens in callback';
      return;
    }
    await applySession({ access_token: access, refresh_token: refresh, access_expires_at: exp });
  });

  async function submit2FA(e: Event) {
    e.preventDefault();
    const c = code2fa.trim();
    if (!c) return;
    submitting = true;
    formErr = null;
    try {
      const t = await api<{ access_token: string; refresh_token: string; access_expires_at: string }>(
        '/api/auth/login/2fa',
        { method: 'POST', body: { temp_token: twoFAToken, code: c } }
      );
      await applySession(t);
    } catch (e2) {
      formErr = e2 instanceof ApiError && e2.status === 401 ? 'Code invalide ou expiré.' : 'Échec de la vérification.';
    } finally {
      submitting = false;
    }
  }
</script>

<div class="grid min-h-screen place-items-center px-4">
  <div class="w-full max-w-sm animate-fade-in text-center">
    <img
      src="/krovara.png"
      alt=""
      width="56"
      height="56"
      class="mx-auto mb-5 size-14 rounded-2xl shadow-lg shadow-primary/15"
    />

    {#if err}
      <h1 class="text-subtitle font-bold">Connexion impossible</h1>
      <p class="mt-3 text-body text-danger">{err}</p>
      <a href="/login" class="mt-6 inline-block font-medium text-accent hover:text-content">
        Retour à la connexion
      </a>
    {:else if signupToken}
      <h1 class="text-subtitle font-bold text-content">Choisis ton pseudo</h1>
      <p class="mt-2 text-body text-muted">
        Dernière étape avant de rejoindre Krovara. Tu pourras le changer plus tard.
      </p>
      <form onsubmit={submitUsername} class="mt-6 space-y-3 text-left">
        <Input label="Pseudo" bind:value={username} placeholder="ex. ton_pseudo" />
        {#if formErr}<p class="text-label text-danger">{formErr}</p>{/if}
        <Button type="submit" loading={submitting} class="w-full">Créer mon compte</Button>
      </form>
    {:else if twoFAToken}
      <h1 class="text-subtitle font-bold text-content">Vérification en deux étapes</h1>
      <p class="mt-2 text-body text-muted">Saisis le code de ton application d'authentification.</p>
      <form onsubmit={submit2FA} class="mt-6 space-y-3">
        <input bind:value={code2fa} inputmode="numeric" maxlength="8" placeholder="123456"
               class="h-11 w-full rounded-lg border border-border bg-base/50 text-center text-subtitle tracking-[0.3em] text-content outline-none focus:border-primary" />
        {#if formErr}<p class="text-label text-danger">{formErr}</p>{/if}
        <Button type="submit" loading={submitting} class="w-full">Valider</Button>
      </form>
    {:else}
      <p class="flex items-center justify-center gap-2 text-body text-muted">
        <span class="size-4 animate-spin rounded-full border-2 border-muted/30 border-t-accent"></span>
        Connexion en cours…
      </p>
    {/if}
  </div>
</div>
