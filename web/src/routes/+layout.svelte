<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { auth, hasSession } from '$lib/stores/auth';
  import { loadMe } from '$lib/api';
  import { checkForUpdates } from '$lib/updater';
  import { isNative } from '$lib/native';

  let { children } = $props();

  onMount(() => void checkForUpdates());

  function handleOAuthDeepLink(raw: string): boolean {
    if (!raw.includes('/oauth/')) return false;
    try {
      const u = new URL(raw);
      const provider = u.pathname.split('/').filter(Boolean)[0];
      if (!provider) return false;
      void goto(`/oauth/${provider}/callback${u.search}${u.hash}`);
      return true;
    } catch {
      return false;
    }
  }
  onMount(() => {
    if (!isNative()) return;
    let unlisten: (() => void) | undefined;
    void (async () => {
      try {
        const dl = await import('@tauri-apps/plugin-deep-link');
        try {
          const current = await dl.getCurrent();
          if (current) for (const raw of current) if (handleOAuthDeepLink(raw)) break;
        } catch {
        }
        unlisten = await dl.onOpenUrl((urls) => {
          for (const raw of urls) if (handleOAuthDeepLink(raw)) break;
        });
      } catch (e) {
        console.warn('[deep-link]', e);
      }
    })();
    return () => unlisten?.();
  });

  onMount(async () => {
    if (!hasSession()) return;
    try {
      await loadMe();
    } catch {
    }
  });

  const publicRoutes = ['/login', '/register'];
  $effect(() => {
    const onPublic = publicRoutes.includes(page.url.pathname) || page.url.pathname.startsWith('/oauth/');
    if (!$auth.accessToken && !hasSession() && !onPublic) {
      goto('/login');
    }
  });
</script>

{@render children()}
