# TODO - URP CLI

## 🏗️ PLAN: Integración OpenCode Features

### Filosofía de Integración

```
PRINCIPIO: Cada feature debe mapear a los PRU primitives
           D (Domain) | τ (Temporal) | Φ (Morphism) | ⊆ (Inclusion) | ⊥ (Orthogonal)

SOLID en Go:
- S: Un paquete = una responsabilidad
- O: Interfaces para extensión, structs cerrados
- L: Interfaces pequeñas (io.Reader pattern)
- I: Interfaces específicas por consumidor
- D: Depender de interfaces, no implementaciones
```

---

## FASE 1: Core Tools ✅ COMPLETADO

### 1.1 Task Tool (Subagentes) ✅

**PRU**: Φ (Morphism) - flujo de control entre agentes

```
go/internal/opencode/tool/task.go
go/internal/opencode/subagent/
├── subagent.go        # interface SubAgent
├── registry.go        # mapa de subagentes disponibles
├── types.go           # Explore, Plan, Build, etc.
└── executor.go        # ejecución aislada
```

**Interfaces (SOLID-D)**:
```go
// tool/task.go
type SubAgentExecutor interface {
    Execute(ctx context.Context, prompt string, cfg SubAgentConfig) (*Result, error)
}

type SubAgentConfig struct {
    Type        string            // "explore", "plan", "build"
    Model       string            // override model
    Tools       []string          // tools disponibles
    WorkDir     string
    Timeout     time.Duration
}
```

**Graph Integration**:
```cypher
(:Session)-[:SPAWNED]->(:SubTask {type, prompt, status})
(:SubTask)-[:PRODUCED]->(:Message)
```

### 1.2 MultiEdit Tool ✅

**PRU**: Φ (Morphism) - múltiples transformaciones atómicas

```
go/internal/opencode/tool/multiedit.go
```

**Diseño**:
```go
type MultiEdit struct {
    workDir string
    editor  *Edit  // reusar Edit existente (SOLID-O)
}

type MultiEditArgs struct {
    FilePath string     `json:"file_path"`
    Edits    []EditOp   `json:"edits"`
}

type EditOp struct {
    OldString string `json:"old_string"`
    NewString string `json:"new_string"`
}

// Ejecuta todas o ninguna (transaccional)
func (m *MultiEdit) Execute(ctx context.Context, args map[string]any) (*Result, error)
```

### 1.3 TodoWrite/Read Tools ✅

**PRU**: τ (Temporal) - tracking de progreso

```
go/internal/opencode/tool/todo.go
go/internal/opencode/domain/todo.go
```

**Modelo**:
```go
// domain/todo.go
type Todo struct {
    ID        string    `json:"id"`
    Content   string    `json:"content"`
    Status    string    `json:"status"` // pending, in_progress, completed
    CreatedAt time.Time `json:"created_at"`
    SessionID string    `json:"session_id"`
}

// Almacenado en session context (no requiere Memgraph)
```

### 1.4 Patch Tool (Unified Diff) ✅

**PRU**: Φ (Morphism) - transformación vía diff

```
go/internal/opencode/tool/patch.go
go/internal/opencode/patch/
├── parser.go    # parsear unified diff
├── applier.go   # aplicar parches
└── validator.go # validar antes de aplicar
```

---

## FASE 2: Slash Commands ✅ COMPLETADO

### 2.1 Command System ✅

**PRU**: D (Domain) - comandos como entidades

```
go/internal/opencode/command/
├── command.go     # interface Command
├── registry.go    # registro de comandos
├── builtin/
│   ├── init.go    # /init - crear AGENTS.md
│   └── review.go  # /review - code review
├── custom/
│   └── loader.go  # cargar desde .urp/commands/
└── template/
    ├── init.txt
    └── review.txt
```

**Interface (SOLID-I)**:
```go
type Command interface {
    Name() string
    Description() string
    Execute(ctx context.Context, args string, sess *Session) error
}

type TemplatedCommand struct {
    name     string
    desc     string
    template string
    agent    string  // optional: run with specific agent
}
```

### 2.2 TUI Integration

**Modificar** `go/internal/tui/agent.go`:

```go
// En Update(), case "enter":
if strings.HasPrefix(prompt, "/") {
    return m, m.handleSlashCommand(prompt)
}

func (m *AgentModel) handleSlashCommand(input string) tea.Cmd {
    parts := strings.SplitN(input[1:], " ", 2)
    cmdName := parts[0]
    args := ""
    if len(parts) > 1 {
        args = parts[1]
    }

    cmd, ok := command.Get(cmdName)
    if !ok {
        return m.showError("Unknown command: /" + cmdName)
    }

    return m.executeCommand(cmd, args)
}
```

### 2.3 Comandos Builtin

**/init**:
```
Analiza el codebase y crea AGENTS.md con:
1. Build/lint/test commands
2. Code style guidelines
3. Patrones del proyecto
```

**/review [commit|branch|pr]**:
```
Reviews cambios:
- Sin args: uncommitted changes
- Commit SHA: ese commit
- Branch name: diff vs HEAD
- PR URL/number: via gh cli
```

---

## FASE 3: Session Management ✅ COMPLETADO

### 3.1 Session Compaction

**PRU**: τ (Temporal) - compresión de historial

```
go/internal/opencode/session/compaction.go
```

**Algoritmo**:
```go
type Compactor struct {
    maxTokens   int
    summarizer  Summarizer  // interface for LLM call
}

func (c *Compactor) Compact(ctx context.Context, messages []Message) ([]Message, error) {
    // 1. Calcular tokens actuales
    // 2. Si > threshold, resumir mensajes antiguos
    // 3. Mantener últimos N mensajes intactos
    // 4. Crear CompactionMessage con resumen
}
```

**Graph**:
```cypher
(:Session)-[:HAS_COMPACTION]->(:Compaction {summary, token_count, created_at})
```

### 3.2 Title Generation

**PRU**: D (Domain) - metadata de sesión

```go
// session/title.go
func GenerateTitle(ctx context.Context, firstMessage string, prov llm.Provider) (string, error) {
    // Usar modelo pequeño/rápido
    // Prompt: "Generate a 5-word title for: <message>"
    // Max 100 chars
}
```

### 3.3 @ File References

**PRU**: ⊆ (Inclusion) - archivos en contexto

```go
// input/parser.go
var fileRefRegex = regexp.MustCompile(`@([^\s,.\`]+)`)

func ParseFileRefs(input string) []FileRef {
    // Extraer @paths
    // Resolver relativos a workDir
    // Validar que existen
}

// En prompt handling:
func expandFileRefs(prompt string, refs []FileRef) (string, []FilePart) {
    // Leer archivos
    // Añadir como FilePart al mensaje
    // Reemplazar @path con indicador
}
```

---

## FASE 4: Agent System ✅ COMPLETADO

### 4.1 Agent Registry

**PRU**: D (Domain) - agentes como entidades

```
go/internal/opencode/agent/
├── agent.go       # Agent struct y métodos
├── builtin.go     # build, plan, explore, etc.
├── registry.go    # registro global
├── config.go      # configuración por agente
└── permission.go  # permisos de herramientas
```

**Tipos de Agente**:
```go
var BuiltinAgents = map[string]AgentConfig{
    "build": {
        Prompt: "You are a coding assistant...",
        Tools:  AllTools,
        Mode:   "primary",
    },
    "plan": {
        Prompt: "You are a software architect...",
        Tools:  []string{"read", "glob", "grep", "ls"},
        Mode:   "subagent",
    },
    "explore": {
        Prompt: "You explore codebases...",
        Tools:  []string{"read", "glob", "grep", "ls"},
        Mode:   "subagent",
    },
}
```

### 4.2 Agent Cycling en TUI

```go
// Keyboard shortcut: Ctrl+A o Tab
case "ctrl+a":
    m.cycleAgent()

func (m *AgentModel) cycleAgent() {
    agents := []string{"build", "plan", "explore"}
    current := m.currentAgent
    for i, a := range agents {
        if a == current {
            m.currentAgent = agents[(i+1)%len(agents)]
            break
        }
    }
    m.updateStatusBar()
}
```

---

## FASE 5: Observabilidad ✅ COMPLETADO

### 5.1 LSP Integration

**PRU**: ⊥ (Orthogonal) - diagnósticos externos

```
go/internal/opencode/lsp/
├── client.go      # LSP client
├── hover.go       # hover info
├── diagnostic.go  # error/warnings
└── symbols.go     # document symbols
```

**Tools**:
```go
// tool/lsp_hover.go
type LSPHover struct {
    client *lsp.Client
}

// tool/lsp_diagnostics.go
type LSPDiagnostics struct {
    client *lsp.Client
}
```

### 5.2 CodeSearch Tool

**PRU**: Φ (Morphism) - búsqueda semántica

```go
// tool/codesearch.go
type CodeSearch struct {
    vecStore *vector.Store
}

func (c *CodeSearch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
    query := args["query"].(string)
    results := c.vecStore.Search(ctx, query, 10)
    // Format results with file:line references
}
```

---

## FASE 6: Advanced Features ✅ COMPLETADO

### 6.1 Batch Tool

**PRU**: Φ (Morphism) - ejecución paralela

```go
// tool/batch.go
type Batch struct {
    registry *Registry  // para ejecutar otros tools
}

type BatchArgs struct {
    Operations []BatchOp `json:"operations"`
    Parallel   bool      `json:"parallel"`
}

type BatchOp struct {
    Tool string         `json:"tool"`
    Args map[string]any `json:"args"`
}
```

### 6.2 Invalid Tool (Auto-repair)

**PRU**: ⊥ (Orthogonal) - manejo de errores

```go
// tool/invalid.go - catch-all para tool calls malformados
type Invalid struct{}

func (i *Invalid) Execute(ctx context.Context, args map[string]any) (*Result, error) {
    toolName := args["tool"].(string)
    errorMsg := args["error"].(string)

    return &Result{
        Output: fmt.Sprintf("Tool '%s' was called incorrectly: %s\n"+
            "Please check the tool's parameters and try again.",
            toolName, errorMsg),
    }, nil
}
```

### 6.3 Session Share

**PRU**: Φ (Morphism) - exportar/importar

```go
// session/share.go
func Export(ctx context.Context, sessionID string) ([]byte, error) {
    // Serializar sesión + mensajes a JSON
}

func Import(ctx context.Context, data []byte) (*Session, error) {
    // Crear nueva sesión desde JSON
}

// CLI: urp oc session share <id> [--url]
// Genera link compartible o archivo
```

---

## Registro de Herramientas Final

```go
// tool/tool.go - DefaultRegistry actualizado
func DefaultRegistry(workDir string) *Registry {
    r := NewRegistry()

    // Existentes
    r.Register(NewBash(workDir))
    r.Register(NewRead())
    r.Register(NewWrite())
    r.Register(NewEdit())
    r.Register(NewGlob(workDir))
    r.Register(NewGrep(workDir))
    r.Register(NewLS(workDir))
    r.Register(NewWebFetch())
    r.Register(NewWebSearch())
    r.Register(NewScreenshot())
    r.Register(NewScreenCapture())
    r.Register(NewComputer())
    r.Register(NewBrowser())

    // Nuevas FASE 1
    r.Register(NewTask(workDir))      // subagentes
    r.Register(NewMultiEdit(workDir))
    r.Register(NewTodoWrite())
    r.Register(NewTodoRead())
    r.Register(NewPatch(workDir))

    // Nuevas FASE 5
    r.Register(NewCodeSearch())
    r.Register(NewLSPHover())
    r.Register(NewLSPDiagnostics())

    // Nuevas FASE 6
    r.Register(NewBatch(r))  // pasa registry para ejecutar otros
    r.Register(NewInvalid())

    return r
}
```

---

## Estructura de Archivos Final

```
go/internal/opencode/
├── agent/
│   ├── agent.go
│   ├── builtin.go
│   ├── config.go
│   ├── executor.go
│   ├── permission.go
│   └── registry.go
├── command/
│   ├── command.go
│   ├── registry.go
│   ├── builtin/
│   │   ├── init.go
│   │   └── review.go
│   ├── custom/
│   │   └── loader.go
│   └── template/
│       ├── init.txt
│       └── review.txt
├── domain/
│   ├── ... (existente)
│   └── todo.go
├── hook/
├── lsp/
│   ├── client.go
│   ├── hover.go
│   ├── diagnostic.go
│   └── symbols.go
├── patch/
│   ├── parser.go
│   ├── applier.go
│   └── validator.go
├── permission/
├── provider/
├── session/
│   ├── ... (existente)
│   ├── compaction.go
│   ├── share.go
│   └── title.go
├── subagent/
│   ├── subagent.go
│   ├── registry.go
│   ├── types.go
│   └── executor.go
└── tool/
    ├── ... (existentes)
    ├── task.go
    ├── multiedit.go
    ├── todo.go
    ├── patch.go
    ├── codesearch.go
    ├── lsp_hover.go
    ├── lsp_diagnostics.go
    ├── batch.go
    └── invalid.go
```

---

## Testing Strategy

```
Para cada nuevo componente:
1. Unit tests con mocks (SOLID-D)
2. Integration tests con fixtures
3. E2E tests en TUI

Cobertura mínima: 70%
```

---

## 🔴 Crítico (Original)

### Responsividad UI ✅
- [x] **Ajustar texto al redimensionar ventana** en TUI (Bubble Tea)
  - Los paneles responden a cambios de tamaño de terminal
  - Viewport recalcula dimensiones en `tea.WindowSizeMsg`
  - Texto hace wrap correctamente (WordWrap con ANSI awareness)
  - Bordes y estilos se adaptan al ancho disponible

### Bugs Conocidos
- [ ] Verificar persistencia de sesiones cuando Memgraph se reinicia
- [x] Manejar errores de conexión a Memgraph más gracefully (ConnectWithRetry)
- [x] Validar paths de proyecto antes de `urp launch`

## 🟡 Mejoras Importantes

### TUI Interactivo
- [ ] Scroll suave en viewport de mensajes
- [ ] Colores personalizables vía config
- [ ] Panel de status en tiempo real (CPU/RAM de contenedores)
- [x] Navegación con vim-keys (j/k/g/G/Ctrl+u/Ctrl+f/Ctrl+b)
- [ ] Búsqueda en historial de sesiones (/)
- [ ] Preview de mensajes en lista de sesiones

### OpenCode Agent
- [ ] Soporte para múltiples workers en paralelo
- [ ] Dashboard de estado de workers activos
- [ ] Reintentos automáticos con backoff exponencial
- [x] Logs estructurados (JSON) para análisis (AgentLogger)
- [ ] Métricas de uso de tokens por sesión/proyecto

### Spec-Kit
- [ ] Templates de specs para casos comunes (API, CLI, Service)
- [ ] Validación de specs antes de ejecutar
- [ ] Diff entre spec y código generado
- [ ] Modo interactivo para crear specs (`urp spec wizard`)
- [ ] Exportar spec a PDF/HTML para documentación

### Vector Store
- [ ] Integrar embeddings de código en búsqueda semántica
- [ ] Auto-indexación de nuevos archivos en proyecto
- [ ] Búsqueda híbrida (keyword + semantic)
- [ ] Caché de embeddings para acelerar consultas

### Memoria Cognitiva
- [ ] Auto-aprendizaje de patrones exitosos
- [ ] Sugerencias proactivas basadas en contexto
- [ ] Exportar/importar memoria entre proyectos
- [ ] Limpieza automática de memorias obsoletas

## 🟢 Nice to Have

### CLI/UX
- [ ] Autocompletado bash/zsh para subcomandos
- [ ] Progress bars para operaciones largas
- [ ] Notificaciones desktop cuando workers terminan
- [ ] Modo verbose (`-v`, `-vv`, `-vvv`) con diferentes niveles
- [ ] Themes para output (dark/light/colorblind)

### Networking
- [ ] Soporte para Docker Swarm
- [ ] Networking multi-host (proyectos distribuidos)
- [ ] VPN integration para workers remotos
- [ ] Service mesh entre contenedores

### Seguridad
- [ ] Sandboxing adicional para workers (gVisor, Kata)
- [ ] Audit logs encriptados
- [ ] Secret management integrado (Vault)
- [ ] Rate limiting en API de LLM

### Documentación
- [ ] Video tutoriales de uso
- [ ] Ejemplos de casos de uso reales
- [ ] Arquitectura decision records (ADRs)
- [ ] API reference generada desde código

### Testing
- [ ] Tests de integración E2E completos
- [ ] Tests de performance (benchmarks)
- [ ] Tests de carga para multi-worker
- [ ] Fuzzing para parsers de código
- [ ] Coverage > 80% en todos los paquetes

### Observabilidad
- [ ] Integración con Prometheus/Grafana
- [ ] Tracing distribuido (OpenTelemetry)
- [ ] Alertas cuando workers fallan repetidamente
- [ ] Dashboard web para monitoreo

### Skills System
- [ ] Marketplace de skills compartidos
- [ ] Versionado de skills
- [ ] Dependencies entre skills
- [ ] Testing framework para skills

### Git Integration
- [ ] Auto-commit de cambios por workers
- [ ] Branch por worker (aislamiento)
- [ ] PR creation automática
- [ ] Git hooks personalizados

## 📦 Infraestructura

### Builds
- [ ] Multi-arch builds (amd64, arm64)
- [ ] Static binaries verdaderamente portables
- [ ] Reducir tamaño del binario (actualmente ~25MB)
- [ ] Builds reproducibles

### Distribución
- [ ] Homebrew formula
- [ ] APT/YUM repositories
- [ ] Snap/Flatpak packages
- [ ] Chocolatey para Windows
- [ ] Docker Hub automated builds

### CI/CD
- [ ] Tests automáticos en PRs
- [ ] Security scanning (Snyk, Trivy)
- [ ] Dependabot para actualizaciones
- [ ] Release notes automáticas
- [ ] Changelog generation

## 🔬 Experimental

### AI/ML
- [ ] Fine-tuning de modelos para URP específicamente
- [ ] Local LLM support (Ollama, llama.cpp)
- [ ] Embeddings locales para privacidad
- [ ] Reinforcement learning de decisiones

### Extensibilidad
- [ ] Plugin system (Go plugins o WASM)
- [ ] Custom providers de LLM
- [ ] Custom tools para agents
- [ ] Webhooks para eventos

### Datos
- [ ] Time-series DB para métricas históricas
- [ ] Data pipeline para análisis de uso
- [ ] ML para predecir fallas antes de ocurrir
- [ ] Knowledge graph visualization (D3.js)

### Multi-tenancy
- [ ] Soporte para equipos/organizaciones
- [ ] Permisos granulares por usuario
- [ ] Billing/quotas por proyecto
- [ ] Shared knowledge base

## 📝 Notas

### Prioridades Inmediatas
1. **FASE 1: Task Tool** - crítico para subagentes
2. **FASE 2: Slash Commands** - /init, /review
3. **Responsividad TUI** - impacto en UX

### Decisiones Tomadas
- Interfaces pequeñas al estilo Go (io.Reader pattern)
- Graph storage para todo lo persistente
- Subagentes como sesiones aisladas
- Comandos como templates + agentes

### Tech Debt
- Refactorizar `container/manager.go` (demasiado largo)
- Unificar manejo de errores (muchos patrones diferentes)
- Documentar interfaces públicas con godoc
- Eliminar código muerto detectado por análisis

---

**Última actualización:** 2024-12-05
**Mantenedor:** @joss
