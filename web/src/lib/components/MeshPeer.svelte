<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { ActiveSession } from '$lib/voip/signaling';
  import type { SfuPeer } from '$lib/voip/sfu';

  type Props = { session?: ActiveSession; sfuPeer?: SfuPeer };
  let { session, sfuPeer }: Props = $props();

  let audioEl: HTMLAudioElement | null = $state(null);
  let connState = $state('connecting');
  let unsubState: (() => void) | null = null;
  let unsubStream: (() => void) | null = null;

  function cleanup() {
    unsubState?.();
    unsubStream?.();
    unsubState = unsubStream = null;
  }

  $effect(() => {
    cleanup();
    if (session) {
      unsubState = session.peer.state.subscribe((v) => (connState = v));
      unsubStream = session.peer.remoteStream.subscribe((s) => {
        if (audioEl) audioEl.srcObject = s;
      });
    } else if (sfuPeer) {
      connState = 'connected';
      if (audioEl) audioEl.srcObject = sfuPeer.stream;
    }
  });

  onDestroy(cleanup);

  const label = $derived(
    session
      ? session.peerJID.split('@')[0].slice(0, 8)
      : (sfuPeer?.peerId ?? '').slice(0, 8)
  );
  const connected = $derived(connState === 'connected');
</script>

<div class="flex items-center gap-2 rounded bg-slate-900 px-2 py-1.5 ring-1 {connected ? 'ring-emerald-600' : 'ring-slate-800'}">
  <span class="h-2 w-2 rounded-full {connected ? 'bg-emerald-500' : 'bg-amber-500'}"></span>
  <span class="truncate text-xs font-mono text-slate-300">{label}</span>
  <audio bind:this={audioEl} autoplay></audio>
</div>
