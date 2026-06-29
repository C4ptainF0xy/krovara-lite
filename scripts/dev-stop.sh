#!/usr/bin/env bash
# Re-exec under Git Bash when launched via WSL (see scripts/dev-run.sh): the PIDs
# in .dev-logs were written by Git Bash, so they must be killed from there.
if grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    for gb in "/mnt/c/Program Files/Git/bin/bash.exe" "/mnt/c/Program Files/Git/usr/bin/bash.exe"; do
        [ -x "$gb" ] && exec "$gb" "$0" "$@"
    done
    echo "ERROR: lancé sous WSL et Git Bash introuvable." >&2
    exit 1
fi

set -u

for svc in api worker; do
    pidfile=".dev-logs/$svc.pid"
    if [[ -f "$pidfile" ]]; then
        pid=$(cat "$pidfile")
        if kill "$pid" 2>/dev/null; then
            echo "stopped $svc (pid $pid)"
        fi
        rm -f "$pidfile"
    fi
done

docker compose -f docker-compose.dev.yml down
