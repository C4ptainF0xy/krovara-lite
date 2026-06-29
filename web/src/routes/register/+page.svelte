<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { ArrowRight } from '@lucide/svelte';
  import { env } from '$env/dynamic/public';
  import { api, ApiError, loadMe } from '$lib/api';
  import { setSession } from '$lib/stores/auth';
  import { t } from '$lib/i18n';
  import { AuthShell, Button, Input, OAuthButtons } from '$lib/ui';

  let username = $state('');
  let email = $state('');
  let password = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);

  const siteKey = env.PUBLIC_TURNSTILE_SITEKEY ?? '';
  let captchaToken = $state('');
  let widgetEl = $state<HTMLDivElement | null>(null);
  let widgetId = '';

  type TurnstileAPI = {
    render: (
      el: HTMLElement,
      opts: { sitekey: string; callback: (t: string) => void; 'expired-callback'?: () => void; theme?: string }
    ) => string;
    reset: (id?: string) => void;
  };
  function resetCaptcha() {
    captchaToken = '';
    const w = window as unknown as { turnstile?: TurnstileAPI };
    if (siteKey && w.turnstile) w.turnstile.reset(widgetId || undefined);
  }
  onMount(() => {
    if (!siteKey) return;
    const w = window as unknown as { turnstile?: TurnstileAPI };
    const renderWidget = () => {
      if (widgetEl && w.turnstile) {
        widgetId = w.turnstile.render(widgetEl, {
          sitekey: siteKey,
          theme: 'dark',
          callback: (t) => (captchaToken = t),
          'expired-callback': () => (captchaToken = '')
        });
      }
    };
    if (w.turnstile) {
      renderWidget();
      return;
    }
    const script = document.createElement('script');
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
    script.async = true;
    script.defer = true;
    script.onload = renderWidget;
    document.head.appendChild(script);
  });

  type TokenResp = {
    access_token: string;
    refresh_token: string;
    access_expires_at: string;
  };

  async function submit(e: Event) {
    e.preventDefault();
    err = null;
    busy = true;
    try {
      const r = await api<TokenResp>('/api/auth/register', {
        method: 'POST',
        body: { username, email, password, captcha_token: captchaToken }
      });
      setSession({
        accessToken: r.access_token,
        refreshToken: r.refresh_token,
        accessExpiresAt: r.access_expires_at
      });
      try {
        localStorage.setItem('krovara.show_onboarding', '1');
      } catch {
      }
      await loadMe().catch(() => {});
      await goto('/app');
    } catch (e) {
      err = e instanceof ApiError ? e.message : $t('auth.register.error');
      resetCaptcha();
    } finally {
      busy = false;
    }
  }
</script>

<AuthShell title={$t('auth.register.title')} subtitle={$t('auth.register.subtitle')}>
  <form onsubmit={submit} class="space-y-4">
    <Input
      label={$t('auth.field.username')}
      required
      minlength={3}
      maxlength={32}
      autocomplete="username"
      placeholder="ton_pseudo"
      bind:value={username}
    />
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
      autocomplete="new-password"
      placeholder="8 caractères minimum"
      bind:value={password}
      error={err}
    />

    {#if siteKey}
      <div bind:this={widgetEl} class="flex justify-center"></div>
    {/if}

    <Button type="submit" full size="lg" loading={busy} disabled={busy || (!!siteKey && !captchaToken)}>
      {#if !busy}{$t('auth.register.submit')} <ArrowRight size={18} />{/if}
    </Button>
  </form>

  <div class="my-5 flex items-center gap-3 text-label text-muted">
    <span class="h-px flex-1 bg-border"></span>
    {$t('auth.or')}
    <span class="h-px flex-1 bg-border"></span>
  </div>

  <OAuthButtons />

  <p class="mt-6 text-center text-body text-muted lg:text-left">
    {$t('auth.haveAccount')}
    <a href="/login" class="font-medium text-accent hover:text-content">{$t('auth.goLogin')}</a>
  </p>
</AuthShell>
