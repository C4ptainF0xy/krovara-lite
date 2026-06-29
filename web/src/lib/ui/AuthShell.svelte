<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    children
  }: { title: string; subtitle: string; children: Snippet } = $props();

  const slogan = 'Connect instantly.';
  const tagline = 'Ton espace. Tes conversations. En temps réel.';
</script>

<div class="fixed inset-0 grid lg:grid-cols-2">
  <aside
    class="relative hidden overflow-hidden lg:flex lg:flex-col lg:justify-between lg:p-12
           bg-gradient-to-br from-base via-surface to-elevated"
  >
    <div
      class="pointer-events-none absolute -left-32 top-1/3 size-[28rem] rounded-full
             bg-primary/20 blur-[120px]"
      aria-hidden="true"
    ></div>

    <a href="/" class="relative flex items-center gap-3">
      <img src="/krovara.png" alt="" width="40" height="40" class="size-10 rounded-lg" />
      <span class="text-subtitle font-bold tracking-tight">Krovara</span>
    </a>

    <div class="relative max-w-md">
      <p class="text-[2.5rem] font-bold leading-tight tracking-tight">{slogan}</p>
      <p class="mt-4 text-subtitle text-muted">{tagline}</p>
    </div>

    <p class="relative text-label text-muted/70">
      © {new Date().getFullYear()} Krovara · Messagerie temps réel
    </p>
  </aside>

  <main
    class="flex items-center justify-center overflow-y-auto px-6 pt-[calc(1.5rem+var(--safe-top))] pb-[calc(1.5rem+var(--safe-bottom))] pl-[calc(1.5rem+var(--safe-left))] pr-[calc(1.5rem+var(--safe-right))]"
  >
    <div class="w-full max-w-sm animate-fade-in">
      <div class="mb-8 flex flex-col items-center text-center lg:items-start lg:text-left">
        <img
          src="/krovara.png"
          alt="Krovara"
          width="56"
          height="56"
          class="mb-4 size-14 rounded-xl shadow-lg shadow-primary/15 lg:hidden"
        />
        <h1 class="text-title font-bold tracking-tight">{title}</h1>
        <p class="mt-1 text-body text-muted">{subtitle}</p>
      </div>

      {@render children()}
    </div>
  </main>
</div>
