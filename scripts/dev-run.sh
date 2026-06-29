#!/usr/bin/env bash
# Bring up the full dev stack:
#   docker compose -f docker-compose.dev.yml up -d
#   migrate up
#   go run ./cmd/api   in the background
#   go run ./cmd/worker in the background
#
# Loads env from .env.dev. Run from the repo root.

# This script targets the Windows Go toolchain via Git Bash (paths like
# /c/Program Files/Go/bin). When launched through WSL's bash — the default
# C:\Windows\System32\bash.exe that `bash ...` from cmd.exe resolves to — those
# paths live under /mnt/c and the toolchain isn't reachable. Detect WSL and
# re-exec under Git Bash so `bash scripts/dev-run.sh` works either way.
if grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    for gb in "/mnt/c/Program Files/Git/bin/bash.exe" "/mnt/c/Program Files/Git/usr/bin/bash.exe"; do
        [ -x "$gb" ] && exec "$gb" "$0" "$@"
    done
    echo "ERROR: lancé sous WSL et Git Bash introuvable." >&2
    echo '       Lance-le depuis Git Bash : "C:\Program Files\Git\bin\bash.exe" scripts/dev-run.sh' >&2
    exit 1
fi

set -euo pipefail
set -a
. ./.env.dev
set +a

# Make the Go toolchain + tools reachable even when this script is launched from
# a shell that didn't load them (e.g. `bash scripts/dev-run.sh` from cmd.exe,
# which gets a minimal PATH). go lives in the standard Windows install dir;
# migrate/sqlc are `go install`ed into GOPATH/bin (= $HOME/go/bin by default).
for d in "/c/Program Files/Go/bin" "$HOME/go/bin"; do
    [ -d "$d" ] && case ":$PATH:" in *":$d:"*) ;; *) PATH="$d:$PATH";; esac
done
export PATH

command -v go >/dev/null 2>&1 || { echo "ERROR: 'go' introuvable sur le PATH." >&2; exit 1; }

mkdir -p .dev-files .dev-logs

echo "==> docker compose"
docker compose -f docker-compose.dev.yml up -d

echo "==> waiting for postgres"
until docker exec krovara-postgres-dev pg_isready -U krovara -d krovara > /dev/null 2>&1; do
    sleep 1
done

echo "==> migrate up"
if command -v migrate >/dev/null 2>&1; then
    migrate -path migrations -database "postgres://krovara:krovara@localhost:5433/krovara?sslmode=disable" up || true
else
    echo "    ⚠️  'migrate' introuvable — migrations NON appliquées." >&2
    echo "    Installe-le : go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest" >&2
fi

# Build first (blocking) instead of `go run ... &`: a cold build takes ~1 min,
# far longer than any sleep, so a backgrounded `go run` gets killed mid-compile
# when this script exits and never binds its port. A prebuilt binary launches in
# ~1s and keeps running independently once the shell exits.
echo "==> compiling cmd/api + cmd/worker (première fois : ~1 min, ensuite ~2 s)"
go build -o .dev-files/api.exe    ./cmd/api
go build -o .dev-files/worker.exe ./cmd/worker

echo "==> starting cmd/api  (logs: .dev-logs/api.log)"
.dev-files/api.exe  > .dev-logs/api.log  2>&1 &
echo $! > .dev-logs/api.pid

echo "==> starting cmd/worker (logs: .dev-logs/worker.log)"
.dev-files/worker.exe > .dev-logs/worker.log 2>&1 &
echo $! > .dev-logs/worker.pid

api_url="http://localhost${KROVARA_HTTP_ADDR:-:8082}"
echo "==> waiting for API ($api_url/healthz)"
ready=0
for _ in $(seq 1 30); do
    if curl -fsS "$api_url/healthz" >/dev/null 2>&1; then ready=1; break; fi
    if ! kill -0 "$(cat .dev-logs/api.pid)" 2>/dev/null; then
        echo "    ⚠️  cmd/api s'est arrêté — dernières lignes de .dev-logs/api.log :" >&2
        tail -n 20 .dev-logs/api.log >&2
        break
    fi
    sleep 1
done

echo
if [ "$ready" = "1" ]; then
    echo "==> ready"
else
    echo "==> API PAS prête (timeout/crash) — voir .dev-logs/api.log" >&2
fi
echo "    API:    $api_url  (try: curl -i $api_url/healthz)"
echo "    Meili:  http://localhost:7700"
echo "    ntfy:   http://localhost:8081"
echo "    Web:    cd web && pnpm dev   (http://localhost:5173)"
echo
echo "Stop with: bash scripts/dev-stop.sh"
