# Agentic Development Guide

## Commands
- **Build**: `make build` (builds `urp` binary)
- **Test All**: `make test` (runs all Go tests)
- **Test Single**: `cd go && go test -v ./internal/<package> -run <TestName>`
- **Lint**: `make lint` (runs `golangci-lint`)
- **Format**: `make fmt` (runs `go fmt` and `goimports`)

## Code Style & Conventions
- **Language**: Go 1.24+
- **Imports**: Grouped as: Standard Lib (`fmt`, `os`) -> 3rd Party (`github.com/...`) -> Internal (`github.com/joss/urp/...`)
- **Naming**: PascalCase for exported symbols, camelCase for internal. Clear, descriptive names.
- **Structure**: Entry points in `cmd/urp/`, logic in `internal/`. New logic goes to `internal/`.
- **Error Handling**: Explicit check (`if err != nil`). Return errors in `internal/`, use `fatalError` only in `main`.
- **CLI**: Use `cobra` framework. Define commands in `cmd/urp/`.
- **Testing**: Table-driven tests preferred. Use `assert` from `testify` if available.
- **Agent Rules**: 
  - Read `CLAUDE.md` for high-level axioms.
  - Master/Worker architecture must be respected.
  - Do not introduce cycles in imports.
