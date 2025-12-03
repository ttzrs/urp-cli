# Tutorial Interactivo URP-CLI (Go)

## Guía de Aprendizaje Progresivo

```
╔═══════════════════════════════════════════════════════════════════════════════╗
║                         URP-CLI LEARNING PATH                                 ║
╠═══════════════════════════════════════════════════════════════════════════════╣
║                                                                               ║
║   NIVEL 1: Básico          NIVEL 2: Intermedio       NIVEL 3: Avanzado       ║
║   ───────────────          ──────────────────        ─────────────────       ║
║   • Comandos básicos       • Cognitive Skills        • Memory System         ║
║   • Code Analysis          • Focus (context)         • Knowledge Base        ║
║   • Git History            • Runtime Observation     • Memgraph Queries      ║
║                                                                               ║
║   Tiempo: 15 min           Tiempo: 30 min            Tiempo: 45 min          ║
║                                                                               ║
╚═══════════════════════════════════════════════════════════════════════════════╝
```

---

# NIVEL 1: Fundamentos (15 min)

## 1.1 Instalación y Verificación

```bash
# Compilar el binario Go
cd go && go build -o urp ./cmd/urp

# Verificar instalación
./urp version

# Ver estado (conecta a Memgraph si disponible)
./urp
```

**Deberías ver:**
- Versión del CLI
- Estado de conexión a Memgraph
- Proyecto actual detectado

---

## 1.2 Ingestar Código al Grafo

### Parsear código fuente

```bash
# Ingestar directorio actual
urp code ingest .

# Ver estadísticas del grafo
urp code stats
```

**Qué hace:**
- Parsea archivos Go y Python
- Crea nodos: `File`, `Function`, `Class`, `Struct`
- Crea edges: `CONTAINS`, `CALLS`

### Cargar historial Git

```bash
# Ingestar commits
urp git ingest .

# Ver historial de un archivo
urp git history main.go
```

**Qué hace:**
- Crea nodos: `Commit`, `Author`, `Branch`
- Crea edges: `PARENT_OF`, `AUTHORED`, `TOUCHED`

---

## 1.3 Consultas al Grafo de Conocimiento

### Impacto de cambios (Φ - Morfismo)

```bash
# ¿Qué se rompe si cambio esta función?
urp code impact main.go:runCommand

# ¿De qué depende esta función?
urp code deps internal/graph/driver.go:Query
```

### Detección de conflictos (⊥ - Ortogonal)

```bash
# Funciones que nadie llama
urp code dead

# Dependencias circulares
urp code cycles

# Archivos más modificados (alto riesgo)
urp code hotspots
```

---

## 1.4 Ejercicio Nivel 1

```bash
# MISIÓN: Analizar tu codebase
#
# 1. Ingestar código y git:
urp code ingest .
urp git ingest .

# 2. Explorar el grafo:
urp code stats
urp code hotspots

# 3. Buscar código muerto:
urp code dead

# 4. Ver impacto de una función clave:
urp code impact <tu_funcion>
```

**Checkpoint:** Si puedes ver estadísticas y relaciones, pasa al Nivel 2.

---

# NIVEL 2: Cognitive Skills (30 min)

## 2.1 Wisdom - Aprender de Errores Pasados

Cuando encuentres un error:

```bash
# Buscar errores similares en el historial
urp think wisdom "ModuleNotFoundError: No module named 'foo'"
```

**Resultado:**
- Si similarity > 80%: Aplica la solución histórica
- Si "PIONEER": Eres el primero, investiga y luego usa `learn`

### Ejemplo de flujo

```bash
# Error ocurre
$ python3 -c "import nonexistent"
ModuleNotFoundError: No module named 'nonexistent'

# Consultar sabiduría
$ urp think wisdom "ModuleNotFoundError nonexistent"

# Si no hay match, resolver y registrar
$ urp think learn "Fixed import error by installing package with pip"
```

---

## 2.2 Novelty - Detectar Patrones Inusuales

Antes de implementar código nuevo:

```bash
# Verificar si el patrón es inusual
urp think novelty "func (s *Service) Process() error { ... }"
```

**Interpretación:**
- 🟢 < 30%: Patrón estándar, procede
- 🟡 30-70%: Revisar, justificar elección
- 🔴 > 70%: **ALTO**. Explicar al usuario antes de implementar

---

## 2.3 Focus - Cargar Contexto Específico

### Token Economy

**Problema:** Leer archivos completos desperdicia tokens.
**Solución:** Cargar solo contexto relevante.

```bash
# Cargar función y dependencias directas
urp focus main.go:runCommand

# Cargar con profundidad 2 (2 niveles de dependencias)
urp focus main.go:runCommand -d 2
```

### Perfiles de Contexto

| Perfil | Tarea | Depth | Tokens |
|--------|-------|-------|--------|
| BUG FIX | Reparación quirúrgica | 1 | ~100 |
| REFACTOR | Cambios estructurales | 2 | ~200 |
| FEATURE | Copiar patrones | 1 | ~150 |
| DEBUG | Traza de errores | - | ~50 |

---

## 2.4 Observación del Runtime

```bash
# CPU/RAM de contenedores
urp sys vitals

# Mapa de red
urp sys topology

# Problemas de salud
urp sys health

# Runtime detectado (docker/podman)
urp sys runtime
```

---

## 2.5 Ejercicio Nivel 2

```bash
# MISIÓN: Usar cognitive skills en un flujo real
#
# 1. Simular un error:
python3 -c "import nonexistent"

# 2. Consultar sabiduría:
urp think wisdom "ModuleNotFoundError"

# 3. Cargar contexto de una función:
urp focus <alguna_funcion>

# 4. Verificar novedad de tu solución:
urp think novelty "pip install nonexistent"

# 5. Registrar éxito:
urp think learn "Resolved import by installing missing package"
```

**Checkpoint:** Si entiendes el flujo wisdom→solve→learn, pasa al Nivel 3.

---

# NIVEL 3: Sistema de Memoria (45 min)

## 3.1 Session Memory (Memoria Privada)

Tu espacio cognitivo para esta sesión:

```bash
# Recordar una nota
urp mem add "SELinux requiere :z para volúmenes"

# Buscar en memoria
urp mem recall "SELinux"

# Listar todo
urp mem list

# Estadísticas
urp mem stats

# Limpiar sesión
urp mem clear
```

**Cuándo usar:**
- Notas temporales
- Observaciones de debugging
- Decisiones de sesión

---

## 3.2 Knowledge Base (Conocimiento Compartido)

Conocimiento que persiste entre sesiones:

```bash
# Almacenar conocimiento
urp kb store "Docker socket requiere permisos 666"

# Buscar (cascade: session → instance → global)
urp kb query "docker socket"

# Listar todo
urp kb list

# Estadísticas
urp kb stats
```

### Promoción y Rechazo

```bash
# Promover memoria de sesión a global
urp kb promote m-xxx

# Rechazar conocimiento que no aplica
urp kb reject k-xxx "Diferente entorno, no aplica"
```

---

## 3.3 Terminal Events

```bash
# Ejecutar comando y loguearlo
urp events run "go test ./..."

# Ver comandos recientes
urp events list

# Ver solo errores
urp events errors
```

---

## 3.4 Queries Avanzados en Memgraph

```bash
# Conectar a Memgraph
docker exec -it urp-memgraph mgconsole
```

```cypher
// Ver todas las funciones
MATCH (f:Function) RETURN f.signature, f.file LIMIT 20;

// Dependencias de una función
MATCH (f:Function {signature: 'main.go:main'})-[:CALLS]->(dep)
RETURN f.signature, dep.signature;

// Funciones sin llamadas (código muerto)
MATCH (f:Function)
WHERE NOT (f)<-[:CALLS]-()
RETURN f.signature;

// Hotspots (archivos más tocados)
MATCH (c:Commit)-[:TOUCHED]->(f:File)
RETURN f.path, count(c) AS touches
ORDER BY touches DESC LIMIT 10;
```

---

## 3.5 Ejercicio Nivel 3

```bash
# MISIÓN: Flujo completo de memoria
#
# 1. Agregar nota de sesión:
urp mem add "Probando sistema de memoria"

# 2. Buscar:
urp mem recall "memoria"

# 3. Almacenar conocimiento:
urp kb store "URP usa Memgraph como graph database"

# 4. Buscar conocimiento:
urp kb query "graph database"

# 5. Ver estadísticas:
urp mem stats
urp kb stats
```

---

# FLUJO DE TRABAJO DIARIO

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        FLUJO DE TRABAJO RECOMENDADO                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  INICIO DE SESIÓN                                                           │
│  ─────────────────                                                          │
│  1. urp                       → Verificar estado                            │
│  2. urp code ingest .         → Actualizar grafo                           │
│  3. urp git ingest .          → Sincronizar historial                      │
│                                                                             │
│  DURANTE EL TRABAJO                                                         │
│  ──────────────────                                                         │
│  4. urp focus <target>        → Cargar contexto relevante                  │
│  5. urp events errors         → Si hay errores, consultar historial       │
│  6. urp think wisdom "error"  → Buscar soluciones previas                  │
│  7. urp sys vitals            → Monitorear recursos                        │
│                                                                             │
│  AL RESOLVER UN PROBLEMA                                                    │
│  ───────────────────────                                                    │
│  8. urp think learn "desc"    → Guardar solución para futuro              │
│  9. urp mem add "nota"        → Notas de sesión                            │
│                                                                             │
│  FIN DE SESIÓN                                                              │
│  ─────────────                                                              │
│  10. urp kb store "insight"   → Promover conocimiento útil                 │
│  11. urp code stats           → Ver estado final                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

# QUICK REFERENCE

## Comandos Esenciales

| Comando | Propósito |
|---------|-----------|
| `urp` | Estado del sistema |
| `urp version` | Versión del CLI |
| `urp code ingest .` | Parsear código |
| `urp git ingest .` | Cargar historial |
| `urp code stats` | Estadísticas |
| `urp code impact <sig>` | Impacto de cambios |
| `urp code deps <sig>` | Dependencias |
| `urp code dead` | Código muerto |
| `urp focus <target>` | Cargar contexto |
| `urp think wisdom <error>` | Buscar soluciones |
| `urp think learn <desc>` | Guardar conocimiento |
| `urp mem add <text>` | Nota de sesión |
| `urp kb store <text>` | Conocimiento global |
| `urp sys vitals` | Recursos |
| `urp events errors` | Errores recientes |

## Primitivas PRU

| Primitiva | Símbolo | Comandos |
|-----------|---------|----------|
| Domain | D | `code ingest`, `code stats` |
| Vector | τ | `git ingest`, `git history`, `events` |
| Morphism | Φ | `code deps`, `code impact`, `sys vitals` |
| Inclusion | ⊆ | `focus` (jerarquía código) |
| Orthogonal | ⊥ | `code dead`, `code cycles`, `events errors` |
| Tensor | T | Contexto (branch, session) |

---

# SIGUIENTE PASO

Ahora que completaste el tutorial:

1. **Usa el sistema diariamente** - La mejor manera de aprender
2. **Ingestar tu proyecto real** - `urp code ingest .`
3. **Consulta sabiduría ante errores** - `urp think wisdom`
4. **Contribuye soluciones con** - `urp think learn`

```bash
# Comando para recordar dónde dejaste el tutorial:
echo "Tutorial Go completado: $(date)" >> ~/.urp_progress
```
