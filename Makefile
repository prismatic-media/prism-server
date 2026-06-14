BINARY     := prism
CMD        := ./cmd/server
WEB_DIR    := web
DIST_DIR   := $(WEB_DIR)/dist/browser

.PHONY: all build run watch-server test lint clean reset web web-dev help

all: build ## Build everything (backend + frontend)

deploy: 
	git pull
	cd $(WEB_DIR) && npm run build && cd ..
	go build -o $(BINARY) $(CMD)
	systemctl restart prism
	journalctl -f -u prism

# ── Backend ──────────────────────────────────────────────────────────────────

build: web ## Build backend binary + frontend
	go build -o $(BINARY) $(CMD)

build-server: ## Build backend binary only (no frontend)
	go build -o $(BINARY) $(CMD)

worker: ## Build remote transcode worker binary
	go build -o prism-worker ./cmd/worker

run: build ## Build and run the server
	./$(BINARY)

watch-server: ## Start Go server with a file watcher for automatic restarting
	bash scripts/watch-go.sh

swagger: ## Generate Swagger API specification from code annotations
	go run github.com/swaggo/swag/cmd/swag@v1.16.3 init -g cmd/server/main.go -o internal/api/handler/docs

test: ## Run all Go tests
	go test ./...

lint: ## Run go vet
	go vet ./...

dev: web-build-dev
	go run $(CMD)

# ── Frontend ─────────────────────────────────────────────────────────────────

web: ## Build Angular frontend to web/dist/
	cd $(WEB_DIR) && npm run build

web-dev: ## Start Angular dev server with proxy to localhost:8080
	cd $(WEB_DIR) && npm start

web-install: ## Install npm dependencies
	cd $(WEB_DIR) && npm install

web-build-dev:
	cd $(WEB_DIR) && ng build --configuration development

# ── Housekeeping ──────────────────────────────────────────────────────────────

clean: ## Remove build artefacts
	rm -f $(BINARY)
	rm -f server
	rm -f prism-worker
	rm -rf $(WEB_DIR)/dist
	rm -f /tmp/smoke2.db
	rm -rf /data/*

reset: ## Remove runtime data (/data contents and /tmp/smoke2.db)
	rm -f /tmp/smoke2.db
	rm -rf /data/*

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  %-18s %s\n", $$1, $$2}'
