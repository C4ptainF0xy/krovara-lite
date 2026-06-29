#!/usr/bin/env bash
# Generate self-signed certs for the dev Prosody instance.
#
# Idempotent: skips generation if certs already exist. Run from repo root.

set -euo pipefail

CERT_DIR="prosody/certs"
HOST="krovara.local"

mkdir -p "$CERT_DIR"

if [[ -f "$CERT_DIR/$HOST.crt" && -f "$CERT_DIR/$HOST.key" ]]; then
    echo "certs already exist at $CERT_DIR/$HOST.{crt,key} — skipping"
    exit 0
fi

# The leading `//` is a Git-Bash MSYS workaround: a single `/CN=…` would
# otherwise be rewritten to a Windows path by the shell. Real Unix shells
# treat `//CN=…` the same as `/CN=…` for OpenSSL's parser.
openssl req -x509 -newkey rsa:2048 -nodes \
    -days 365 \
    -keyout "$CERT_DIR/$HOST.key" \
    -out    "$CERT_DIR/$HOST.crt" \
    -subj   "//CN=$HOST" \
    -addext "subjectAltName=DNS:$HOST,DNS:conference.$HOST,DNS:pubsub.$HOST,DNS:upload.$HOST,DNS:localhost,IP:127.0.0.1"

chmod 600 "$CERT_DIR/$HOST.key"
echo "generated $CERT_DIR/$HOST.crt"
