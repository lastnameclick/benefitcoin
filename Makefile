.PHONY: tb pg up down logs api web build test test-integration tidy reset fmt

# Container engine for Postgres. Override with `make COMPOSE="docker compose"`.
COMPOSE ?= podman compose

# On macOS, TigerBeetle's prebuilt static archive isn't 8-byte aligned, which
# the modern linker rejects; fall back to the classic linker there.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	export CGO_LDFLAGS = -Wl,-ld_classic
endif

# --- infra ---------------------------------------------------------------

# Run TigerBeetle natively (downloads + formats on first run). Foreground.
tb:
	./scripts/tigerbeetle.sh start

# Start Postgres (detached).
pg up:
	$(COMPOSE) up -d
	@echo "postgres starting..."

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

# Wipe Postgres data AND the TigerBeetle data file (fresh ledger + db).
reset:
	$(COMPOSE) down -v
	rm -f .tigerbeetle/0_0.tigerbeetle

# --- app -----------------------------------------------------------------

# Run the Go API server (auto-migrates Postgres on boot). Needs `make pg` + `make tb`.
api:
	go run ./cmd/api

build:
	go build -o bin/cpal-api ./cmd/api

web:
	cd web && npm run dev

# --- tests ---------------------------------------------------------------

# Unit tests, no infra required.
test:
	go test ./internal/money/... ./internal/auth/...

# Full end-to-end test (needs `make pg` + `make tb`).
test-integration:
	CPAL_INTEGRATION=1 go test ./internal/api/ -run TestEndToEnd -v

tidy:
	go mod tidy

fmt:
	go fmt ./...
