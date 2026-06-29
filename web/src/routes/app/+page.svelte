<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { Users, MessageSquare, Inbox, Bookmark, Gamepad2 } from '@lucide/svelte';
  import { inboxUnread } from '$lib/stores/inbox';
  import { dmUnread } from '$lib/stores/dm';
  import { incoming, loadRequests } from '$lib/stores/friends';

  onMount(() => {
    void loadRequests();
  });
  import OnboardingGuide from '$lib/components/OnboardingGuide.svelte';
  import DmList from '$lib/components/home/DmList.svelte';
  import DmThread from '$lib/components/home/DmThread.svelte';
  import GroupThread from '$lib/components/home/GroupThread.svelte';
  import FriendsPanel from '$lib/components/home/FriendsPanel.svelte';
  import InboxPanel from '$lib/components/home/InboxPanel.svelte';
  import SavedPanel from '$lib/components/home/SavedPanel.svelte';

  type Tab = 'messages' | 'amis' | 'inbox' | 'saved';
  const TABS: Tab[] = ['messages', 'amis', 'inbox', 'saved'];

  const tab = $derived.by<Tab>(() => {
    const t = page.url.searchParams.get('tab');
    return (TABS as string[]).includes(t ?? '') ? (t as Tab) : 'messages';
  });

  function setTab(k: Tab) {
    void goto(`/app?tab=${k}`, { replaceState: true, keepFocus: true, noScroll: true });
  }

  let selectedPeer = $state<string | null>(null);
  let selectedGroup = $state<string | null>(null);
  function pickPeer(id: string) {
    selectedGroup = null;
    selectedPeer = id;
    if (tab !== 'messages') setTab('messages');
  }
  function pickGroup(id: string) {
    selectedPeer = null;
    selectedGroup = id;
    if (tab !== 'messages') setTab('messages');
  }

  let lastDmParam: string | null = null;
  $effect(() => {
    const dm = page.url.searchParams.get('dm');
    if (dm && dm !== lastDmParam) {
      lastDmParam = dm;
      selectedGroup = null;
      selectedPeer = dm;
    }
  });
  let lastGroupParam: string | null = null;
  $effect(() => {
    const g = page.url.searchParams.get('group');
    if (g && g !== lastGroupParam) {
      lastGroupParam = g;
      selectedPeer = null;
      selectedGroup = g;
    }
  });

  const dmTotalUnread = $derived(Object.values($dmUnread).reduce((n, v) => n + v, 0));

  const showRailMobile = $derived(tab === 'messages' && !selectedPeer);

  const TAB_META = [
    { k: 'amis' as Tab, label: 'Amis', icon: Users },
    { k: 'messages' as Tab, label: 'Messages', icon: MessageSquare },
    { k: 'inbox' as Tab, label: 'Boîte de réception', icon: Inbox },
    { k: 'saved' as Tab, label: 'Enregistrés', icon: Bookmark }
  ];
</script>

<OnboardingGuide />

<div class="flex h-full min-h-0 flex-col">
  <nav class="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border px-2 py-1.5">
    {#each TAB_META as m (m.k)}
      {@const Icon = m.icon}
      {@const badge = m.k === 'inbox' ? $inboxUnread : m.k === 'messages' ? dmTotalUnread : m.k === 'amis' ? $incoming.length : 0}
      <button
        type="button"
        onclick={() => setTab(m.k)}
        aria-current={tab === m.k ? 'page' : undefined}
        class="relative flex shrink-0 items-center gap-2 rounded-md px-3 py-1.5 text-body transition-colors duration-150
               {tab === m.k ? 'bg-elevated text-content' : 'text-muted hover:bg-elevated/60 hover:text-content'}"
      >
        <Icon size={16} class={tab === m.k ? 'text-content' : 'text-muted'} />
        <span class="hidden sm:inline">{m.label}</span>
        {#if badge > 0}
          <span class="grid h-4 min-w-4 place-items-center rounded-full bg-danger px-1 text-[0.625rem] font-semibold text-white">{badge > 9 ? '9+' : badge}</span>
        {/if}
      </button>
    {/each}
    <a
      href="/app/games"
      class="ml-auto flex shrink-0 items-center gap-2 rounded-md px-3 py-1.5 text-body text-muted transition-colors duration-150 hover:bg-elevated/60 hover:text-content"
    >
      <Gamepad2 size={16} class="text-muted" />
      <span class="hidden sm:inline">Jeux</span>
    </a>
  </nav>

  <div class="flex min-h-0 flex-1">
    <aside
      class="shrink-0 border-r border-border bg-surface md:flex md:w-72
             {showRailMobile ? 'flex w-full' : 'hidden'}"
    >
      <DmList selected={selectedGroup ?? selectedPeer} onpick={pickPeer} onpickgroup={pickGroup} />
    </aside>

    <div class="min-w-0 flex-1 overflow-y-auto {showRailMobile ? 'hidden md:block' : 'block'}">
      {#if tab === 'messages'}
        {#if selectedGroup}
          <GroupThread groupId={selectedGroup} onback={() => (selectedGroup = null)} />
        {:else if selectedPeer}
          <DmThread peerId={selectedPeer} onback={() => (selectedPeer = null)} />
        {:else}
          <div class="grid h-full place-items-center px-6 text-center">
            <div class="max-w-sm">
              <div class="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-surface text-muted">
                <MessageSquare size={26} />
              </div>
              <p class="text-body text-muted">Choisis une conversation à gauche pour discuter.</p>
            </div>
          </div>
        {/if}
      {:else if tab === 'amis'}
        <FriendsPanel heading={false} />
      {:else if tab === 'inbox'}
        <InboxPanel heading={false} />
      {:else if tab === 'saved'}
        <SavedPanel />
      {/if}
    </div>
  </div>
</div>
