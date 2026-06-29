<script lang="ts">
  import Button from './Button.svelte';
  import GoogleIcon from './icons/GoogleIcon.svelte';
  import GithubIcon from './icons/GithubIcon.svelte';
  import { apiUrl } from '$lib/config';
  import { isNative } from '$lib/native';

  async function oauth(provider: 'google' | 'github') {
    if (isNative()) {
      const { openUrl } = await import('@tauri-apps/plugin-opener');
      await openUrl(apiUrl(`/api/auth/${provider}?platform=app`));
      return;
    }
    window.location.href = apiUrl(`/api/auth/${provider}`);
  }
</script>

<div class="space-y-2">
  <Button variant="secondary" full onclick={() => oauth('google')}>
    <GoogleIcon /> Continuer avec Google
  </Button>
  <Button variant="secondary" full onclick={() => oauth('github')}>
    <GithubIcon /> Continuer avec GitHub
  </Button>
</div>
