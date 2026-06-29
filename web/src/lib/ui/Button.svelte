<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
  type Size = 'sm' | 'md' | 'lg';

  interface Props extends HTMLButtonAttributes {
    variant?: Variant;
    size?: Size;
    loading?: boolean;
    full?: boolean;
    children: Snippet;
  }

  let {
    variant = 'primary',
    size = 'md',
    loading = false,
    full = false,
    disabled,
    class: klass = '',
    children,
    ...rest
  }: Props = $props();

  const variants: Record<Variant, string> = {
    primary: 'bg-primary text-white hover:bg-primary-hover',
    secondary: 'bg-elevated text-content hover:bg-border-strong',
    ghost: 'bg-transparent text-muted hover:bg-surface hover:text-content',
    danger: 'bg-danger text-white hover:brightness-110'
  };
  const sizes: Record<Size, string> = {
    sm: 'h-8 px-3 text-label',
    md: 'h-10 px-4 text-label',
    lg: 'h-11 px-5 text-body'
  };
</script>

<button
  class="inline-flex select-none items-center justify-center gap-2 rounded font-medium
         transition-[background-color,filter] duration-150 ease-smooth
         focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50
         {variants[variant]} {sizes[size]} {full ? 'w-full' : ''} {klass}"
  disabled={disabled || loading}
  {...rest}
>
  {#if loading}
    <span
      class="size-4 animate-spin rounded-full border-2 border-white/30 border-t-white"
      aria-hidden="true"
    ></span>
  {/if}
  {@render children()}
</button>
