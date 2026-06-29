# VoIP (session 17 — partial)

What ships now:

- `turn.ts` — `getTurnCredentials()` + `toICEServers()`. The Go API gates
  this behind the auth middleware, so creds can't be farmed anonymously.
- Backend Coturn service + `/api/voip/turn-credentials` (see
  `internal/voip/`). HMAC verified by Coturn against its
  `static-auth-secret`.
- `peer.ts` — `RTCPeerConnection` wrapper with reactive stores
  (`state`, `iceCounts`, `localStream`, `remoteStream`) and a Promise API
  for offer/answer/ICE. No signaling — the wrapper expects someone to
  drive the SDP exchange from outside.
- `/app/voip/test` — diagnostic page that fetches creds and gathers ICE
  candidates. Relay count > 0 proves Coturn is reachable through the
  deployment's firewall, without needing two browsers and a real call.

What's intentionally **not** shipped yet (deferred):

- Jingle signaling over XMPP. `@xmpp/client` doesn't bundle a Jingle
  module; we need either a third-party plugin or hand-rolled `<iq>` handlers
  for `session-initiate` / `session-accept` / `session-terminate` and
  trickle-ICE (XEP-0176). `peer.ts` is shaped so the future signaling
  layer just forwards SDP/ICE between the local methods (`createOffer`,
  `acceptOffer`, `addRemoteIce`) and Jingle `<iq>` stanzas.
- Call UI (incoming-call modal, in-call panel, hangup/mute/camera).

Pick this up when you can spend a chunk of contiguous time on it —
piecemeal call-signaling is brittle.
