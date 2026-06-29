<script lang="ts">
  import { onDestroy } from 'svelte';
  import { newPeer, type Peer } from '$lib/voip/peer';
  import { getTurnCredentials, type TurnCredential } from '$lib/voip/turn';

  let peer = $state<Peer | null>(null);
  let cred = $state<TurnCredential | null>(null);
  let err = $state<string | null>(null);
  let busy = $state(false);

  let peerState = $state<string>('idle');
  let counts = $state({ host: 0, srflx: 0, relay: 0, other: 0 });

  async function start() {
    err = null;
    busy = true;
    try {
      cred = await getTurnCredentials();
      const p = await newPeer();
      peer = p;
      p.state.subscribe((s) => (peerState = s));
      p.iceCounts.subscribe((c) => (counts = c));
      p.pc.createDataChannel('probe');
      await p.createOffer();
    } catch (e) {
      err = e instanceof Error ? e.message : 'failed';
    } finally {
      busy = false;
    }
  }

  function stop() {
    peer?.close();
    peer = null;
    peerState = 'idle';
    counts = { host: 0, srflx: 0, relay: 0, other: 0 };
  }

  onDestroy(stop);

  const relayOK = $derived(counts.relay > 0);
</script>

<h1 class="text-2xl font-semibold">VoIP TURN probe</h1>
<p class="mt-2 text-sm text-slate-400">
  Tests that the browser can fetch credentials from /api/voip/turn-credentials
  and actually gather ICE candidates against Coturn. No call is placed —
  this only validates the network/auth path. Use it after deploying or
  changing Coturn config.
</p>

<div class="mt-6 flex gap-2">
  <button
    type="button"
    disabled={busy || peer !== null}
    onclick={start}
    class="rounded bg-violet-600 px-3 py-2 text-sm font-medium hover:bg-violet-500 disabled:opacity-50"
  >
    {busy ? '…' : 'Probe'}
  </button>
  {#if peer}
    <button
      type="button"
      onclick={stop}
      class="rounded ring-1 ring-slate-700 px-3 py-2 text-sm hover:bg-slate-900"
    >
      Stop
    </button>
  {/if}
</div>

{#if err}
  <p class="mt-4 rounded bg-red-950 px-3 py-2 text-sm text-red-300">{err}</p>
{/if}

{#if cred}
  <section class="mt-6 rounded ring-1 ring-slate-800 bg-slate-950 p-4">
    <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">Credentials</h2>
    <dl class="mt-2 grid grid-cols-3 gap-x-4 gap-y-1 text-xs">
      <dt class="text-slate-500">URIs</dt>
      <dd class="col-span-2 font-mono break-all text-slate-300">{cred.uris.join(', ') || 'STUN-only fallback'}</dd>
      <dt class="text-slate-500">Username</dt>
      <dd class="col-span-2 font-mono text-slate-300">{cred.username}</dd>
      <dt class="text-slate-500">TTL</dt>
      <dd class="col-span-2 text-slate-300">{cred.ttl_seconds}s</dd>
    </dl>
  </section>
{/if}

{#if peer}
  <section class="mt-4 rounded ring-1 ring-slate-800 bg-slate-950 p-4">
    <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">ICE gathering</h2>
    <p class="mt-1 text-xs text-slate-500">State: <code class="text-slate-300">{peerState}</code></p>
    <ul class="mt-3 grid grid-cols-4 gap-2 text-center text-sm">
      <li class="rounded bg-slate-900 px-2 py-3">
        <p class="text-2xl font-semibold text-slate-200">{counts.host}</p>
        <p class="text-xs text-slate-500">host</p>
      </li>
      <li class="rounded bg-slate-900 px-2 py-3">
        <p class="text-2xl font-semibold text-slate-200">{counts.srflx}</p>
        <p class="text-xs text-slate-500">srflx (STUN)</p>
      </li>
      <li class="rounded bg-slate-900 px-2 py-3 ring-1 {relayOK ? 'ring-emerald-600' : 'ring-slate-800'}">
        <p class="text-2xl font-semibold {relayOK ? 'text-emerald-400' : 'text-slate-200'}">{counts.relay}</p>
        <p class="text-xs text-slate-500">relay (TURN)</p>
      </li>
      <li class="rounded bg-slate-900 px-2 py-3">
        <p class="text-2xl font-semibold text-slate-200">{counts.other}</p>
        <p class="text-xs text-slate-500">other</p>
      </li>
    </ul>
    <p class="mt-3 text-xs text-slate-500">
      Relay candidates &gt; 0 means Coturn is reachable and accepting the HMAC
      credential. If you see only host/srflx, check the firewall and
      <code>turnserver.conf</code>'s <code>listening-ip</code> / <code>relay-ip</code>.
    </p>
  </section>
{/if}
