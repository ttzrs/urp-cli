# URP Desktop

Desktop GUI for URP using Tauri + SolidJS.

## Architecture

```
┌─────────────────────────────────────────────┐
│              TAURI (Rust)                   │
│  - Window management                        │
│  - Spawns urp serve as sidecar              │
│  - Native dialogs                           │
└─────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│            URP SERVER (Go)                  │
│  urp serve --port 7878                      │
│  - REST API: /api/status, /sessions, etc    │
│  - Graph connectivity                       │
│  - Agent execution                          │
└─────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────┐
│           UI (SolidJS + Vite)               │
│  - Status bars (FOCUS, CTX, Graph)          │
│  - Session management                       │
│  - Chat interface                           │
└─────────────────────────────────────────────┘
```

## Development

```bash
# Install dependencies
bun install

# Build sidecar + run Tauri dev (all-in-one)
bun run tauri:dev

# Or manually:
# 1. Build urp sidecar
bun run build:sidecar

# 2. Start frontend dev server
bun run dev

# 3. In another terminal, run Tauri
bun run tauri dev
```

## Building (Single Executable)

```bash
# Build everything (sidecar + frontend + Tauri bundle)
bun run tauri:build
```

This produces a single executable that includes:
- The Tauri shell (Rust)
- The SolidJS frontend (bundled)
- The urp binary (Go, as sidecar)

Output locations:
- Linux: `src-tauri/target/release/bundle/appimage/`
- macOS: `src-tauri/target/release/bundle/dmg/`
- Windows: `src-tauri/target/release/bundle/msi/`

## Cross-compilation

```bash
# Build sidecar for specific target
bash scripts/build-sidecar.sh x86_64-unknown-linux-gnu
bash scripts/build-sidecar.sh aarch64-apple-darwin
bash scripts/build-sidecar.sh x86_64-pc-windows-msvc
```

## Status Bar Components

The status bar shows:
- **Graph**: Connection status to Memgraph
- **PRJ**: Current project name
- **FOCUS**: Current focus target and depth
- **CTX**: Token usage and loaded files
- **Events**: Number of terminal events
- **Workers**: Active worker containers

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/status` | GET | URP status |
| `/api/sessions` | GET | List sessions |
| `/api/prompt` | POST | Send prompt |
| `/api/focus` | GET/POST | Get/set focus |
