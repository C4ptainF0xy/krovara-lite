<script lang="ts">

  type Props = {
    width: number;
    min: number;
    max: number;
    edge: 'left' | 'right';
    onresize: (px: number) => void;
    label?: string;
  };
  let { width, min, max, edge, onresize, label = 'Redimensionner le volet' }: Props = $props();

  let dragging = $state(false);

  function clamp(v: number) {
    return Math.min(max, Math.max(min, v));
  }

  function splitter(node: HTMLElement) {
    let startX = 0;
    let startW = 0;

    const onPointerDown = (e: PointerEvent) => {
      dragging = true;
      startX = e.clientX;
      startW = width;
      node.setPointerCapture(e.pointerId);
      e.preventDefault();
    };
    const onPointerMove = (e: PointerEvent) => {
      if (!dragging) return;
      const delta = e.clientX - startX;
      onresize(clamp(edge === 'right' ? startW + delta : startW - delta));
    };
    const onPointerUp = (e: PointerEvent) => {
      if (!dragging) return;
      dragging = false;
      try {
        node.releasePointerCapture(e.pointerId);
      } catch {
      }
    };
    const onWindowUp = () => {
      if (dragging) dragging = false;
    };
    const onKeyDown = (e: KeyboardEvent) => {
      const step = e.shiftKey ? 48 : 16;
      const grow = edge === 'right' ? 'ArrowRight' : 'ArrowLeft';
      const shrink = edge === 'right' ? 'ArrowLeft' : 'ArrowRight';
      if (e.key === grow) {
        onresize(clamp(width + step));
        e.preventDefault();
      } else if (e.key === shrink) {
        onresize(clamp(width - step));
        e.preventDefault();
      }
    };

    node.addEventListener('pointerdown', onPointerDown);
    node.addEventListener('pointermove', onPointerMove);
    node.addEventListener('pointerup', onPointerUp);
    node.addEventListener('keydown', onKeyDown);
    window.addEventListener('pointerup', onWindowUp);
    window.addEventListener('pointercancel', onWindowUp);
    return {
      destroy() {
        node.removeEventListener('pointerdown', onPointerDown);
        node.removeEventListener('pointermove', onPointerMove);
        node.removeEventListener('pointerup', onPointerUp);
        node.removeEventListener('keydown', onKeyDown);
        window.removeEventListener('pointerup', onWindowUp);
        window.removeEventListener('pointercancel', onWindowUp);
      }
    };
  }
</script>

{#if dragging}
  <div class="fixed inset-0 z-[100] cursor-col-resize" aria-hidden="true"></div>
{/if}

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div
  use:splitter
  role="separator"
  aria-orientation="vertical"
  aria-label={label}
  aria-valuenow={Math.round(width)}
  aria-valuemin={min}
  aria-valuemax={max}
  tabindex="0"
  class="group relative z-10 -mx-0.5 w-1 shrink-0 cursor-col-resize touch-none
         focus-visible:outline-none {dragging ? 'bg-brand/60' : ''}"
>
  <span
    class="absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-border transition-colors duration-150
           group-hover:bg-brand/60 group-focus-visible:bg-brand {dragging ? 'bg-brand' : ''}"
  ></span>
</div>
