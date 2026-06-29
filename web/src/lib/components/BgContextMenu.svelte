<script lang="ts">
  import type { Component } from 'svelte';

  type Item = { label: string; icon?: Component; onclick: () => void; danger?: boolean };
  let {
    open = $bindable(false),
    x = 0,
    y = 0,
    items = []
  }: { open?: boolean; x?: number; y?: number; items?: Item[] } = $props();

  const show = $derived(open && items.length > 0);

  const MENU_W = 180;
  const rowH = 32;
  const left = $derived(
    typeof window !== 'undefined' ? Math.max(6, Math.min(x, window.innerWidth - MENU_W - 6)) : x
  );
  const top = $derived(
    typeof window !== 'undefined'
      ? Math.max(6, Math.min(y, window.innerHeight - items.length * rowH - 14))
      : y
  );

  let menuEl = $state<HTMLDivElement | null>(null);
  function onPointerDown(e: PointerEvent) {
    if (!open) return;
    if (menuEl && menuEl.contains(e.target as Node)) return;
    open = false;
  }
</script>

<svelte:window
  onpointerdowncapture={onPointerDown}
  onkeydowncapture={(e) => {
    if (open && e.key === 'Escape') open = false;
  }}
  onscrollcapture={() => {
    if (open) open = false;
  }}
  onresize={() => {
    if (open) open = false;
  }}
/>

{#if show}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    bind:this={menuEl}
    role="menu"
    tabindex="-1"
    class="fixed z-[61] min-w-44 rounded-lg border border-border bg-overlay p-1 shadow-xl"
    style="left:{left}px;top:{top}px"
    oncontextmenu={(e) => {
      e.preventDefault();
      open = false;
    }}
  >
    {#each items as it (it.label)}
      {@const Icon = it.icon}
      <button
        type="button"
        onclick={() => {
          open = false;
          it.onclick();
        }}
        class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-label transition-colors hover:bg-elevated
               {it.danger ? 'text-danger hover:bg-danger/10' : 'text-content'}"
      >
        {#if Icon}<Icon size={14} class={it.danger ? '' : 'text-muted'} />{/if}
        {it.label}
      </button>
    {/each}
  </div>
{/if}
