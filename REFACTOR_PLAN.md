# URP CLI - Plan de Refactorización SOLID

```
VERDICT: refactorizar porque hay duplicación masiva y god objects
DATA: 67 duplicados exactos, 5 god objects, 12 incoherencias arquitectónicas
REMOVE: boilerplate commands, type switches, concrete dependencies
RISK: Agent y Healer son el núcleo - tocarlos con cuidado
ACTION: simplify(data) → kill(edge_cases) → implement(obvious)
```

---

## AUDITORÍA ACTUALIZADA (2025-12-05)

### CÓDIGO DUPLICADO EXACTO DETECTADO

| Función | Ubicaciones | LOC Duplicadas |
|---------|-------------|----------------|
| `getString(r graph.Record, key string)` | 10 archivos | ~80 LOC |
| `getInt(r graph.Record, key string)` | 8 archivos | ~80 LOC |
| `getInt64(r graph.Record, key string)` | 5 archivos | ~50 LOC |
| `getFloat(r graph.Record, key string)` | 4 archivos | ~40 LOC |
| `getBool(r graph.Record, key string)` | 2 archivos | ~20 LOC |
| `truncate(s string, n int)` | 7 archivos | ~35 LOC |
| **TOTAL** | **36 instancias** | **~305 LOC** |

### Ubicaciones `getString`:
```
internal/memory/session.go:341
internal/cognitive/wisdom.go:286
internal/bridge/opencode.go:426
internal/query/query.go:323
internal/planning/plan.go:453
internal/skills/store.go:271
internal/runner/queries.go:127
internal/audit/store.go:302
internal/orchestrator/persistence.go:292
internal/opencode/graphstore/store.go:358
```

### Ubicaciones `truncate`:
```
internal/runner/executor.go:189
internal/tui/tui.go:475
internal/ingest/git.go:241
internal/memory/session.go:334
internal/skills/store.go:305
internal/opencode/permission/permission.go:295
internal/opencode/cognitive/signals.go:302
cmd/urp/helpers.go:31 (truncateStr - diferente nombre)
```

---

## RESUMEN EJECUTIVO

| Métrica | Valor | Impacto |
|---------|-------|---------|
| Duplicados exactos | 67 | Mantenibilidad CRÍTICA |
| God Objects | 5 | SRP violado |
| Violaciones DIP | 6 | Testabilidad BAJA |
| Violaciones OCP | 4 | Extensibilidad BAJA |
| Incoherencias | 12 | Cognición del código |
| Archivos >300 líneas | 44 | Complejidad |

---

## FASE 1: QUICK WINS (Eliminar Duplicación)

**Impacto: ALTO | Esfuerzo: BAJO | Riesgo: BAJO**

### 1.1 Crear `cmd/urp/helpers.go`

Elimina **67 duplicados** del patrón db-check + **3 truncate** + **5 path helpers**:

```go
// go/cmd/urp/helpers.go
package main

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/joss/urp/internal/audit"
)

// checkDB - elimina 67 duplicados
func checkDB(event *audit.Event) {
    if db == nil {
        auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
        fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
        os.Exit(1)
    }
}

// handleError - elimina 30+ patrones similares
func handleError(event *audit.Event, err error) {
    auditLogger.LogError(event, err)
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}

// truncate - elimina 3 funciones duplicadas
func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n-3] + "..."
}

// urpPath - elimina 5-6 construcciones path duplicadas
func urpPath(subdir ...string) string {
    home, _ := os.UserHomeDir()
    parts := append([]string{home, ".urp-go"}, subdir...)
    return filepath.Join(parts...)
}
```

**Archivos a modificar:**
- `cmd/urp/main.go` - 45 instancias
- `cmd/urp/skills.go` - 8 instancias
- `cmd/urp/opencode.go` - 11 instancias
- `cmd/urp/backup.go` - 3 instancias

### 1.2 Crear `cmd/urp/factory.go`

Factory para comandos con boilerplate común:

```go
// go/cmd/urp/factory.go
package main

import (
    "github.com/joss/urp/internal/audit"
    "github.com/spf13/cobra"
)

type CommandFunc func(cmd *cobra.Command, args []string) error

func newCommand(use, short string, category audit.Category, action string, fn CommandFunc) *cobra.Command {
    return &cobra.Command{
        Use:   use,
        Short: short,
        Run: func(cmd *cobra.Command, args []string) {
            event := auditLogger.Start(category, action)
            checkDB(event)

            if err := fn(cmd, args); err != nil {
                handleError(event, err)
            }

            auditLogger.LogSuccess(event)
        },
    }
}
```

---

## FASE 2: DESCOMPONER GOD OBJECTS

**Impacto: ALTO | Esfuerzo: MEDIO | Riesgo: MEDIO**

### 2.1 Split Agent (639 líneas → 5 componentes)

**Archivo:** `internal/opencode/agent/agent.go`

```
┌─────────────────────────────────────────┐
│ Agent (orquestador, ~150 líneas)        │
│ - Run()                                 │
│ - processEvents() [delega]              │
└─────────┬─────────────────────────────────┘
          │
    ┌─────┼─────────┬─────────────┬────────────────┐
    ▼     ▼         ▼             ▼                ▼
Message   Tool      Autocorrect   Cognitive        System
Store     Executor  Engine        Adapter          Prompt
(nuevo)   (existe)  (nuevo)       (nuevo)          Builder
```

**Nuevos archivos:**
```
internal/opencode/agent/
├── agent.go          (orquestador ~150 LOC)
├── message_store.go  (persistencia mensajes ~80 LOC)
├── autocorrect.go    (lógica retry ~100 LOC)
├── prompt.go         (system prompt builder ~60 LOC)
└── cognitive.go      (bridge a cognitive engine ~50 LOC)
```

### 2.2 Split Healer (555 líneas → 4 componentes)

**Archivo:** `internal/audit/healing.go`

```
┌─────────────────────────────────────┐
│ Healer (coordinador, ~100 líneas)   │
└─────────┬───────────────────────────┘
          │
    ┌─────┼─────────┬─────────────┬────────────┐
    ▼     ▼         ▼             ▼            ▼
Anomaly   Remed.    Cooldown      Healing      Retry
Classifier Engine   Manager       Repository   Tracker
```

**Nuevos archivos:**
```
internal/audit/
├── healing.go           (coordinador ~100 LOC)
├── anomaly_rules.go     (clasificación ~120 LOC)
├── remediation.go       (acciones ~150 LOC)
├── healing_store.go     (persistencia ~180 LOC)
└── cooldown.go          (rate limiting ~60 LOC)
```

### 2.3 Split Ingester (428 líneas → 4 componentes)

**Archivo:** `internal/ingest/ingester.go`

```
internal/ingest/
├── ingester.go       (orquestador ~100 LOC)
├── discovery.go      (file walking + gitignore ~80 LOC)
├── persistence.go    (batch graph writes ~150 LOC)
├── indexer.go        (async vector indexing ~80 LOC)
└── linker.go         (call graph linking ~50 LOC)
```

---

## FASE 3: INVERSIÓN DE DEPENDENCIAS (DIP)

**Impacto: MEDIO | Esfuerzo: BAJO | Riesgo: BAJO**

### 3.1 Inyectar HTTPClient en OpenAIEmbedder

**Archivo:** `internal/vector/openai_embedder.go:44`

```go
// ANTES
client: &http.Client{}

// DESPUÉS
type OpenAIEmbedder struct {
    client HTTPClient  // interface ya definida en provider
    // ...
}

func NewOpenAIEmbedder(apiKey string, opts ...Option) *OpenAIEmbedder {
    e := &OpenAIEmbedder{client: http.DefaultClient}
    for _, opt := range opts {
        opt(e)
    }
    return e
}

func WithHTTPClient(c HTTPClient) Option {
    return func(e *OpenAIEmbedder) { e.client = c }
}
```

### 3.2 Inyectar ImmuneSystem en Executor

**Archivo:** `internal/runner/executor.go:32`

```go
// ANTES
immune: NewImmuneSystem()

// DESPUÉS
type Immune interface {
    Check(cmd string) (blocked bool, reason string)
}

func NewExecutor(db graph.Driver, immune Immune) *Executor {
    if immune == nil {
        immune = NewImmuneSystem()
    }
    return &Executor{db: db, immune: immune}
}
```

### 3.3 Usar interface en specs/Engine

**Archivo:** `internal/specs/engine.go:28`

```go
// ANTES
func (e *Engine) WithDB(db *graph.Memgraph) *Engine

// DESPUÉS
func (e *Engine) WithDB(db graph.Driver) *Engine
```

### 3.4 Extraer FileSystem interface

**Nuevo archivo:** `internal/common/filesystem.go`

```go
package common

import (
    "io/fs"
    "os"
    "path/filepath"
)

type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm fs.FileMode) error
    Walk(root string, fn filepath.WalkFunc) error
    MkdirAll(path string, perm fs.FileMode) error
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(path string) ([]byte, error) {
    return os.ReadFile(path)
}
// ... implementar resto
```

**Usar en:** `ingest/ingester.go`, `specs/engine.go`, `vector/store.go`

### 3.5 Extraer CommandRunner interface

**Nuevo archivo:** `internal/common/process.go`

```go
package common

import (
    "context"
)

type CommandRunner interface {
    Run(ctx context.Context, cmd string, args ...string) (stdout, stderr string, exitCode int, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, cmd string, args ...string) (string, string, int, error) {
    // wrapper sobre exec.CommandContext
}
```

**Usar en:** `runner/executor.go`, `ingest/git.go`, `container/manager.go`

---

## FASE 4: CERRAR PARA EXTENSIÓN (OCP)

**Impacto: MEDIO | Esfuerzo: MEDIO | Riesgo: BAJO**

### 4.1 Registry de Entity Types

**Archivo:** `internal/ingest/ingester.go:143-338` (elimina 3 switches)

```go
// internal/domain/entity_type.go
package domain

type EntityTypeMeta struct {
    Label    string
    StatKey  string
}

var entityMeta = map[EntityType]EntityTypeMeta{
    EntityFile:      {"File", "files"},
    EntityFunction:  {"Function", "functions"},
    EntityMethod:    {"Method", "functions"},
    EntityStruct:    {"Struct", "structs"},
    EntityInterface: {"Interface", "interfaces"},
    EntityClass:     {"Class", "classes"},
}

func (e EntityType) GraphLabel() string {
    if m, ok := entityMeta[e]; ok {
        return m.Label
    }
    return "Entity"
}

func (e EntityType) StatKey() string {
    if m, ok := entityMeta[e]; ok {
        return m.StatKey
    }
    return "other"
}
```

### 4.2 TokenCount en Part interface

**Archivo:** `internal/opencode/domain/message.go`

```go
type Part interface {
    PartType() string
    TokenCount(counter TokenCounter) int
}

// Implementar en cada Part type
func (t TextPart) TokenCount(c TokenCounter) int {
    return c.Count(t.Text)
}

func (t ToolCallPart) TokenCount(c TokenCounter) int {
    return c.Count(t.Name) + 50 + c.Count(t.Result)
}
```

**Elimina switches en:** `usage.go:56`, `anthropic.go:166`, `openai.go:169`

---

## FASE 5: COHERENCIA ARQUITECTÓNICA

**Impacto: MEDIO | Esfuerzo: MEDIO | Riesgo: MEDIO**

### 5.1 Estandarizar Error Handling

**Crear:** `internal/common/errors.go`

```go
package common

import "encoding/json"

// SafeUnmarshal - nunca silencia errores
func SafeUnmarshal(data []byte, v interface{}) error {
    if err := json.Unmarshal(data, v); err != nil {
        return fmt.Errorf("json unmarshal: %w", err)
    }
    return nil
}
```

**Arreglar en:**
- `opencode/config/config.go:46` - usar SafeUnmarshal
- `vector/memgraph_store.go:98` - usar SafeUnmarshal
- `bridge/opencode.go:98` - propagar error

### 5.2 Unificar Config Pattern

**Elegir:** Environment vía función inyectable (como graph/driver.go)

```go
// internal/common/env.go
package common

import "os"

var EnvLookup = os.Getenv

func SetEnvLookup(fn func(string) string) {
    EnvLookup = fn
}

// Usar en todos los constructores
apiKey := common.EnvLookup("ANTHROPIC_API_KEY")
```

### 5.3 Renombrar Event Types (evitar colisiones)

```go
// internal/domain/event.go
type TerminalEvent struct { ... }  // antes: Event

// internal/opencode/domain/event.go
type StreamEvent struct { ... }    // antes: Event

// internal/audit/event.go
type AuditEvent struct { ... }     // ya tiene nombre correcto
```

### 5.4 Unificar Builder vs Setter

**En Agent:** convertir todo a With* (fluent):

```go
func (a *Agent) WithWorkDir(dir string) *Agent {
    a.workDir = dir
    return a
}

func (a *Agent) WithThinkingBudget(b int) *Agent {
    a.thinkingBudget = b
    return a
}

func (a *Agent) WithAutocorrection(cfg AutocorrectionConfig) *Agent {
    a.autocorrection = cfg
    return a
}
```

---

## FASE 6: REDUCIR main.go (4,278 → ~500 líneas)

**Impacto: ALTO | Esfuerzo: ALTO | Riesgo: MEDIO**

### 6.1 Extraer comandos a archivos separados

```
cmd/urp/
├── main.go           (~200 LOC - setup + root command)
├── helpers.go        (~80 LOC - FASE 1)
├── factory.go        (~50 LOC - FASE 1)
├── code_cmds.go      (~300 LOC - urp code *)
├── git_cmds.go       (~150 LOC - urp git *)
├── think_cmds.go     (~200 LOC - urp think *)
├── mem_cmds.go       (~150 LOC - urp mem *)
├── kb_cmds.go        (~200 LOC - urp kb *)
├── sys_cmds.go       (~150 LOC - urp sys *)
├── vec_cmds.go       (~100 LOC - urp vec *)
├── focus_cmds.go     (~100 LOC - urp focus)
├── container_cmds.go (~200 LOC - urp launch/spawn/etc)
├── skills.go         (ya existe)
├── backup.go         (ya existe)
└── opencode.go       (ya existe)
```

---

## ORDEN DE EJECUCIÓN

```
                    ┌─────────────────┐
                    │   FASE 1        │  ← START HERE
                    │  Quick Wins     │
                    │  (2-3 horas)    │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
       ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
       │   FASE 3    │ │   FASE 4    │ │   FASE 5    │
       │    DIP      │ │    OCP      │ │ Coherencia  │
       │ (2-3 horas) │ │ (2-3 horas) │ │ (2-3 horas) │
       └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
              │               │               │
              └───────────────┼───────────────┘
                              ▼
                    ┌─────────────────┐
                    │    FASE 2       │  ← MAYOR ESFUERZO
                    │  Split Objects  │
                    │  (6-8 horas)    │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │    FASE 6       │
                    │  Split main.go  │
                    │  (4-5 horas)    │
                    └─────────────────┘
```

---

## MÉTRICAS DE ÉXITO

| Métrica | Antes | Objetivo |
|---------|-------|----------|
| Duplicados exactos | 67 | 0 |
| Archivos >500 LOC | 8 | 2 |
| God Objects | 5 | 0 |
| Test coverage | 55% | 75% |
| Violaciones DIP | 6 | 0 |
| main.go LOC | 4,278 | <500 |

---

## ARCHIVOS CRÍTICOS (NO ROMPER)

```
PROTECTED (alta dependencia):
- internal/graph/driver.go (18 dependientes)
- internal/opencode/domain/* (15 dependientes)

TOUCH WITH CARE:
- internal/opencode/agent/agent.go (núcleo del sistema)
- internal/audit/healing.go (sistema inmune)
```

---

## COMANDOS DE VERIFICACIÓN

```bash
# Después de cada fase:
cd go && go build -o urp ./cmd/urp && go test ./...

# Verificar que no hay regresiones:
./urp doctor -v
./urp code stats
./urp sys vitals
```

---

## FASE 1.5: QUICK WINS ADICIONALES (Post-Auditoría Phase 2)

### 1.5.1 CommandRunner Interface

**Impacto: ALTO | Esfuerzo: MEDIO | Riesgo: BAJO**

80+ llamadas directas a `exec.Command` impiden testing:

```go
// internal/exec/runner.go
package exec

import (
    "bytes"
    "context"
    osexec "os/exec"
)

type Runner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
    RunInDir(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type OSRunner struct{}

func (r *OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
    cmd := osexec.CommandContext(ctx, name, args...)
    var out bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &out
    err := cmd.Run()
    return out.Bytes(), err
}

// MockRunner for testing
type MockRunner struct {
    Responses map[string][]byte
    Errors    map[string]error
}
```

**Archivos a modificar:**
- `internal/container/manager.go:190` (detectRuntime)
- `internal/orchestrator/orchestrator.go:176,297` (SpawnWorker)
- `internal/opencode/tool/diagnostics.go:135-364`
- `internal/skills/executor.go:161`
- `internal/ingest/git.go:50-200`

### 1.5.2 Aplicar requireDB() consistentemente

**67 db==nil checks en main.go/monitor.go que no usan helper:**

```bash
# Archivos afectados:
cmd/urp/main.go: ~45 instancias
cmd/urp/monitor.go: ~22 instancias
```

### 1.5.3 Centralizar Container Naming

```go
// internal/container/names.go
package container

func InfraName() string                    { return "urp-memgraph" }
func MasterName(project string) string     { return fmt.Sprintf("urp-master-%s", project) }
func WorkerName(project string, n int) string { return fmt.Sprintf("urp-%s-w%d", project, n) }
func NeMoName(project string) string       { return fmt.Sprintf("urp-nemo-%s", project) }
```

### 1.5.4 Centralizar URPEnv

```go
// internal/config/env.go
package config

type URPEnv struct {
    Project   string // URP_PROJECT
    SessionID string // URP_SESSION_ID
    HostPath  string // URP_HOST_PROJECT_PATH
    HostHome  string // URP_HOST_HOME
    WorkerID  string // URP_WORKER_ID
    Model     string // DEFAULT_MODEL
}

var env *URPEnv
var envOnce sync.Once

func Env() *URPEnv {
    envOnce.Do(func() {
        env = &URPEnv{
            Project:   os.Getenv("URP_PROJECT"),
            SessionID: os.Getenv("URP_SESSION_ID"),
            // ...
        }
    })
    return env
}
```

---

## RESUMEN FASE 1 COMPLETADA

| Tarea | Estado | LOC Eliminadas |
|-------|--------|----------------|
| `getString/getInt/etc` → `graph.Record` | ✅ | ~200 |
| `truncate` → `strings.Truncate` | ✅ | ~50 |
| Tests actualizados | ✅ | — |
| Build passing | ✅ | — |
| **TOTAL Phase 1** | ✅ | **~250 LOC** |

---

## PRÓXIMAS FASES

| Fase | Prioridad | Descripción |
|------|-----------|-------------|
| 1.5 | P0 | Quick wins adicionales (CommandRunner, requireDB, naming) |
| 2 | P1 | Split god objects (main.go, manager.go) |
| 3 | P2 | DIP - eliminar globals |
| 4 | P3 | OCP - strategy patterns |

---

*Updated: 2025-12-05*
*Talk is cheap. Show me the code.*
