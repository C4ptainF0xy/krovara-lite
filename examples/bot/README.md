# Krovara bot example

Minimal XEP-0114 component that replies `pong` to `!ping`.

## Provision

```bash
curl -X POST https://krovara.example.com/api/spaces/<space>/bots \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name":"pingbot"}'
```

Response (the `secret` is shown **once**):

```json
{
  "id": "…",
  "component_jid": "bot-abc123de.krovara.example.com",
  "secret": "…",
  "connect": { "host": "xmpp.krovara.example.com", "port": 5347, "domain": "bot-abc123de.…", "secret": "…" }
}
```

## Register with Prosody

Edit `prosody/components.cfg.lua`:

```lua
Component "bot-abc123de.krovara.example.com"
    component_secret = "<secret>"
```

Reload:

```bash
docker compose exec prosody prosodyctl reload
```

## Run the bot

This folder is a **separate Go module** (so its mellium.im dependency
doesn't bloat the main API binaries). Run it from inside the folder:

```bash
cd examples/bot
go mod tidy            # one-time
export BOT_JID=bot-abc123de.krovara.example.com
export BOT_SECRET=<secret>
export BOT_HOST=localhost
export BOT_PORT=5347
go run .
```

The example code uses APIs from the version of `mellium.im/xmpp` that
exists at build time; if upstream renames anything, the `go mod tidy` step
will surface it. The code in `main.go` may need small adjustments — treat
it as a **starting point**, not a working binary out of the box.

## Caveats

- `mellium.im/xmpp` is intentionally low-level. This example handles
  `!ping` only; production bots should also reply to presence probes and
  follow MUC join semantics if they're meant to live inside channels.
- This is an **example**, not a `cmd/`. It's not built or tested in CI
  (the import isn't part of the API/worker binaries).
