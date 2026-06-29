<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { User, Bell, Circle, Palette, Lock, KeyRound, Shield, Webhook, Sparkles, ChevronDown, HardDrive } from '@lucide/svelte';
  import { auth } from '$lib/stores/auth';
  import { t as tr } from '$lib/i18n';

  let { children } = $props();

  const groups = $derived(
    [
      {
        label: $tr('settings.group.user'),
        items: [
          { href: '/app/settings/profile', label: $tr('settings.nav.profile'), icon: User },
          { href: '/app/settings/status', label: $tr('settings.nav.status'), icon: Circle },
          { href: '/app/settings/account', label: $tr('settings.nav.account'), icon: KeyRound },
          { href: '/app/settings/subscription', label: $tr('settings.nav.subscription'), icon: Sparkles },
          { href: '/app/settings/storage', label: 'Stockage', icon: HardDrive },
          { href: '/app/settings/privacy', label: $tr('settings.nav.privacy'), icon: Lock }
        ]
      },
      {
        label: $tr('settings.group.app'),
        items: [
          { href: '/app/settings/appearance', label: $tr('settings.nav.appearance'), icon: Palette },
          { href: '/app/settings/devices', label: $tr('settings.nav.notifications'), icon: Bell },
          { href: '/app/settings/api', label: $tr('settings.nav.api'), icon: Webhook }
        ]
      },
      ...($auth.user?.is_admin
        ? [{ label: $tr('settings.group.admin'), items: [{ href: '/app/admin', label: $tr('settings.nav.admin'), icon: Shield }] }]
        : [])
    ]
  );
  const tabs = $derived(groups.flatMap((g) => g.items));

  const active = $derived(page.url.pathname);
  const isActive = (href: string) => active === href || active.startsWith(href + '/');
</script>

<div class="flex h-full min-h-0 bg-base">
  <nav class="hidden w-60 shrink-0 flex-col overflow-y-auto border-r border-border bg-surface px-3 py-6 sm:flex">
    <h1 class="px-2 pb-3 text-subtitle font-bold text-content">{$tr('settings.title')}</h1>
    {#each groups as g (g.label)}
      <p class="mt-4 px-2 pb-1 text-[0.6875rem] font-semibold uppercase tracking-wider text-muted/70 first:mt-0">
        {g.label}
      </p>
      {#each g.items as t (t.href)}
        {@const Icon = t.icon}
        <a
          href={t.href}
          aria-current={isActive(t.href) ? 'page' : undefined}
          class="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-body transition-colors duration-150
                 {isActive(t.href)
            ? 'bg-elevated font-medium text-content'
            : 'text-muted hover:bg-elevated/60 hover:text-content'}"
        >
          <Icon size={16} class="shrink-0" />
          {t.label}
        </a>
      {/each}
    {/each}
  </nav>

  <div class="flex min-w-0 flex-1 flex-col overflow-y-auto">
    <div class="border-b border-border px-3 py-2 sm:hidden">
      <label class="sr-only" for="settings-nav">Section des réglages</label>
      <div class="relative">
        <select
          id="settings-nav"
          value={tabs.find((t) => isActive(t.href))?.href ?? tabs[0]?.href}
          onchange={(e) => goto((e.currentTarget as HTMLSelectElement).value)}
          class="w-full appearance-none rounded-lg border border-border-strong bg-surface px-3 py-2.5 pr-9 text-body font-medium text-content outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          {#each groups as g (g.label)}
            <optgroup label={g.label}>
              {#each g.items as t (t.href)}
                <option value={t.href}>{t.label}</option>
              {/each}
            </optgroup>
          {/each}
        </select>
        <ChevronDown
          size={18}
          class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted"
        />
      </div>
    </div>

    <div class="w-full max-w-2xl px-6 pt-8 pb-[calc(2rem+var(--safe-bottom))] sm:px-10 sm:pt-10">
      {@render children()}
    </div>
  </div>
</div>
