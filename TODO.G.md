# Estado del Proyecto: Orquestador Total (OpenCode + Spec-Kit)

**Fecha:** 4 de Diciembre 2025
**Estrategia:** Strangler Fig (Evolución Paralela)

## 🟢 Estado Actual
**FASE 5 COMPLETADA** - URP defaults a modo OpenCode interactivo.

### Logros Fase 3
- [x] **Infraestructura Híbrida:** Contenedores `opencode-master` y `opencode-worker` desplegados junto a los legacy.
- [x] **Portabilidad del Núcleo:** Paquetes `agent`, `provider`, `tool`, `mcp` migrados y refactorizados.
- [x] **Spec-Kit Básico:** Motor de inicialización de proyectos (`urp spec init`) funcional.
- [x] **Conexión Proxy:** Provider OpenAI soporta endpoints compatibles (CLIProxyAPI).
- [x] **Modelo Correcto:** `claude-sonnet-4-5-20250929` configurado por defecto.
- [x] **Lectura de Specs:** `specs.Engine.ReadSpec()` inyecta contenido en prompt.
- [x] **Lectura de Constitution:** Reglas del proyecto incluidas en prompt.
- [x] **Persistencia Memgraph:** Sesiones y mensajes se guardan en grafo via `graphstore`.
- [x] **Ejecución Real:** Agente crea archivos, ejecuta tests, compila binarios.
- [x] **Primera Prueba Exitosa:** `urp spec run health` generó API completa con Clean Architecture.

### Logros Fase 4
- [x] **Networking Project-Scoped:** Cada proyecto tiene su propia red `urp-<project>-net`.
- [x] **Sin Port Mapping al Host:** Memgraph y contenedores solo accesibles via red interna.
- [x] **X11/Firefox en Master:** Soporte para GUIs web desde master container.
- [x] **Loop de Autocorrección:** Si tests fallan, agente reintenta hasta 3 veces con prompt de corrección.
- [x] **urp launch:** Master container con proyecto read-only (ya existente).
- [x] **urp spawn:** Workers efímeros con write access (ya existente).
- [x] **Envelope Protocol:** JSON-lines en `internal/protocol/` (ya existente).
- [x] **urp ask:** Comunicación master→worker via Claude CLI.

## ✅ Prueba Verificada
```
prueba-orquestador/
├── go.mod
├── server (binary)
├── cmd/server/main.go
└── internal/health/
    ├── handler/{health.go, health_test.go}
    └── usecase/{health.go, health_test.go}
```
Tests pasan. Spec cumplido: `/health` → `{"status":"ok"}` en puerto 8080.

## 📋 Tareas Pendientes (Roadmap)

### Fase 4: Completada
- [x] **Test E2E:** `TestE2E_OrchestrationFlow` en `container_test.go` (spawn→exec→kill).
- [x] **Test Aislamiento:** `TestE2E_ProjectIsolation` verifica redes separadas por proyecto.
- [x] **Test X11:** `TestE2E_X11Configuration`, `TestX11SocketMount`, `TestDisplayEnvPropagation`.

### Fase 5: Completada
- [x] `urp` sin argumentos lanza modo interactivo OpenCode.
- [x] `urp <path>` lanza sesión en directorio especificado.
- [x] `urp status` muestra estado de infraestructura (antiguo default).
- [x] Comandos legacy organizados en grupos (infra, analysis, cognitive, runtime).

### Fase 6: Completada
- [x] TUI interactivo con Bubble Tea (`urp tui`).
- [x] Histórico de sesiones navegable (tecla `s`).
- [x] Panel de estado con actualizaciones en tiempo real.
- [x] GitHub Actions CI (test, build, lint, docker).
- [x] GitHub Actions Release (GoReleaser).

## 🎉 Proyecto Completo
El orquestador URP está listo para producción.

## 🔧 Configuración Requerida
```bash
export OPENAI_API_KEY=sk-dummy
export OPENAI_BASE_URL=http://100.105.212.98:8317
```
El proxy CLIProxyAPI usa formato OpenAI (`/v1/chat/completions`).
