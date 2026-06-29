<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import {
    currentDirect,
    hangup,
    onIncomingCall,
    placeCall,
    sessions,
    type ActiveSession,
    type SessionInfo
  } from '$lib/voip/signaling';
  import { activeRoom, leaveVoice } from '$lib/voip/room';
  import {
    cameraOn,
    screenOn,
    micOn,
    toggleMic,
    enableCamera,
    disableCamera,
    enableScreenShare,
    disableScreenShare
  } from '$lib/voip/sfu';
  import { voiceMode } from '$lib/voip/mode';
  import MeshPeer from './MeshPeer.svelte';
  import VideoGrid from './VideoGrid.svelte';
  import { PhoneOff, Mic, MicOff, Video, VideoOff, MonitorUp, Phone, PhoneCall, Check, X } from '@lucide/svelte';
  import { sfuState } from '$lib/voip/sfu';

  async function toggleCam() {
    if ($cameraOn) await disableCamera();
    else await enableCamera().catch((e) => console.error('camera', e));
  }
  async function toggleScreen() {
    if ($screenOn) await disableScreenShare();
    else await enableScreenShare().catch((e) => console.error('screen', e));
  }

  const meshPeers = $derived($sessions.filter((s) => s.room && s.room === $activeRoom));

  let session = $state<ActiveSession | null>(null);
  let peerState = $state('idle');
  let localStream = $state<MediaStream | null>(null);
  let remoteStream = $state<MediaStream | null>(null);
  let muted = $state(false);
  let cameraOff = $state(false);

  let incoming = $state<{
    info: SessionInfo;
    accept: () => Promise<void>;
    reject: () => void;
  } | null>(null);

  let localVideo: HTMLVideoElement | null = $state(null);
  let remoteVideo: HTMLVideoElement | null = $state(null);

  let unsubIncoming: (() => void) | null = null;
  let unsubState: (() => void) | null = null;
  let unsubLocal: (() => void) | null = null;
  let unsubRemote: (() => void) | null = null;

  onMount(() => {
    unsubIncoming = onIncomingCall((info, accept, reject) => {
      incoming = { info, accept, reject };
    });
  });

  onDestroy(() => {
    unsubIncoming?.();
    detach();
  });

  function detach() {
    unsubState?.();
    unsubLocal?.();
    unsubRemote?.();
    unsubState = unsubLocal = unsubRemote = null;
  }

  function attach(s: ActiveSession) {
    unsubState = s.peer.state.subscribe((v) => (peerState = v));
    unsubLocal = s.peer.localStream.subscribe((v) => {
      localStream = v;
      if (localVideo) localVideo.srcObject = v;
    });
    unsubRemote = s.peer.remoteStream.subscribe((v) => {
      remoteStream = v;
      if (remoteVideo) remoteVideo.srcObject = v;
    });
  }

  async function start(peerJID: string) {
    if (session) return;
    try {
      const s = await placeCall(peerJID);
      session = s;
      attach(s);
    } catch (err) {
      console.error('placeCall failed', err);
    }
  }

  async function accept() {
    if (!incoming) return;
    const inc = incoming;
    incoming = null;
    await inc.accept();
    const s = currentDirect();
    if (s) {
      session = s;
      attach(s);
    }
  }

  function reject() {
    incoming?.reject();
    incoming = null;
  }

  async function end() {
    if (!session) return;
    await hangup(session.sid);
    detach();
    session = null;
    peerState = 'idle';
    localStream = null;
    remoteStream = null;
  }

  function toggleMute() {
    if (!localStream) return;
    muted = !muted;
    localStream.getAudioTracks().forEach((t) => (t.enabled = !muted));
  }

  function toggleCamera() {
    if (!localStream) return;
    cameraOff = !cameraOff;
    localStream.getVideoTracks().forEach((t) => (t.enabled = !cameraOff));
  }

  $effect(() => {
    (window as unknown as { krovaraCall?: (jid: string) => void }).krovaraCall = start;
    return () => {
      delete (window as unknown as { krovaraCall?: (jid: string) => void }).krovaraCall;
    };
  });
</script>

{#if $activeRoom}
  <div
    class="fixed z-40 flex flex-col rounded-lg border border-border bg-[#1E1F22] shadow-2xl shadow-black/40 overflow-hidden left-2 right-2 bottom-[calc(0.5rem+var(--safe-bottom))] md:left-20 md:right-auto md:bottom-4 md:w-64"
  >
    <div class="flex flex-col p-2 bg-[#2B2D31]">
      <div class="flex items-center gap-2 px-1">
        <div class="grid size-6 place-items-center">
          <span class="flex size-3.5 rounded-full bg-success {$sfuState === 'connected' ? '' : 'animate-pulse opacity-50'}"></span>
        </div>
        <div class="flex flex-col min-w-0 flex-1">
          <span class="text-xs font-bold text-success truncate">
            {$sfuState === 'connected' ? 'Voix connectée' : ($sfuState === 'failed' ? 'Voix déconnectée' : 'Connexion...')}
          </span>
          <span class="text-[10px] font-medium text-muted truncate">
            Salon {$activeRoom.slice(0, 8)}…
          </span>
        </div>
        <button
          type="button"
          onclick={() => leaveVoice()}
          title="Se déconnecter"
          class="grid size-7 shrink-0 place-items-center rounded bg-transparent text-muted transition-colors hover:bg-surface-hover hover:text-content"
        >
          <PhoneOff size={16} />
        </button>
      </div>

      {#if voiceMode !== 'sfu'}
        <div class="mt-2 space-y-1 px-1">
          {#each meshPeers as p (p.sid)}
            <MeshPeer session={p} />
          {/each}
          {#if meshPeers.length === 0}
            <p class="text-[10px] text-muted italic">Seul(e). Clique sur 📞 pour appeler.</p>
          {/if}
        </div>
      {/if}
    </div>

    <div class="flex gap-1 justify-between p-2 bg-[#232428] border-t border-border">
      <button
        type="button"
        onclick={toggleCam}
        title={$cameraOn ? 'Désactiver la caméra' : 'Activer la caméra'}
        class="grid flex-1 h-8 place-items-center rounded transition-colors {$cameraOn ? 'text-content hover:bg-surface-active' : 'text-muted hover:bg-surface-hover hover:text-content'}"
      >
        {#if $cameraOn}<Video size={18} />{:else}<VideoOff size={18} />{/if}
      </button>
      <button
        type="button"
        onclick={toggleScreen}
        title={$screenOn ? 'Arrêter le partage' : 'Partager l\'écran'}
        class="grid flex-1 h-8 place-items-center rounded transition-colors {$screenOn ? 'text-content hover:bg-surface-active' : 'text-muted hover:bg-surface-hover hover:text-content'}"
      >
        <MonitorUp size={18} />
      </button>
      <button
        type="button"
        onclick={() => toggleMic()}
        title={$micOn ? 'Désactiver le micro' : 'Activer le micro'}
        class="grid flex-1 h-8 place-items-center rounded transition-colors {$micOn ? 'text-content hover:bg-surface-active' : 'text-danger hover:bg-danger/20'}"
      >
        {#if $micOn}<Mic size={18} />{:else}<MicOff size={18} />{/if}
      </button>
    </div>
  </div>
{/if}

{#if incoming}
  <div class="fixed inset-x-0 top-[calc(1rem+var(--safe-top))] z-50 mx-auto w-full max-w-sm px-2 rounded-lg border border-border bg-surface shadow-2xl shadow-black/50 p-5 flex flex-col gap-4 animate-in slide-in-from-top-4">
    <div class="flex items-center justify-between">
      <div class="flex flex-col">
        <h3 class="text-lg font-bold text-content">Appel entrant</h3>
        <p class="text-sm text-muted break-all">{incoming.info.peerJID}</p>
      </div>
      <div class="grid size-10 place-items-center rounded-full bg-brand/20 text-brand">
        <PhoneCall size={20} class="animate-pulse" />
      </div>
    </div>
    <div class="flex gap-3">
      <button
        type="button"
        onclick={accept}
        class="flex-1 flex justify-center items-center gap-2 rounded-md bg-success hover:bg-success-hover px-4 py-2 text-sm font-semibold text-white transition-colors"
      >
        <Check size={16} /> Répondre
      </button>
      <button
        type="button"
        onclick={reject}
        class="flex-1 flex justify-center items-center gap-2 rounded-md bg-danger hover:bg-danger-hover px-4 py-2 text-sm font-semibold text-white transition-colors"
      >
        <X size={16} /> Refuser
      </button>
    </div>
  </div>
{/if}

{#if session}
  <div class="fixed bottom-[calc(1rem+var(--safe-bottom))] right-4 z-40 w-80 max-w-[calc(100vw-2rem)] overflow-hidden rounded-lg border border-border bg-surface shadow-2xl shadow-black/40">
    <div class="relative aspect-video bg-black/50">
      <video
        bind:this={remoteVideo}
        autoplay
        playsinline
        class="h-full w-full object-cover"
      ></video>
      <video
        bind:this={localVideo}
        autoplay
        muted
        playsinline
        class="absolute bottom-2 right-2 h-16 w-24 rounded-md object-cover border border-border shadow-lg"
      ></video>
      <span class="absolute top-2 left-2 rounded-md bg-black/60 px-2 py-0.5 text-xs font-semibold uppercase tracking-wider text-white backdrop-blur-sm">
        {peerState}
      </span>
    </div>
    <div class="flex items-center justify-between gap-2 px-4 py-3 bg-surface">
      <span class="truncate text-sm font-semibold text-content">{session.peerJID}</span>
      <div class="flex gap-2">
        <button
          type="button"
          onclick={toggleMute}
          title={muted ? 'Activer le micro' : 'Désactiver le micro'}
          class="grid size-8 place-items-center rounded-full transition-colors {muted ? 'bg-danger text-white' : 'bg-surface-hover text-content hover:bg-surface-active'}"
        >
          {#if muted}<MicOff size={14} />{:else}<Mic size={14} />{/if}
        </button>
        <button
          type="button"
          onclick={toggleCamera}
          title={cameraOff ? 'Activer la caméra' : 'Désactiver la caméra'}
          class="grid size-8 place-items-center rounded-full transition-colors {cameraOff ? 'bg-surface-hover text-muted hover:text-content hover:bg-surface-active' : 'bg-surface-active text-content hover:bg-surface-hover'}"
        >
          {#if cameraOff}<VideoOff size={14} />{:else}<Video size={14} />{/if}
        </button>
        <button
          type="button"
          onclick={end}
          title="Raccrocher"
          class="grid size-8 place-items-center rounded-full bg-danger text-white transition-colors hover:bg-danger-hover"
        >
          <PhoneOff size={14} />
        </button>
      </div>
    </div>
  </div>
{/if}
