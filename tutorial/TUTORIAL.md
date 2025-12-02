# Tutorial Interactivo URP-CLI

## Guía de Aprendizaje Progresivo

```
╔═══════════════════════════════════════════════════════════════════════════════╗
║                         URP-CLI LEARNING PATH                                 ║
╠═══════════════════════════════════════════════════════════════════════════════╣
║                                                                               ║
║   NIVEL 1: Básico          NIVEL 2: Intermedio       NIVEL 3: Avanzado       ║
║   ───────────────          ──────────────────        ─────────────────       ║
║   • Comandos básicos       • Optimización            • A/B Testing           ║
║   • Percepción             • Working Memory          • PCx Experiments       ║
║   • Consultas              • Token Economy           • Memgraph Queries      ║
║                                                                               ║
║   Tiempo: 15 min           Tiempo: 30 min            Tiempo: 45 min          ║
║                                                                               ║
╚═══════════════════════════════════════════════════════════════════════════════╝
```

---

# NIVEL 1: Fundamentos (15 min)

## 1.1 Verificar Estado del Sistema

```bash
# Paso 1: Verifica que URP está activo
urp-status
```

**Deberías ver:**
- `URP_ENABLED=1`
- `Runner: OK`
- `Graph: CONNECTED` (si Memgraph está corriendo)

```bash
# Paso 2: Ver la topología de red
urp-topology
```

**Aprenderás:** Cómo tu contenedor se conecta al grafo compartido.

---

## 1.2 Percepción: Sentir el Sistema

### Pain - Ver errores recientes

```bash
# ¿Qué ha fallado recientemente?
pain

# Mirar más atrás (últimos 30 min)
pain --minutes 30
```

**Ejercicio:** Ejecuta un comando que falle y luego usa `pain` para verlo.

```bash
# Provoca un error
python3 -c "import nonexistent"

# Ahora mira el dolor
pain
```

### Recent - Ver comandos ejecutados

```bash
# ¿Qué comandos se han ejecutado?
recent

# Solo errores
recent --errors
```

### Vitals - Estado de recursos

```bash
# CPU, RAM del contenedor
vitals
```

---

## 1.3 Consultas al Grafo de Conocimiento

### Impacto de cambios

```bash
# ¿Qué se rompe si cambio esta función?
urp impact runner.py:add_to_focus

# ¿De qué depende esta función?
urp deps runner.py:_calculate_eviction_score
```

### Historia y hotspots

```bash
# Historia de cambios de un archivo
urp history context_manager.py

# Archivos más modificados (alto riesgo)
urp hotspots
```

### Código muerto

```bash
# Funciones que nadie llama
urp dead
```

---

## 1.4 Ejercicio Nivel 1

```bash
# MISIÓN: Diagnosticar el sistema
#
# 1. Ejecuta estos comandos en orden:
urp-status
pain
recent
vitals

# 2. Responde:
#    - ¿Hay errores recientes?
#    - ¿Cuántos comandos se han ejecutado?
#    - ¿Cómo está el uso de recursos?
```

**Checkpoint:** Si puedes responder estas preguntas, pasa al Nivel 2.

---

# NIVEL 2: Optimización de Contexto (30 min)

## 2.1 Entender Token Economy

```bash
# Ver uso actual de tokens
tokens-status

# Ver estado compacto
tokens-compact

# Ver presupuesto
tokens-budget
```

**Concepto clave:** Tienes ~50,000 tokens/hora. La optimización ayuda a usarlos mejor.

---

## 2.2 Sistema de Optimización (cc-*)

### Ver estado actual

```bash
# Estado completo de optimización
cc-status
```

**Output explicado:**
```
Mode: HYBRID              ← Modo actual de optimización
Working Memory:
  Items: 5 (2 old)        ← Elementos en memoria (2 viejos)
  Tokens: 1200            ← Tokens consumidos
Token Budget:
  Used: 15,000 / 50,000   ← Uso de presupuesto
Detected Noise:
  [medium] old_items: 2   ← Patrones de ruido detectados
```

### Detectar ruido

```bash
# ¿Qué está consumiendo tokens sin valor?
cc-noise
```

**Tipos de ruido:**
- `old_items`: Contexto > 30 min sin acceso
- `unused`: Items con bajo access_count
- `duplicate_basenames`: Múltiples archivos con mismo nombre
- `low_importance`: Items marcados como poco importantes

### Limpiar

```bash
# Limpiar según modo actual
cc-compact

# Limpiar manualmente
cc-clean           # Todo el ruido
cc-clean --old     # Solo items viejos
cc-clean --unused  # Solo items sin usar
cc-clean --all     # Reset total
```

---

## 2.3 Modos de Optimización

### Ver modo actual

```bash
cc-mode
```

### Cambiar modo

```bash
# Sin optimización (baseline para testing)
cc-none

# Semi-automático (genera instrucciones, tú decides)
cc-semi

# Automático (limpia agresivamente)
cc-auto

# Híbrido (balance inteligente) - RECOMENDADO
cc-smart
```

### Comparación de modos

| Modo | Ahorro | Retención | Cuándo usar |
|------|--------|-----------|-------------|
| none | 0% | 50% | Testing A/B |
| semi | 10% | 63% | Debug crítico |
| auto | 40% | 51% | Sesiones largas |
| **hybrid** | **30%** | **63%** | **Uso diario** |

---

## 2.4 Working Memory (Focus)

### Cargar contexto específico

```bash
# Cargar una función y sus dependencias
focus add_to_focus --depth 2

# Cargar un archivo
focus context_manager.py --depth 1
```

**depth explicado:**
- `--depth 1`: Solo el target
- `--depth 2`: Target + dependencias directas
- `--depth 3`: Target + 2 niveles de dependencias

### Descargar contexto

```bash
# Quitar algo de memoria
unfocus context_manager.py

# Limpiar toda la memoria
clear-context
```

### Ver memoria actual

```bash
mem-status
mem-context
```

---

## 2.5 Ejercicio Nivel 2

```bash
# MISIÓN: Optimizar una sesión de trabajo
#
# 1. Simula una sesión de trabajo:
focus runner.py --depth 1
focus context_manager.py --depth 1
focus brain_render.py --depth 1

# 2. Verifica estado:
cc-status
cc-noise

# 3. Espera 1 minuto y vuelve a verificar:
sleep 60
cc-status

# 4. Optimiza:
cc-compact

# 5. Compara el antes/después:
cc-stats
```

**Checkpoint:** Si entiendes la diferencia entre los modos, pasa al Nivel 3.

---

# NIVEL 3: A/B Testing y PCx (45 min)

## 3.1 Sistema de Métricas

### Registrar feedback

```bash
# Después de una sesión exitosa, registra calidad (1-5)
cc-quality 5

# Si perdiste contexto importante
cc-error
```

### Ver estadísticas

```bash
# Estadísticas por modo
cc-stats

# Recomendación basada en datos
cc-recommend
```

---

## 3.2 A/B Testing con ab-*

### Ejecutar test A/B

```bash
# Test rápido con 4 contenedores paralelos
ab-test "Refactorizar módulo X" "pytest tests/"
```

**Qué hace:**
1. Crea 4 contenedores (none, semi, auto, hybrid)
2. Crea 4 ramas git (ab/<session>/<mode>)
3. Ejecuta el mismo script en cada uno
4. Compara métricas
5. Guarda resultados en Memgraph

### Ver resultados

```bash
# Estadísticas agregadas
ab-stats

# Recomendación
ab-recommend

# Listar sesiones
ab-list
```

---

## 3.3 PCx - Performance Comparison eXperiment

### Ejecutar experimentos

```bash
# Workload simple (5 archivos, 1 error)
pcx-simple

# Workload medio (20 archivos, 5 errores)
pcx-medium

# Workload complejo (100 archivos, 20 errores)
pcx-complex

# Ejecutar todos
pcx-all
```

### Analizar resultados

```bash
# Comparar modos
pcx-compare

# Análisis desde Memgraph
pcx-analyze

# Exportar a CSV
pcx-export -o resultados.csv
```

### Interpretar resultados

```
Mode       Efficiency   Retention   Recovery
────────────────────────────────────────────
none           0%         50%         0%     ← Baseline
semi          10%         63%       100%     ← Conservador
auto          40%         51%       100%     ← Agresivo
hybrid        30%         63%       100%     ← Balance ✓
```

**Métricas:**
- **Efficiency**: tokens_saved / tokens_consumed
- **Retention**: contexto útil preservado
- **Recovery**: errores resueltos / errores totales

---

## 3.4 Queries Memgraph Avanzados

```bash
# Conectar a Memgraph
docker exec -it urp-memgraph mgconsole
```

```cypher
// Ver todos los experimentos PCx
MATCH (e:PCxExperiment)-[:TESTED]->(r:PCxResult)
RETURN e.experiment_id, e.workload, r.mode, r.efficiency_ratio
ORDER BY e.start_time DESC;

// Mejor modo por tipo de workload
MATCH (e:PCxExperiment)-[:TESTED]->(r:PCxResult)
WITH e.workload AS workload, r.mode AS mode,
     avg(r.efficiency_ratio) AS efficiency
RETURN workload, mode, efficiency
ORDER BY workload, efficiency DESC;

// Recomendación dinámica
MATCH (r:PCxResult)
WITH r.mode AS mode,
     avg(r.efficiency_ratio) * 0.3 +
     avg(r.context_retention) * 0.3 +
     avg(r.error_recovery) * 0.4 AS score
RETURN mode, score
ORDER BY score DESC
LIMIT 1;
```

---

## 3.5 Ejercicio Nivel 3

```bash
# MISIÓN: Ejecutar experimento completo y analizar
#
# 1. Ejecutar experimento:
pcx-simple

# 2. Ver resultados:
pcx-compare

# 3. Exportar datos:
pcx-export -o /tmp/mi_experimento.csv
cat /tmp/mi_experimento.csv

# 4. Registrar tu feedback:
cc-quality 4  # o el que corresponda

# 5. Ver estadísticas actualizadas:
cc-stats
cc-recommend
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
│  1. urp-status          → Verificar sistema                                 │
│  2. cc-mode             → Confirmar modo (hybrid recomendado)               │
│  3. tokens-status       → Ver presupuesto disponible                        │
│                                                                             │
│  DURANTE EL TRABAJO                                                         │
│  ──────────────────                                                         │
│  4. focus <target>      → Cargar contexto relevante                        │
│  5. pain                → Si hay errores, consultar historial              │
│  6. wisdom "error msg"  → Buscar soluciones previas                        │
│  7. cc-status           → Monitorear uso de tokens                         │
│                                                                             │
│  CUANDO TOKENS > 70%                                                        │
│  ───────────────────                                                        │
│  8. cc-noise            → Identificar ruido                                │
│  9. cc-compact          → Optimizar (según modo)                           │
│                                                                             │
│  AL RESOLVER UN PROBLEMA                                                    │
│  ───────────────────────                                                    │
│  10. learn "descripción" → Guardar solución para futuro                    │
│                                                                             │
│  FIN DE SESIÓN                                                              │
│  ─────────────                                                              │
│  11. cc-quality N       → Registrar satisfacción (1-5)                     │
│  12. cc-stats           → Ver rendimiento del modo                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

# QUICK REFERENCE

## Comandos Esenciales

| Comando | Propósito |
|---------|-----------|
| `urp-status` | Estado del sistema |
| `pain` | Errores recientes |
| `recent` | Comandos ejecutados |
| `focus <target>` | Cargar contexto |
| `cc-status` | Estado de optimización |
| `cc-compact` | Ejecutar optimización |
| `cc-mode` | Ver/cambiar modo |
| `wisdom "error"` | Buscar soluciones |
| `learn "desc"` | Guardar conocimiento |

## Modos de Optimización

| Comando | Modo | Descripción |
|---------|------|-------------|
| `cc-none` | none | Sin optimización |
| `cc-semi` | semi | Manual |
| `cc-auto` | auto | Agresivo |
| `cc-smart` | hybrid | Balance (default) |

## A/B Testing

| Comando | Propósito |
|---------|-----------|
| `ab-test` | Ejecutar test paralelo |
| `ab-stats` | Ver estadísticas |
| `ab-recommend` | Mejor modo |
| `pcx-simple/medium/complex` | Experimentos PCx |
| `pcx-compare` | Comparar resultados |

---

# SIGUIENTE PASO

Ahora que completaste el tutorial:

1. **Usa el sistema diariamente** - La mejor manera de aprender
2. **Ejecuta `pcx-all` semanalmente** - Genera datos para optimización
3. **Revisa `cc-recommend` mensualmente** - Ajusta tu modo según datos
4. **Contribuye soluciones con `learn`** - Mejora el conocimiento colectivo

```bash
# Comando para recordar dónde dejaste el tutorial:
echo "Tutorial completado: $(date)" >> ~/.urp_progress
```
