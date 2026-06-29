<script lang="ts">
  import { Check } from '@lucide/svelte';
  import { prefs, setPrefs, type Theme, type Scale, type Density } from '$lib/stores/prefs';
  import { locale, LOCALES, t as tr } from '$lib/i18n';

  const THEMES = $derived<{ value: Theme; label: string; desc: string; swatch: string }[]>([
    { value: 'dark', label: $tr('appearance.theme.dark'), desc: $tr('appearance.theme.darkDesc'), swatch: '#0f0f14' },
    { value: 'oled', label: $tr('appearance.theme.oled'), desc: $tr('appearance.theme.oledDesc'), swatch: '#000000' },
    { value: 'light', label: $tr('appearance.theme.light'), desc: $tr('appearance.theme.lightDesc'), swatch: '#f6f6fa' }
  ]);

  let cssDraft = $state($prefs.customCss);
  let cssApplied = $state(false);
  function applyCss() {
    setPrefs({ customCss: cssDraft });
    cssApplied = true;
    setTimeout(() => (cssApplied = false), 1500);
  }
  function clearCss() {
    cssDraft = '';
    setPrefs({ customCss: '' });
  }

  const SCALES = $derived<{ value: Scale; label: string }[]>([
    { value: 'compact', label: $tr('appearance.scale.compact') },
    { value: 'normal', label: $tr('appearance.scale.normal') },
    { value: 'large', label: $tr('appearance.scale.large') }
  ]);

  const DENSITIES = $derived<{ value: Density; label: string; desc: string }[]>([
    { value: 'cosy', label: $tr('appearance.density.cosy'), desc: $tr('appearance.density.cosyDesc') },
    { value: 'compact', label: $tr('appearance.density.compact'), desc: $tr('appearance.density.compactDesc') }
  ]);
</script>

<div class="space-y-10">
  <section>
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.theme.title')}</h2>
    <p class="mt-1 text-body text-muted">{$tr('appearance.theme.hint')}</p>
    <div class="mt-4 grid gap-2 sm:grid-cols-2">
      {#each THEMES as t (t.value)}
        {@const active = $prefs.theme === t.value}
        <button
          type="button"
          onclick={() => setPrefs({ theme: t.value })}
          class="flex items-center gap-3 rounded-lg border p-3 text-left transition-colors duration-150 ease-smooth
                 {active ? 'border-primary bg-primary/10' : 'border-border hover:border-border-strong'}"
        >
          <span
            class="size-9 shrink-0 rounded-lg border border-border"
            style="background:{t.swatch}"
          ></span>
          <span class="min-w-0">
            <span class="block text-body font-medium text-content">{t.label}</span>
            <span class="block text-label text-muted">{t.desc}</span>
          </span>
          {#if active}<Check size={16} class="ml-auto shrink-0 text-accent" />{/if}
        </button>
      {/each}
    </div>
  </section>

  <section>
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.scale.title')}</h2>
    <p class="mt-1 text-body text-muted">{$tr('appearance.scale.hint')}</p>
    <div class="mt-4 inline-flex rounded-lg border border-border p-1">
      {#each SCALES as s (s.value)}
        {@const active = $prefs.scale === s.value}
        <button
          type="button"
          onclick={() => setPrefs({ scale: s.value })}
          class="rounded px-4 py-1.5 text-label font-medium transition-colors duration-150 ease-smooth
                 {active ? 'bg-primary text-white' : 'text-muted hover:text-content'}"
        >
          {s.label}
        </button>
      {/each}
    </div>
  </section>

  <section>
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.density.title')}</h2>
    <p class="mt-1 text-body text-muted">{$tr('appearance.density.hint')}</p>
    <div class="mt-4 grid gap-2 sm:grid-cols-2">
      {#each DENSITIES as d (d.value)}
        {@const active = $prefs.density === d.value}
        <button
          type="button"
          onclick={() => setPrefs({ density: d.value })}
          class="rounded-lg border p-3 text-left transition-colors duration-150 ease-smooth
                 {active ? 'border-primary bg-primary/10' : 'border-border hover:border-border-strong'}"
        >
          <span class="flex items-center gap-2">
            <span class="text-body font-medium text-content">{d.label}</span>
            {#if active}<Check size={16} class="ml-auto shrink-0 text-accent" />{/if}
          </span>
          <span class="mt-0.5 block text-label text-muted">{d.desc}</span>
        </button>
      {/each}
    </div>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.a11y.title')}</h2>
    <label class="mt-4 flex cursor-pointer items-center justify-between gap-4 rounded-lg border border-border p-3">
      <span class="min-w-0">
        <span class="block text-body text-content">{$tr('appearance.a11y.reduceMotion')}</span>
        <span class="block text-label text-muted">{$tr('appearance.a11y.reduceMotionDesc')}</span>
      </span>
      <input
        type="checkbox"
        checked={$prefs.reduceMotion}
        onchange={(e) => setPrefs({ reduceMotion: e.currentTarget.checked })}
        class="size-5 shrink-0 cursor-pointer accent-primary"
      />
    </label>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$tr('settings.appearance.language')}</h2>
    <p class="mt-1 text-body text-muted">{$tr('settings.appearance.languageHint')}</p>
    <div class="mt-4 inline-flex rounded-lg border border-border p-1">
      {#each LOCALES as l (l.value)}
        {@const active = $locale === l.value}
        <button
          type="button"
          onclick={() => locale.set(l.value)}
          class="rounded px-4 py-1.5 text-label font-medium transition-colors duration-150 ease-smooth
                 {active ? 'bg-primary text-white' : 'text-muted hover:text-content'}"
        >
          {l.label}
        </button>
      {/each}
    </div>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.advanced.title')}</h2>
    <label class="mt-4 flex cursor-pointer items-center justify-between gap-4 rounded-lg border border-border p-3">
      <span class="min-w-0">
        <span class="block text-body text-content">{$tr('appearance.advanced.devMode')}</span>
        <span class="block text-label text-muted">{$tr('appearance.advanced.devModeDesc')}</span>
      </span>
      <input
        type="checkbox"
        checked={$prefs.developerMode}
        onchange={(e) => setPrefs({ developerMode: e.currentTarget.checked })}
        class="size-5 shrink-0 cursor-pointer accent-primary"
      />
    </label>
  </section>

  <section>
    <h2 class="text-subtitle font-semibold text-content">{$tr('appearance.css.title')}</h2>
    <p class="mt-1 text-body text-muted">{$tr('appearance.css.hint')}</p>
    <textarea
      bind:value={cssDraft}
      spellcheck="false"
      rows="8"
      placeholder={$tr('appearance.css.placeholder')}
      class="mt-4 w-full resize-y rounded-lg border border-border bg-base px-3 py-2 font-mono text-label text-content
             outline-none focus:border-primary"
    ></textarea>
    <div class="mt-2 flex items-center gap-2">
      <button
        type="button"
        onclick={applyCss}
        class="rounded-md bg-primary px-4 py-1.5 text-label font-medium text-white transition-colors duration-150 hover:bg-primary-hover"
      >
        {$tr('appearance.css.apply')}
      </button>
      <button
        type="button"
        onclick={clearCss}
        class="rounded-md border border-border px-4 py-1.5 text-label font-medium text-muted transition-colors duration-150 hover:text-content"
      >
        {$tr('appearance.css.clear')}
      </button>
      {#if cssApplied}
        <span class="flex items-center gap-1 text-label text-success">
          <Check size={14} /> {$tr('appearance.css.applied')}
        </span>
      {/if}
    </div>
  </section>
</div>
