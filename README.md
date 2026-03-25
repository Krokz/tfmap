# tfmap

A local-first Terraform visualization tool. Point it at any Terraform directory and get an interactive dependency graph, diagnostics, state drift detection, and multi-module navigation — all in the browser.

![hero](web/src/assets/hero.png)

## Features

- **Dependency graph** — interactive React Flow graph of resources, data sources, and modules with auto-layout. Expand any module node inline to reveal its child resources, data sources, and sub-modules without leaving the current view
- **Diagnostics engine** — detects dependency cycles, unused variables, missing tags, missing descriptions, provider version pins, sensitive naming, high blast-radius resources, and orphaned state entries
- **State drift detection** — reads remote state (S3 backends) and compares with HCL, flagging resources as in-sync, drifted, not-in-state, or orphaned. Modules show an aggregate indicator: green (all children in sync), light yellow (partially drifted), or orange (all children drifted)
- **Multi-module navigation** — discover and jump into nested modules; see which internal resources participate in cross-module cycles
- **Live reload** — watches the filesystem for `.tf` changes and pushes updates over WebSocket

## Quick start

### Prerequisites

| Tool | Version |
|------|---------|
| Go | 1.25+ |
| Node.js | 20+ (see `web/.nvmrc`) |
| npm | 10+ |
| AWS CLI | optional, for S3 state reading |

### Build and run

```bash
# Build everything (frontend + backend) for your current OS/arch
make build

# Run against a Terraform directory
./tfmap /path/to/terraform/project

# Or just run tfmap — an interactive directory picker will appear
./tfmap

# Or run from source
make run DIR=/path/to/terraform/project
```

The frontend is embedded into the binary at compile time via `go:embed`, so the resulting `tfmap` binary is fully self-contained — copy it anywhere and it works.

When run without a path argument, tfmap opens a native OS folder picker dialog so you can browse to your Terraform project directory.

The UI opens automatically at `http://127.0.0.1:<port>` (a random available port is chosen by default).

### Cross-compilation

Build for all supported platforms at once:

```bash
make build-all
```

This produces binaries in `dist/`:

| File | Platform |
|------|----------|
| `dist/tfmap-darwin-arm64` | macOS Apple Silicon |
| `dist/tfmap-darwin-amd64` | macOS Intel |
| `dist/tfmap-linux-amd64` | Linux 64-bit |
| `dist/tfmap-linux-386` | Linux 32-bit |
| `dist/tfmap-windows-amd64.exe` | Windows 64-bit |

Or build for a single target:

```bash
make build-linux-amd64
make build-darwin-arm64
make build-windows-amd64
```

### CLI flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | `0` (random) | Port to serve the UI on |
| `--no-browser` | | `false` | Don't open the browser automatically |
| `--no-state` | | `false` | Skip state reading entirely |
| `--aws-profile` | | | AWS profile for S3 state reading |

### Examples

```bash
# Use a specific port
tfmap -p 3000 ./infra

# Skip state, just visualize the HCL
tfmap --no-state ./modules/networking

# Use a named AWS profile for S3 state
tfmap --aws-profile production ./envs/prod
```

## Development

### Project structure

```
tfmap/
├── cmd/                          # CLI entrypoint (Cobra)
├── internal/
│   ├── diagnostics/              # Diagnostic rules and cycle detection
│   ├── model/                    # Shared data model (Project, Resource, etc.)
│   ├── parser/                   # HCL parser (hclsyntax AST)
│   ├── server/                   # HTTP + WebSocket server
│   ├── state/                    # Terraform state reader (S3, local)
│   └── watcher/                  # Filesystem watcher (fsnotify)
├── web/                          # React SPA (Vite + TypeScript + Tailwind)
│   └── src/
│       ├── components/           # TreeExplorer, GraphView, DetailPanel, etc.
│       ├── hooks/                # useProject (WebSocket + fetch)
│       ├── utils/                # Shared utilities (module resolution, etc.)
│       └── types.ts              # TypeScript types mirroring Go model
├── main.go                       # Entrypoint, passes embedded FS to CLI
├── embed.go                      # go:embed directive for web/dist
├── Makefile
└── README.md
```

### Full-stack dev

Run the Go backend and Vite dev server separately for hot-reload:

```bash
# Terminal 1: backend on port 8080
make dev-backend

# Terminal 2: frontend with proxy to :8080
make dev-frontend
```

The Vite dev server proxies `/api` and `/ws` to `127.0.0.1:8080`.

### Testing

```bash
# Run all Go tests
make test

# Run tests with verbose output
make test-verbose

# Run only diagnostics tests
go test ./internal/diagnostics/ -v
```

> **Note:** Use `make test` rather than `go test ./...` directly — the Makefile ensures `web/dist` exists, which is required by the `go:embed` directive in the root package.

### Linting

```bash
# Run all linters
make lint

# Or individually
go vet ./...
cd web && npm run lint
```

## Architecture

```
┌─────────────┐     ┌──────────┐     ┌─────────────┐
│  .tf files  │────▶│  Parser  │────▶│   Project   │
└─────────────┘     └──────────┘     │   (model)   │
                                     └──────┬──────┘
┌─────────────┐                             │
│  State (S3) │──────────────────────▶ CompareWithState
└─────────────┘                             │
                                     ┌──────▼──────┐
                                     │ Diagnostics │
                                     └──────┬──────┘
                                            │
               ┌────────────────────────────▼────────────────┐
               │            HTTP + WebSocket Server          │
               │  GET /api/project    WS /ws (live reload)   │
               └────────────────────────────┬────────────────┘
                                            │
                              ┌──────────────▼──────────────┐
                              │      React SPA (browser)    │
                              │  Graph ─ Tree ─ Diagnostics │
                              └─────────────────────────────┘
```

**Data flow:** Parser reads `.tf` files → builds a `Project` model → state reader enriches with drift info → diagnostics engine analyzes → server serves over HTTP/WS → React app renders the graph, tree, and diagnostic panels. The filesystem watcher re-triggers the pipeline on changes.

The React SPA is embedded into the Go binary at build time (`go:embed`), so the server serves it directly from memory. During development, the server falls back to the `web/dist` directory on disk, allowing the Vite dev server to handle frontend hot-reload via proxy.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
