# Contributing to tfmap

Thanks for your interest in contributing! This document covers the basics.

## Getting started

1. Fork and clone the repo
2. Install prerequisites: Go 1.25+, Node.js 20+, npm 10+
3. Run `make build` to verify everything compiles
4. Run `make test` to verify tests pass

## Development workflow

### Full-stack dev (recommended)

```bash
# Terminal 1 — Go backend with hot-reload on save
make dev-backend DIR=/path/to/terraform/project

# Terminal 2 — Vite dev server with HMR
make dev-frontend
```

Open `http://localhost:5173` (Vite proxies API/WS calls to the backend on `:8080`).

### Running tests

```bash
make test            # all Go tests
make test-verbose    # verbose output
make lint            # go vet + eslint
```

## Project layout

| Directory | Language | Purpose |
|-----------|----------|---------|
| `cmd/` | Go | CLI entrypoint (Cobra) |
| `internal/parser/` | Go | HCL parsing via `hclsyntax` |
| `internal/diagnostics/` | Go | Diagnostic rules, cycle detection |
| `internal/model/` | Go | Shared data model |
| `internal/server/` | Go | HTTP + WebSocket server |
| `internal/state/` | Go | State reading (S3, local) |
| `internal/watcher/` | Go | Filesystem watcher |
| `web/src/` | TypeScript/React | Browser UI |

## Conventions

### Go

- Standard library style. `go vet` must pass.
- All types live in `internal/model/model.go`. JSON tags must match the TypeScript types in `web/src/types.ts`.
- New diagnostic rules go in `internal/diagnostics/diagnostics.go` — add a `check*` function, wire it into `Analyze()`, and add a test.
- Tests use the `testing` package directly; no third-party test frameworks.

### Frontend

- TypeScript strict mode, Tailwind CSS for styling, no CSS modules.
- Types in `web/src/types.ts` mirror the Go model — keep them in sync.
- State lives in `App.tsx`; child components receive props.

## Pull requests

1. Create a feature branch from `main`
2. Keep commits focused — one logical change per commit
3. Add or update tests for any new behavior
4. Make sure `make test` and `make lint` pass
5. Write a clear PR description explaining *why*, not just *what*

## Adding a diagnostic rule

1. Add a `check*` function in `internal/diagnostics/diagnostics.go`
2. Wire it into the `Analyze()` function
3. Add a test in `diagnostics_test.go`
4. If the rule should apply regardless of whether the entity is referenced, add it to `referencedFilterExempt`

## Bug reports

Open an issue with:
- What you expected to happen
- What actually happened
- The Terraform directory structure (sanitized) if relevant
- tfmap version or commit hash

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
