# URP - Diagrama de Flujo de Arquitectura

## 1. Flujo Principal de Ejecución

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USUARIO                                         │
│                                                                              │
│   $ urp                    $ urp code ingest        $ urp launch             │
│   (sesión interactiva)     (comando específico)     (orquestación)           │
└──────────────┬─────────────────────┬─────────────────────┬──────────────────┘
               │                     │                     │
               ▼                     ▼                     ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           cmd/urp/main.go                                    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  PersistentPreRun:                                                   │    │
│  │    1. Conectar a Memgraph (graph.Connect)                           │    │
│  │    2. Configurar AuditLogger                                        │    │
│  │    3. Inicializar VectorStore                                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Grupos de Comandos:                                                         │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │
│  │    infra     │ │   analysis   │ │  cognitive   │ │   runtime    │        │
│  │ launch,spawn │ │ code,git,    │ │ think,mem,   │ │ sys,events,  │        │
│  │ workers,ask  │ │ focus        │ │ kb,vec,plan  │ │ session,audit│        │
│  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘        │
└──────────────────────────────────────────────────────────────────────────────┘
```

## 2. Modo Interactivo (TUI/Agent)

```
$ urp
   │
   ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          bootstrap.Initialize()                               │
│                                                                               │
│  1. loadEnvFile()           → Cargar .env                                    │
│  2. graph.Connect()         → Conectar Memgraph                              │
│  3. graphstore.New()        → Store para sesiones                            │
│  4. provider.CreateForModel()→ Inicializar LLM (Anthropic/OpenAI/DeepSeek)   │
│  5. agent.New()             → Crear agente con tools                         │
│  6. orchestrator.New()      → Sistema de workers (opcional)                  │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              tui.RunAgent()                                   │
│                           (Bubble Tea TUI)                                    │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐     │
│  │                      Vista Principal                                 │     │
│  │  ┌─────────────────────────────────────────────────────────────┐   │     │
│  │  │  📊 Memory: ████████░░ 80%   Tokens: 12.4k/100k             │   │     │
│  │  ├─────────────────────────────────────────────────────────────┤   │     │
│  │  │                                                              │   │     │
│  │  │  🤖 Agent: Analizando el código...                          │   │     │
│  │  │                                                              │   │     │
│  │  │  > Ejecutando tool: file_read                                │   │     │
│  │  │  > Resultado: 45 líneas leídas                               │   │     │
│  │  │                                                              │   │     │
│  │  ├─────────────────────────────────────────────────────────────┤   │     │
│  │  │  > Tu mensaje aquí...                              [Enter]   │   │     │
│  │  └─────────────────────────────────────────────────────────────┘   │     │
│  └─────────────────────────────────────────────────────────────────────┘     │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
                                    ▼
```

## 3. Ciclo del Agente (Agent Loop)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            AGENT LOOP                                        │
│                                                                              │
│   Usuario Input                                                              │
│        │                                                                     │
│        ▼                                                                     │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                     agent.Run(ctx, prompt)                          │   │
│   └──────────────────────────────┬──────────────────────────────────────┘   │
│                                  │                                           │
│        ┌─────────────────────────┼─────────────────────────┐                │
│        ▼                         ▼                         ▼                │
│   ┌──────────┐           ┌──────────────┐          ┌──────────────┐        │
│   │ Classify │           │ Build Prompt │          │Route Model   │        │
│   │  Task    │           │ + Context    │          │(cost/perf)   │        │
│   └────┬─────┘           └──────┬───────┘          └──────┬───────┘        │
│        │                        │                         │                 │
│        └────────────────────────┼─────────────────────────┘                 │
│                                 ▼                                            │
│                    ┌───────────────────────┐                                │
│                    │   LLM Provider Call   │                                │
│                    │  (Anthropic/OpenAI/   │                                │
│                    │   DeepSeek/Proxy)     │                                │
│                    └───────────┬───────────┘                                │
│                                │                                             │
│                                ▼                                             │
│                    ┌───────────────────────┐                                │
│                    │   Parse Response      │                                │
│                    │   - Text content      │                                │
│                    │   - Tool calls        │                                │
│                    └───────────┬───────────┘                                │
│                                │                                             │
│              ┌─────────────────┴─────────────────┐                          │
│              │ Tool Call?                        │                          │
│              │                                   │                          │
│         ┌────┴────┐                        ┌─────┴─────┐                    │
│         │   YES   │                        │    NO     │                    │
│         ▼         │                        ▼           │                    │
│   ┌───────────────┴──┐              ┌─────────────┐   │                    │
│   │  Tool Executor   │              │  Return     │   │                    │
│   │                  │              │  Response   │   │                    │
│   │ bash, file_read, │              └─────────────┘   │                    │
│   │ file_write, grep │                                │                    │
│   │ browser, sandbox │                                │                    │
│   └────────┬─────────┘                                │                    │
│            │                                          │                    │
│            ▼                                          │                    │
│   ┌────────────────┐                                  │                    │
│   │ Tool Result    │───────────┐                      │                    │
│   └────────────────┘           │                      │                    │
│            │                   │                      │                    │
│            └───────────────────┴──────────────────────┘                    │
│                                │                                            │
│                                ▼                                            │
│                    ┌───────────────────────┐                                │
│                    │   Continue Loop?      │                                │
│                    │   (más tool calls)    │                                │
│                    └───────────────────────┘                                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 4. Sistema de Herramientas (Tools)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           TOOL REGISTRY                                      │
│                      internal/opencode/tool/                                 │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        File Operations                               │    │
│  │  file_read    file_write    file_edit    file_glob    file_grep     │    │
│  │  file_ls      multiedit     patch                                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Execution                                    │    │
│  │  bash         sandbox       diagnostics                              │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Browser/Web                                  │    │
│  │  browser      web          screenshot                                │    │
│  │  (go-rod)     (fetch)      (screencapture)                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Advanced                                     │    │
│  │  graph        codesearch   lsp_hover    mcp         computer        │    │
│  │  task         todo         multi_expert batch                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 5. Orquestación Master-Worker

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HOST                                            │
│                                                                              │
│   $ urp launch myproject                                                     │
│        │                                                                     │
│        ▼                                                                     │
│   ┌────────────────────────────────────────────────────────────────────┐    │
│   │                    Docker Network: urp-network                      │    │
│   │                                                                     │    │
│   │  ┌──────────────────┐                                              │    │
│   │  │   MEMGRAPH       │◄─────────────────────────────────┐           │    │
│   │  │  (Graph DB)      │                                   │           │    │
│   │  │  :7687           │                                   │           │    │
│   │  └──────────────────┘                                   │           │    │
│   │           ▲                                             │           │    │
│   │           │                                             │           │    │
│   │  ┌────────┴─────────────────────────────────────────────┴───────┐  │    │
│   │  │                                                               │  │    │
│   │  │                    MASTER CONTAINER                           │  │    │
│   │  │                   (urp:master target)                         │  │    │
│   │  │                                                               │  │    │
│   │  │   ┌─────────────────────────────────────────────────────┐    │  │    │
│   │  │   │                  Orchestrator                        │    │  │    │
│   │  │   │                                                      │    │  │    │
│   │  │   │   Plan → Tasks → Assign to Workers → Collect Results │    │  │    │
│   │  │   └──────────────────────┬──────────────────────────────┘    │  │    │
│   │  │                          │                                    │  │    │
│   │  │   /workspace (project:ro)│                                    │  │    │
│   │  │   /var/run/docker.sock   │  Protocol (JSON-lines stdin/out)   │  │    │
│   │  └──────────────────────────┼────────────────────────────────────┘  │    │
│   │                             │                                       │    │
│   │            ┌────────────────┼────────────────┐                      │    │
│   │            ▼                ▼                ▼                      │    │
│   │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │    │
│   │  │   WORKER 1   │  │   WORKER 2   │  │   WORKER N   │              │    │
│   │  │  (urp:worker)│  │  (urp:worker)│  │  (urp:worker)│              │    │
│   │  │              │  │              │  │              │              │    │
│   │  │ project:rw   │  │ project:rw   │  │ project:rw   │              │    │
│   │  │ go,python    │  │ go,python    │  │ go,python    │              │    │
│   │  │ ejecuta task │  │ ejecuta task │  │ ejecuta task │              │    │
│   │  └──────────────┘  └──────────────┘  └──────────────┘              │    │
│   │                                                                     │    │
│   └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 6. Protocolo de Comunicación (Envelope)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         JSON-Lines Protocol                                  │
│                     internal/protocol/envelope.go                            │
│                                                                              │
│   MASTER ────────────────────────────────────────────────────────► WORKER   │
│                                                                              │
│   {"type":"instruction","task_id":"t1","command":"code dead"}               │
│   {"type":"instruction","task_id":"t2","command":"code cycles"}             │
│                                                                              │
│   WORKER ────────────────────────────────────────────────────────► MASTER   │
│                                                                              │
│   {"type":"status","task_id":"t1","status":"running"}                       │
│   {"type":"result","task_id":"t1","success":true,"output":"..."}            │
│   {"type":"error","task_id":"t2","error":"timeout"}                         │
│                                                                              │
│   Tipos de Mensaje:                                                          │
│   ┌────────────────┬─────────────────────────────────────────────────┐      │
│   │ instruction    │ Master → Worker: ejecutar tarea                 │      │
│   │ status         │ Worker → Master: estado actual                  │      │
│   │ result         │ Worker → Master: tarea completada               │      │
│   │ error          │ Bidireccional: reportar error                   │      │
│   │ capability     │ Worker → Master: capacidades disponibles        │      │
│   └────────────────┴─────────────────────────────────────────────────┘      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 7. Capas de Almacenamiento

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DATA LAYER                                         │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         MEMGRAPH (Graph DB)                           │   │
│  │                                                                       │   │
│  │   Nodos:                           Relaciones:                        │   │
│  │   (:File)                          [:CONTAINS]                        │   │
│  │   (:Function)                      [:CALLS]                           │   │
│  │   (:Class)                         [:IMPORTS]                         │   │
│  │   (:Commit)                        [:AUTHORED]                        │   │
│  │   (:Session)                       [:HAS_MESSAGE]                     │   │
│  │   (:Message)                       [:EXECUTED]                        │   │
│  │   (:TerminalEvent)                 [:RESOLVED_BY]                     │   │
│  │   (:Memory)                        [:RELATES_TO]                      │   │
│  │   (:Knowledge)                                                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                      VECTOR STORE (Embeddings)                        │   │
│  │                                                                       │   │
│  │   internal/vector/memgraph_store.go                                   │   │
│  │                                                                       │   │
│  │   - Almacena embeddings en Memgraph (propiedad :embedding)           │   │
│  │   - Búsqueda por coseno similarity                                    │   │
│  │   - TEI Embedder / OpenAI Embedder                                    │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         FILE SYSTEM                                   │   │
│  │                                                                       │   │
│  │   /var/lib/urp/                                                       │   │
│  │   ├── sessions/        # Session data                                 │   │
│  │   ├── vector/          # Vector cache                                 │   │
│  │   └── alerts/          # Alert logs                                   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 8. Proveedores LLM

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         LLM PROVIDER FACTORY                                 │
│                   internal/opencode/provider/factory.go                      │
│                                                                              │
│                              ┌─────────────┐                                │
│                              │   Request   │                                │
│                              └──────┬──────┘                                │
│                                     │                                        │
│                                     ▼                                        │
│                         ┌───────────────────────┐                           │
│                         │    Model Router       │                           │
│                         │  (cost/performance)   │                           │
│                         └───────────┬───────────┘                           │
│                                     │                                        │
│          ┌──────────────────────────┼──────────────────────────┐            │
│          ▼                          ▼                          ▼            │
│   ┌─────────────┐          ┌─────────────┐          ┌─────────────┐        │
│   │  Anthropic  │          │   OpenAI    │          │  DeepSeek   │        │
│   │   Claude    │          │  GPT-4/o1   │          │   V3/R1     │        │
│   └─────────────┘          └─────────────┘          └─────────────┘        │
│          │                          │                          │            │
│          │                          │                          │            │
│          ▼                          ▼                          ▼            │
│   ┌─────────────┐          ┌─────────────┐          ┌─────────────┐        │
│   │   Direct    │          │   Direct    │          │   Direct    │        │
│   │    API      │          │    API      │          │    API      │        │
│   └─────────────┘          └─────────────┘          └─────────────┘        │
│                                     │                                        │
│                          ┌──────────┴──────────┐                            │
│                          ▼                     ▼                            │
│                   ┌─────────────┐       ┌─────────────┐                     │
│                   │   Unified   │       │   Google    │                     │
│                   │   (Proxy)   │       │   Gemini    │                     │
│                   └─────────────┘       └─────────────┘                     │
│                                                                              │
│   Prioridad de fallback:                                                     │
│   1. Proxy (si PROXY_API_KEY)                                               │
│   2. DeepSeek (si DEEPSEEK_API_KEY)                                         │
│   3. OpenAI (si OPENAI_API_KEY)                                             │
│   4. Anthropic (si ANTHROPIC_API_KEY)                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 9. Resumen de Flujo Completo

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│    1. ENTRADA                                                                │
│       └─► CLI (Cobra) → Parsea comando                                      │
│                                                                              │
│    2. INICIALIZACIÓN                                                         │
│       └─► Bootstrap → Conecta DB, Providers, Tools                          │
│                                                                              │
│    3. EJECUCIÓN                                                              │
│       ├─► Comando directo: urp code ingest → Ejecuta y sale                 │
│       └─► Sesión interactiva: urp → TUI + Agent Loop                        │
│                                                                              │
│    4. AGENT LOOP (si interactivo)                                           │
│       ├─► Recibe prompt usuario                                             │
│       ├─► Clasifica tarea                                                   │
│       ├─► Selecciona modelo (routing)                                       │
│       ├─► Llama LLM                                                         │
│       ├─► Ejecuta tools si necesario                                        │
│       └─► Repite hasta completar                                            │
│                                                                              │
│    5. ORQUESTACIÓN (si multi-worker)                                        │
│       ├─► Master planifica tareas                                           │
│       ├─► Workers ejecutan en paralelo                                      │
│       ├─► Resultados via protocol JSON-lines                                │
│       └─► Master consolida resultados                                       │
│                                                                              │
│    6. PERSISTENCIA                                                           │
│       ├─► Sesiones → Memgraph                                               │
│       ├─► Código/Git → Memgraph (grafo)                                     │
│       ├─► Embeddings → Vector Store                                         │
│       └─► Logs/Auditoría → FileSystem + Graph                               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

*Generado automáticamente - Refleja la arquitectura actual del código*
