.PHONY: build build-frontend build-backend build-all run \
       dev-backend dev-frontend test test-verbose lint clean \
       build-darwin-arm64 build-darwin-amd64 \
       build-linux-amd64 build-linux-386 \
       build-windows-amd64 ensure-dist

BINARY  := tfmap
DIR     ?= .
PORT    ?= 8080
DIST    := dist

# ── Ensure web/dist exists (needed for go:embed) ─────

ensure-dist:
	@mkdir -p web/dist
	@test -f web/dist/index.html || printf '<!doctype html><html><body>Run make build to build the frontend</body></html>' > web/dist/index.html

# ── Build (current platform) ──────────────────────────

build: build-frontend build-backend

build-frontend:
	cd web && npm ci && npm run build

build-backend: ensure-dist
	go build -o $(BINARY) .

# ── Cross-compilation ─────────────────────────────────

build-all: build-frontend \
           build-darwin-arm64 build-darwin-amd64 \
           build-linux-amd64 build-linux-386 \
           build-windows-amd64
	@echo "All binaries in $(DIST)/"

build-darwin-arm64: ensure-dist
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 go build -o $(DIST)/$(BINARY)-darwin-arm64 .

build-darwin-amd64: ensure-dist
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 go build -o $(DIST)/$(BINARY)-darwin-amd64 .

build-linux-amd64: ensure-dist
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -o $(DIST)/$(BINARY)-linux-amd64 .

build-linux-386: ensure-dist
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=386 go build -o $(DIST)/$(BINARY)-linux-386 .

build-windows-amd64: ensure-dist
	@mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/$(BINARY)-windows-amd64.exe .

# ── Run ────────────────────────────────────────────────

run: build
	./$(BINARY) $(DIR)

# ── Development ────────────────────────────────────────

dev-backend:
	go run . -p $(PORT) --no-browser $(DIR)

dev-frontend:
	cd web && npm run dev

# ── Test ───────────────────────────────────────────────

test: ensure-dist
	go test ./...

test-verbose: ensure-dist
	go test -v ./...

# ── Lint ───────────────────────────────────────────────

lint: ensure-dist
	go vet ./...
	cd web && npm run lint

# ── Clean ──────────────────────────────────────────────

clean:
	rm -f $(BINARY)
	rm -rf $(DIST)
	rm -rf web/dist
