<script lang="ts">
  import { onDestroy } from 'svelte';
  import { X, MessagesSquare, Bell, BellOff } from '@lucide/svelte';
  import { messages, type ChannelMessage } from '$lib/stores/messages';
  import { reactions, myEmojis } from '$lib/stores/reactions';
  import { rememberReaction } from '$lib/stores/recentReactions';
  import { recentReactions } from '$lib/stores/recentReactions';
  import { memberNames } from '$lib/stores/members';
  import { joinMUC, leaveMUC, sendMessage, sendReactions, fetchHistory } from '$lib/xmpp/muc';
  import { xmppState } from '$lib/xmpp/client';
  import {
    threadRoom,
    touchThread,
    loadThreads,
    subscribeThread,
    unsubscribeThread,
    type Thread
  } from '$lib/stores/threads';
  import MessageList from './MessageList.svelte';
  import MessageInput from './MessageInput.svelte';

  type Mentionable = { username: string; name: string };

  type Props = {
    thread: Thread | null;
    spaceId: string;
    selfId?: string;
    selfUsername?: string;
    mentionUsernames?: Set<string>;
    mentionRoles?: Map<string, string | null>;
    mentionables?: Mentionable[];
    onclose: () => void;
  };
  let {
    thread,
    spaceId,
    selfId,
    selfUsername,
    mentionUsernames,
    mentionRoles,
    mentionables = [],
    onclose
  }: Props = $props();

  const mentionRoleNames = $derived(mentionRoles ? [...mentionRoles.keys()] : []);

  const room = $derived(thread ? threadRoom(thread.id) : '');
  const threadMessages = $derived(room ? ($messages[room] ?? []) : []);

  let joinedFor: string | null = null;
  async function joinIfReady() {
    if (!room || $xmppState !== 'online') return;
    if (joinedFor === room) return;
    if (joinedFor && joinedFor !== room) {
      try {
        await leaveMUC(joinedFor);
      } catch {}
    }
    try {
      await joinMUC(room);
      joinedFor = room;
      void fetchHistory(room, 50);
    } catch (e) {
      console.warn('join thread MUC failed', e);
    }
  }
  $effect(() => {
    void room;
    void $xmppState;
    void joinIfReady();
  });
  onDestroy(() => {
    if (joinedFor) {
      const r = joinedFor;
      joinedFor = null;
      void leaveMUC(r);
    }
  });

  let replyTarget = $state<ChannelMessage | null>(null);
  function authorOf(m: ChannelMessage): string {
    const uid = m.fromResource || m.from;
    return $memberNames[uid] ?? uid;
  }

  async function handleSend(text: string) {
    if (!room || !thread) return;
    const replyToId = replyTarget?.id;
    replyTarget = null;
    await sendMessage(room, text, { replyToId });
    await touchThread(thread.id);
    void loadThreads(thread.channel_id).catch(() => {});
  }

  async function handleReact(m: ChannelMessage, emoji: string) {
    if (!room || !selfId) return;
    const cur = myEmojis(m.id, selfId);
    const adding = !cur.includes(emoji);
    const next = adding ? [...cur, emoji] : cur.filter((e) => e !== emoji);
    if (adding) rememberReaction(emoji);
    await sendReactions(room, m.id, next);
  }

  async function toggleSubscribe() {
    if (!thread) return;
    try {
      if (thread.is_subscribed) await unsubscribeThread(thread.channel_id, thread.id);
      else await subscribeThread(thread.channel_id, thread.id);
    } catch (e) {
      console.error('thread subscribe', e);
    }
  }
</script>

{#if thread}
  <aside
    class="flex h-full w-full shrink-0 animate-slide-in flex-col border-l border-border bg-surface md:w-[24rem]"
    aria-label="Fil de discussion"
  >
    <header class="flex items-center gap-2 border-b border-border px-4 py-3">
      <MessagesSquare size={18} class="shrink-0 text-muted" />
      <div class="min-w-0 flex-1">
        <h2 class="truncate text-body font-semibold leading-tight">{thread.title}</h2>
        <p class="truncate text-label text-muted">Fil de discussion</p>
      </div>
      <button
        type="button"
        title={thread.is_subscribed ? 'Ne plus suivre' : 'Suivre ce fil'}
        aria-pressed={thread.is_subscribed}
        onclick={toggleSubscribe}
        class="grid size-8 place-items-center rounded-md transition-colors duration-150 hover:bg-elevated
               {thread.is_subscribed ? 'text-accent' : 'text-muted hover:text-content'}"
      >
        {#if thread.is_subscribed}<Bell size={17} />{:else}<BellOff size={17} />{/if}
      </button>
      <button
        type="button"
        title="Fermer le fil"
        aria-label="Fermer le fil"
        onclick={onclose}
        class="grid size-8 place-items-center rounded-md text-muted transition-colors duration-150 hover:bg-elevated hover:text-content"
      >
        <X size={18} />
      </button>
    </header>

    <div class="relative flex flex-1 flex-col overflow-hidden">
      {#key room}
        <MessageList
          messages={threadMessages}
          {selfId}
          {selfUsername}
          {mentionUsernames}
          {mentionRoles}
          {spaceId}
          reactions={$reactions}
          recentEmojis={$recentReactions}
          onreact={handleReact}
          onreply={(m) => (replyTarget = m)}
        />
      {/key}
      <MessageInput
        onsend={handleSend}
        {spaceId}
        disabled={$xmppState !== 'online'}
        placeholder="Répondre dans le fil…"
        replyName={replyTarget ? authorOf(replyTarget) : null}
        oncancelreply={() => (replyTarget = null)}
        {mentionables}
        mentionableRoles={mentionRoleNames}
      />
    </div>
  </aside>
{/if}
