# Prosody (dev)

XMPP server for Krovara messaging. Production config = same shape with real
TLS certs (session 15).

## Bring it up

```bash
make dev-prosody-certs   # one-time: self-signed certs for krovara.local
make dev-up              # postgres + prosody
make prosody-logs        # tail Prosody logs
```

`make dev-up` runs the cert script first, so the very first `dev-up` works.

## Create a user

```bash
make prosody-adduser USER=alice PASSWORD=secret
```

Connect with any XMPP client (Conversations, Gajim, Dino) using:

- JID: `alice@krovara.local`
- Server: `localhost`
- Port: `5222` (STARTTLS) — accept the self-signed cert.

## WebSocket

`ws://localhost:5280/xmpp-websocket` — used by the Svelte client (session 13)
and by `make prosody-smoke` (the Go smoke test).

## Storage

Prosody owns its `prosody_*` tables in the `krovara` Postgres DB. We never
edit them from migrations. To wipe state:

```bash
make dev-down
docker volume rm krovara-new_krovara-prosody-data
```

## Architecture

- `VirtualHost krovara.local` — accounts live here.
- `conference.krovara.local` — MUC (XEP-0045), one room per channel.
- `pubsub.krovara.local` — PEP/PubSub (XEP-0060), used for rich presence.
- `upload.krovara.local` — HTTP file share (XEP-0363).

Modules enabled cover the XEP list in `docs/ARCHITECTURE.md` minus OMEMO
(client-side) and Jingle (session 17).

## Session 09 will

- Replace `internal_hashed` auth with a JWT/HTTP shim talking to the Go API
  (`mod_auth_http` or `mod_auth_token`).
- Ensure MAM/MUC mirror creation of Krovara channels.
- Wire ban → XMPP session kill.
