# URP Go - Development TODO

## Completado (2025-12-03)

### Vector Integration
- [x] `internal/vector/lancedb.go` - Store con JSON+binary persistence
- [x] `internal/vector/embedder.go` - LocalEmbedder (384 dims, hash-based)
- [x] `internal/cognitive/wisdom.go` - Vector search + Jaccard fallback
- [x] `internal/cognitive/novelty.go` - Vector search + Jaccard fallback
- [x] `cmd/urp/main.go` - `urp vec stats/search/add`

### Container Orchestration
- [x] `internal/container/manager.go` - Docker/Podman management
- [x] `cmd/urp/main.go` - Infrastructure commands:
  - `urp infra status|start|stop|clean|logs`
  - `urp launch [path] [--master|--readonly]`
  - `urp spawn [n]` (from master)
  - `urp workers`
  - `urp attach <container>`
  - `urp kill <container> [--all]`
- [x] `Dockerfile` - Multi-stage build for URP image
- [x] `shell_hooks.sh` - Terminal flow capture hooks
- [x] `entrypoint.sh` - Container initialization

## Próxima Sesión

### 1. Auto-indexing de errores (executor.go)
**Archivo:** `internal/runner/executor.go`

Cuando un comando falla (exit != 0), indexar el error automáticamente:
```go
if result.ExitCode != 0 && result.Stderr != "" {
    wisdom := cognitive.NewWisdomService(db)
    wisdom.IndexError(ctx, result.Stderr, command, project)
}
```

### 2. Ingester → Vector integration
**Archivo:** `internal/ingest/ingester.go`

Cuando se parsea código, indexar funciones en vectores:
```go
novelty := cognitive.NewNoveltyService(db)
for _, fn := range functions {
    novelty.IndexCode(ctx, fn.Body, fn.Signature, fn.Path)
}
```

### 3. External embeddings (opcional)
**Nuevo archivo:** `internal/vector/openai.go`

Soporte para embeddings de OpenAI/Anthropic:
```go
type OpenAIEmbedder struct {
    apiKey string
    model  string  // text-embedding-3-small
    dims   int     // 1536
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error)
```

Configuración via env:
```
URP_EMBEDDER=openai|local
OPENAI_API_KEY=sk-...
```

### 4. Tests para novelty vector integration
**Archivo:** `internal/cognitive/novelty_test.go`

```go
func TestNoveltyVectorSearch(t *testing.T)
func TestNoveltyFallbackToJaccard(t *testing.T)
func TestIndexCode(t *testing.T)
```

## Orden recomendado
1. Auto-indexing (más impacto inmediato)
2. Tests para novelty
3. Ingester integration
4. External embeddings (nice-to-have)

## Commits pendientes
- `fc2b5c4..7261b0e` ya pusheados a origin/master
