# Deploying Krovara

Target: a single Linux VPS (≥ 2 vCPU, 4 GB RAM, 40 GB disk) with Docker
and Docker Compose v2. Tested on Debian 12.

## 1. DNS

Point an A record at your VPS:

```
krovara.example.com.    A    1.2.3.4
```

If you want XMPP federation, also publish SRV records (optional for MVP):

```
_xmpp-client._tcp.krovara.example.com. SRV 5 0 5222 krovara.example.com.
_xmpp-server._tcp.krovara.example.com. SRV 5 0 5269 krovara.example.com.
```

## 2. Server prep

```bash
sudo apt update && sudo apt install -y docker.io docker-compose-plugin
sudo usermod -aG docker $USER && newgrp docker
```

Open ports 80, 443, 5222, 5269 in your firewall. The internal API listener
(8090) is loopback-only — never expose it.

## 3. Clone & configure

```bash
git clone https://github.com/<you>/krovara.git
cd krovara
cp .env.production.example .env
$EDITOR .env                  # fill REQUIRED values
```

Generate a JWT secret:

```bash
openssl rand -hex 32
```

## 4. First boot

```bash
docker compose pull           # if you push images to a registry; otherwise skipped
docker compose build          # local build of api/worker/voip/web
docker compose up -d
docker compose logs -f caddy  # watch Let's Encrypt cert issuance
```

Caddy will obtain a TLS cert on first launch — DNS must already resolve.

## 5. Verify

```bash
curl -sSf https://krovara.example.com/api/healthz   # TODO: endpoint
docker compose ps             # everything should be "running (healthy)"
```

Open `https://krovara.example.com/` in a browser, register a user, create
a space, send a message in #general from two browsers. That's the
acceptance test for session 15.

## 6. Updates

```bash
git pull
docker compose build
docker compose up -d
```

Migrations run automatically (the `migrate` service is `condition:
service_completed_successfully` for the `api` and `worker` services).

## 7. Backups (minimal)

```bash
docker exec krovara-postgres-1 pg_dump -U krovara krovara | gzip > krovara-$(date +%F).sql.gz
docker run --rm -v krovara_krovara-files:/data -v $(pwd):/backup alpine \
    tar czf /backup/krovara-files-$(date +%F).tar.gz -C /data .
```

Caddy's `/data` volume holds your TLS certs — back it up before
re-imaging the VPS to avoid hitting Let's Encrypt's rate limits.

## 8. Caveats / TODO

- The `cmd/api/main.go` HTTP server is a stub today (session 14 ships the
  worker; the API HTTP wiring lands in a follow-up). All Go packages
  compile and ship a binary; the binary just doesn't bind a listener yet.
  Bring up the API only when its `main` is wired (sessions 14b / 15b).
- Backups above are placeholders — wire to off-site storage before going
  live with real users.
- Federation (s2s on :5269) is open but Prosody dialback is permissive in
  the dev config; harden before exposing.
