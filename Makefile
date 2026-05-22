BINARY     := galactic-media-server
CMD        := ./cmd/server
WEB_DIR    := web
DIST_DIR   := $(WEB_DIR)/dist/browser

.PHONY: all build run test lint clean reset web web-dev help

all: build ## Build everything (backend + frontend)

# ── Backend ──────────────────────────────────────────────────────────────────

build: web ## Build backend binary + frontend
	go build -o $(BINARY) $(CMD)

build-server: ## Build backend binary only (no frontend)
	go build -o $(BINARY) $(CMD)

run: build ## Build and run the server
	./$(BINARY)

test: ## Run all Go tests
	go test ./...

lint: ## Run go vet
	go vet ./...

# ── Frontend ─────────────────────────────────────────────────────────────────

web: ## Build Angular frontend to web/dist/
	cd $(WEB_DIR) && npm run build

web-dev: ## Start Angular dev server with proxy to localhost:8080
	cd $(WEB_DIR) && npm start

web-install: ## Install npm dependencies
	cd $(WEB_DIR) && npm install

# ── Housekeeping ──────────────────────────────────────────────────────────────

clean: ## Remove build artefacts
	rm -f $(BINARY)
	rm -rf $(WEB_DIR)/dist
	rm -f /tmp/smoke2.db
	rm -rf /data/*

reset: ## Remove runtime data (/data contents and /tmp/smoke2.db)
	rm -f /tmp/smoke2.db
	rm -rf /data/*

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  %-18s %s\n", $$1, $$2}'
