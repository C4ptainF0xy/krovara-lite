.PHONY: build test lint dev-up dev-down dev-prosody-certs prosody-adduser prosody-logs prosody-smoke migrate-up migrate-down tidy

GO          ?= go
PG_DSN      ?= postgres://krovara:krovara@localhost:5432/krovara?sslmode=disable
MIGRATE     ?= migrate -path migrations -database "$(PG_DSN)"
COMPOSE_DEV ?= docker compose -f docker-compose.dev.yml

build:
	$(GO) build ./...

test:
	$(GO) test ./...

lint:
	golangci-lint run

tidy:
	$(GO) mod tidy

dev-up: dev-prosody-certs
	$(COMPOSE_DEV) up -d

dev-down:
	$(COMPOSE_DEV) down

dev-prosody-certs:
	bash scripts/dev-prosody-certs.sh

# Create a Prosody user. Usage: make prosody-adduser USER=alice PASSWORD=secret
prosody-adduser:
	$(COMPOSE_DEV) exec prosody prosodyctl register $(USER) krovara.local $(PASSWORD)

prosody-logs:
	$(COMPOSE_DEV) logs -f prosody

# Smoke test the running stack. Requires `make dev-up` first and an existing
# alice/secret user (see prosody-adduser).
prosody-smoke:
	KROVARA_PROSODY_SMOKE=1 $(GO) test -count=1 -run TestProsodyWebSocketSmoke ./internal/xmpp/...

migrate-up:
	$(MIGRATE) up

migrate-down:
	$(MIGRATE) down 1
