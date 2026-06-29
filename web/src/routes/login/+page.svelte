<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { ArrowRight, KeyRound } from '@lucide/svelte';
  import { api, ApiError, loadMe } from '$lib/api';
  import { setSession } from '$lib/stores/auth';
  import { t } from '$lib/i18n';
  import { AuthShell, Button, Input, OAuthButtons } from '$lib/ui';

  let email = $state('');
  let password = $state('');
  let code = $state('');
  let tempToken = $state('');
  let requires2FA = $state(false);
  let busy = $state(false);
  let err = $state<string | null>(null);

  function returnTarget(): string {
    const raw = page.url.searchParams.get('return') ?? '';
    return raw.startsWith('/') && !raw.startsWith('//') ? raw : '/app';
  }

  type TokenResp = {
    access_token: string;
    refresh_token: string;
    access_expires_at: string;
    requires_2fa?: boolean;
    temp_token?: string;
  };

  async function submit(e: Event) {
    e.preventDefault();
    err = null;
    busy = true;
    try {
      if (requires2FA) {
        const r = await api<TokenResp>('/api/auth/login/2fa', {
          method: 'POST',
          body: { temp_token: tempToken, code }
        });
        finishLogin(r);
      } else {
        const r = await api<TokenResp>('/api/auth/login', {
          method: 'POST',
          body: { email, password }
        });
        if (r.requires_2fa && r.temp_token) {
          requires2FA = true;
          tempToken = r.temp_token;
        } else {
          finishLogin(r);
        }
      }
    } catch (e) {
      err = e instanceof ApiError ? e.message : $t('auth.login.error');
    } finally {
      busy = false;
    }
  }

  async function finishLogin(r: TokenResp) {
    setSession({
      accessToken: r.access_token,
      refreshToken: r.refresh_token,
      accessExpiresAt: r.access_expires_at
    });
    await loadMe().catch(() => {});
    await goto(returnTarget());
  }
</script>

<AuthShell title={$t('auth.login.title')} subtitle={$t('auth.login.subtitle')}>
  <form onsubmit={submit} class="space-y-4">
    {#if !requires2FA}
      <Input
        label={$t('auth.field.email')}
        type="email"
        required
        autocomplete="email"
        placeholder="toi@exemple.com"
        bind:value={email}
      />
      <Input
        label={$t('auth.field.password')}
        type="password"
        required
        minlength={8}
        autocomplete="current-password"
        placeholder="••••••••"
        bind:value={password}
        error={err}
      />

      <Button type="submit" full size="lg" loading={busy}>
        {#if !busy}{$t('auth.login.submit')} <ArrowRight size={18} />{/if}
      </Button>

      <div class="my-5 flex items-center gap-3 text-label text-muted">
        <span class="h-px flex-1 bg-border"></span>
        {$t('auth.or')}
        <span class="h-px flex-1 bg-border"></span>
      </div>

      <OAuthButtons />

      <p class="mt-6 text-center text-body text-muted lg:text-left">
        {$t('auth.noAccount')}
        <a href="/register" class="font-medium text-accent hover:text-content">{$t('auth.goRegister')}</a>
      </p>
    {:else}
      <div class="mb-4 text-center">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-brand/20 text-brand mb-4">
          <KeyRound size={24} />
        </div>
        <p class="text-body text-muted">Saisis le code à 6 chiffres de ton application d'authentification ou un code de secours.</p>
      </div>

      <Input
        label="Code d'authentification (2FA)"
        type="text"
        required
        autocomplete="one-time-code"
        placeholder="123456"
        bind:value={code}
        error={err}
        class="text-center tracking-widest text-lg font-mono"
      />

      <Button type="submit" full size="lg" loading={busy}>
        Valider le code
      </Button>

      <button
        type="button"
        onclick={() => { requires2FA = false; err = null; }}
        class="mt-4 w-full text-center text-sm text-muted hover:text-content"
      >
        Annuler et retourner à la connexion
      </button>
    {/if}
  </form>
</AuthShell>
