<script lang="ts">
  import { Dialog } from 'bits-ui';
  import { X } from '@lucide/svelte';

  type Props = {
    open: boolean;
    title: string;
    onclose: () => void;
    wide?: boolean;
    flush?: boolean;
    children?: import('svelte').Snippet;
  };
  let { open, title, onclose, wide = false, flush = false, children }: Props = $props();
</script>

<Dialog.Root
  {open}
  onOpenChange={(v) => {
    if (!v) onclose();
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay
      class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm
             data-[state=open]:animate-fade-in"
    />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 z-50 w-[calc(100vw-2rem)] {wide ? 'max-w-2xl' : 'max-w-md'} -translate-x-1/2 -translate-y-1/2
             max-h-[calc(100dvh-2rem-var(--safe-top)-var(--safe-bottom))] overflow-y-auto
             rounded-lg border border-border bg-surface shadow-2xl shadow-black/40
             data-[state=open]:animate-fade-in"
    >
      <div class="flex items-center justify-between border-b border-border px-5 py-4">
        <Dialog.Title class="text-subtitle font-semibold">{title}</Dialog.Title>
        <Dialog.Close
          class="grid size-7 place-items-center rounded text-muted
                 transition-colors duration-150 hover:bg-elevated hover:text-content"
        >
          <X size={16} />
        </Dialog.Close>
      </div>
      <div class={flush ? '' : 'px-5 py-4'}>
        {@render children?.()}
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
