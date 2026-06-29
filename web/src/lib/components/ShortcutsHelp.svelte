<script lang="ts">
  import { Keyboard } from '@lucide/svelte';
  import { Dialog } from 'bits-ui';

  let open = $state(false);

  type Shortcut = { keys: string[]; label: string };
  const GROUPS: { title: string; items: Shortcut[] }[] = [
    {
      title: 'Navigation',
      items: [
        { keys: ['Ctrl', 'K'], label: 'Aller à un salon ou un espace' },
        { keys: ['Échap'], label: 'Fermer une fenêtre / quitter le focus' },
        { keys: ['?'], label: 'Afficher cette aide' }
      ]
    },
    {
      title: 'Affichage',
      items: [{ keys: ['Ctrl', '.'], label: 'Basculer le mode focus' }]
    },
    {
      title: 'Conversation',
      items: [
        { keys: ['Entrée'], label: 'Envoyer le message' },
        { keys: ['Maj', 'Entrée'], label: 'Saut de ligne' },
        { keys: ['Ctrl', 'B / I / E'], label: 'Gras / italique / code' }
      ]
    },
    {
      title: 'Commandes (au début du message)',
      items: [
        { keys: ['/shrug'], label: '¯\\_(ツ)_/¯' },
        { keys: ['/tableflip'], label: '(╯°□°)╯︵ ┻━┻' },
        { keys: ['/unflip'], label: '┬─┬ ノ( ゜-゜ノ)' },
        { keys: ['/lenny'], label: '( ͡° ͜ʖ ͡°)' }
      ]
    }
  ];

  function onWindowKeydown(e: KeyboardEvent) {
    if (e.key !== '?') return;
    const el = e.target as HTMLElement | null;
    const typing =
      !!el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName));
    if (typing) return;
    e.preventDefault();
    open = true;
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-fade-in" />
    <Dialog.Content
      class="fixed left-1/2 top-1/2 z-50 w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2
             rounded-lg border border-border bg-surface shadow-2xl shadow-black/40 data-[state=open]:animate-fade-in"
    >
      <div class="flex items-center gap-2 border-b border-border px-5 py-4">
        <Keyboard size={18} class="text-muted" />
        <Dialog.Title class="text-subtitle font-semibold">Raccourcis clavier</Dialog.Title>
      </div>
      <div class="space-y-5 px-5 py-4">
        {#each GROUPS as g (g.title)}
          <div>
            <h3 class="mb-2 text-label font-semibold uppercase tracking-wide text-muted">{g.title}</h3>
            <ul class="space-y-1.5">
              {#each g.items as sc (sc.label)}
                <li class="flex items-center justify-between gap-3">
                  <span class="text-body text-content">{sc.label}</span>
                  <span class="flex shrink-0 items-center gap-1">
                    {#each sc.keys as k (k)}
                      <kbd class="rounded border border-border bg-base px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted">{k}</kbd>
                    {/each}
                  </span>
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
