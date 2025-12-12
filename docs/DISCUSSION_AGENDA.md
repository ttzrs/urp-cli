# URP CLI: Agenda de Discusión & Puntos Críticos

**Preparado para**: Sesión de revisión arquitectónica
**Formato**: Preguntas + Escenarios + Decisiones

---

## 1. MASTER/WORKER: ¿ESTÁ REALMENTE IMPLEMENTADO?

### Contexto
CLAUDE.md describe un patrón sofisticado:
```bash
urp launch /path → Master container (read-only)
urp spawn → Worker container (read-write)
urp ask <worker> "fix bug" → Cross-container communication
```

### Problema
No hay evidencia de que estos comandos estén wired en código reciente:
- ✅ Docker isolation exists
- ✅ Container manager exists
- ❌ `urp ask` command no aparece
- ❌ Role-based checks no implementados
- ❌ Tests para orchestration ausentes

### Escenario de Prueba
```bash
# Test 1: ¿Existe el comando?
urp launch /tmp/test-project
# Expected: Master container started
# Actual: ?

# Test 2: ¿Detecta roles correctamente?
docker exec urp-master-test urp doctor
# Expected: "Role: master (read-only)"
# Actual: No hay detección de roles

# Test 3: ¿Previene escritura en master?
docker exec urp-master-test bash -c "echo 'test' > /workspace/file.txt"
# Expected: Permission denied (read-only)
# Actual: ?
```

### Preguntas para Discusión
1. **¿Es Master/Worker un work-in-progress o completamente funcional?**
2. **¿Si no está completo, cuál es el MVP para hacerlo funcional?**
3. **¿Hay casos de uso donde Master/Worker es crítico?**
4. **¿Debemos priorizarlo o enfocarse en simplificar lo que funciona?**

### Recomendación Provisional
- **Opción A**: Completar implementación (1 week sprint)
- **Opción B**: Desactivar en docs + agregar TODO en código
- **Opción C**: Mover a roadmap post-v2.1

---

## 2. CONTEXT COMPILER: ¿5 MODOS SON NECESARIOS?

### El Dilema

**Lado A (Pro-Compiler)**:
```
Ventajas:
- Dynamic adaptation to context
- Token budget aware
- Learns from previous sessions
- Picks best mode per task
```

**Lado B (Pro-Simple)**:
```
Ventajas (Anthropic style):
- Single JSON file (claude-progress.json)
- Read git log directly
- Minimal prompt overhead
- Predictable, debuggable
```

### Medición Requerida
```go
// Antes (5 modos):
// Token count per request: 50k-200k
// Compile time: 50-150ms
// Success rate: ?

// Después (simple):
// Token count per request: 20k-40k
// Compile time: <10ms
// Success rate: should be same
```

### Pregunta Clave
**¿Estamos optimizando tokens que no cuestan mucho?**

```
Anthropic:
- API no cobra por compilación
- Cobra por tokens en CREATE_MESSAGE
- Overhead del compilador: ~200 tokens = $0.0003/call
- Beneficio potencial: 50 tokens saved = $0.00015 saved

Net benefit: -$0.00015 por llamada.
¿Es worth la complejidad?
```

### Escenario de Comparación
```bash
# Medir token budget real
export URP_COST_ANALYSIS=true
urp oc agent "Fix a bug in main.go"

# Output esperado:
# Compile: 150 tokens used
# Message: 45k tokens used
# Thinking: 2k tokens used
# Tools: 5k tokens used
# Total: 52.15k tokens = $0.26 cost

# Con compilador simple:
# Compile: 30 tokens used
# Message: 47k tokens used (2k+ de overhead sin smart compile)
# Total: 47.03k tokens = $0.235 cost

# Savings: $0.025 = 9% cost reduction
# Complexity added: ~500 lines of code
# ROI: Probablemente negativo
```

### Preguntas para Discusión
1. **¿Tenemos métricas reales del compilador?**
2. **¿Cuál es el token savings medido?**
3. **¿Vale la pena la complejidad?**
4. **¿Debemos simplificar a 1 modo inteligente o 2 modos (full/minimal)?**

### Propuesta de Decisión
**Votar en 3 opciones**:
- A: Keep 5 modes, add metrics + documentation
- B: Simplify to 2 modes (full/minimal)
- C: Go full Anthropic style (single JSON)

---

## 3. MODEL ROUTER: 3 SISTEMAS O 1?

### Fragmentación Actual

```
System 1: ModelRouter (agent/model_router.go)
├─ TaskClassifier (what kind of work?)
├─ BudgetTracker (can we afford it?)
└─ ModelSelection (pick best)

System 2: ProviderFactory (provider/factory.go)
├─ Env var lookup
├─ Caching mechanism
└─ Error handling

System 3: ConfigModels (config/models.go)
├─ Builtin registry
├─ Fallback chains
└─ Tier system (1/2/3)
```

### Problema: Quién decide qué

```
Escenario: Task = "review code" (medium complexity)

Pregunta: ¿Quién elige el modelo?
Respuesta: No está claro.

¿ModelRouter? ¿Cuáles son sus reglas?
¿ProviderFactory? ¿Cómo cachea?
¿ConfigModels? ¿Cuál es el fallback?

Resultado: Debugging difficilísimo.
```

### Auditoría de Decisión Real

```bash
# Antes: Opaco
urp oc agent "review user.go"
# Silenciosamente usa deepseek porque presupuesto bajo
# O usa claude si se sintió el router como buena idea

# Después: Auditable
# [ROUTER DECISION LOG]
# Input: task="code_review", budget=$0.50
# Rules matched: ["high_quality_rule"]
# Selected: claude-opus-4-5
# Reason: "Code review requires high quality"
# Confidence: 0.95
# Estimated cost: $0.03
# Decision: APPROVE

# Si es rechazo:
# [FALLBACK LOG]
# Model: claude-opus-4-5 - RATE_LIMITED
# Fallback to: gpt-4-turbo
# Status: SUCCESS
```

### Preguntas para Discusión
1. **¿Necesitamos 3 sistemas o 1 sistema bien diseñado?**
2. **¿Cuál es la fuente de verdad para configuración?**
3. **¿Deberíamos tener decision log para auditoría?**
4. **¿Hay múltiples estrategias (cost vs quality) que queremos soportar?**

### Propuesta de Refactor
```yaml
# Archivo único: config/model-selection.yaml
model_selection:
  strategies:
    cost_optimized:
      rules:
        - task_type: "exploration"
          model: "deepseek-v3"
        - task_type: "bug_triage"
          model: "gpt-4-turbo"

    quality_focused:
      rules:
        - task_type: "code_review"
          model: "claude-opus"
        - task_type: "architecture"
          model: "claude-opus"

  fallback_chain: [claude-opus, gpt-4-turbo, deepseek-v3]
  budget: {daily: 50, per_session: 10}

# Go code: 1 DecisionMaker que lee esto y elige
```

---

## 4. LEARNING LOOP: ¿ESTÁ FUNCIONANDO?

### La Teoría
```
Session 1: Fix bug A → Store solution
Session 2: See similar bug B → Query wisdom → Adapt solution A → Win
```

### La Realidad
```
Pregunta: ¿Tenemos métrics de que esto funcione?
Respuesta: No visibles en código.

Preguntas sin respuesta:
- ¿Cuántos problemas tienen soluciones en vector store?
- ¿De esos, cuántos se encuentran?
- ¿De los encontrados, cuántos se adaptan exitosamente?
- ¿Cuál es el win rate vs re-solving from scratch?
```

### Escenario de Medición
```bash
# Hoy: Sin visibilidad
urp think wisdom "null pointer in struct initialization"
# Silenciosamente retorna top-5 embeddings
# ¿Cuál es relevancia? No se sabe.
# ¿Se usó en siguiente problema? No se sabe.

# Futuro: Con métricas
urp think metrics --learning
# Output:
# Knowledge base: 2500 entries
# Age distribution:
#   - < 1 day: 50 (fresh)
#   - 1-7 days: 200
#   - > 7 days: 2250 (stale?)
#
# Retrieval effectiveness:
#   - Queries with matches > 0.8: 60%
#   - Queries with matches 0.5-0.8: 30%
#   - Queries with no match: 10%
#
# Applied solutions:
#   - Success rate: 75%
#   - Partial success: 20%
#   - Failed adaptation: 5%
#
# ROI: +2.5x faster problem solving
#      (vs re-solving from scratch)
```

### Preguntas para Discusión
1. **¿El learning loop funciona o es cargo-cult?**
2. **¿Debemos agregar métricas o removerlo si no vale?**
3. **¿Es el vector store el mejor storage o debería ser simple rules?**
4. **¿Hay un umbral de similitud que debería ser ajustable?**

---

## 5. TOOL ECOSYSTEM: ¿NECESITAMOS 23 TOOLS?

### Tools Disponibles (23)

```
Core (essential):
  ✅ bash              (execute commands)
  ✅ file_read         (read files)
  ✅ file_write        (write files)
  ✅ file_glob         (find files)
  ✅ file_grep         (search in files)

Nice-to-have:
  ⚠️ browser           (web automation)
  ⚠️ computer          (mouse/keyboard)
  ⚠️ screenshot        (visual capture)
  ⚠️ multi_expert      (route to N LLMs)

Specialized:
  ❓ graph            (cypher queries)
  ❓ codesearch       (semantic search)
  ❓ batch           (parallel operations)
  ❓ patch           (apply patches)
  ❓ lsp_hover        (language server)
  ❓ sandbox         (isolated execution)
  ❓ ... (8 more)
```

### Pregunta Real
```
¿Cuál es el distribution real de uso?

Hipótesis:
- bash: 40-50% (most versatile)
- file_read: 25-30% (reading code)
- file_write: 15-20% (modifying code)
- Others: <5% (rarely used)

Si hipótesis es correcta:
→ 5 tools core, 18 tools overhead
→ Cada tool = maintenance burden + documentation
→ Testing cost increases
```

### Escenario de Auditoría
```bash
# Agregar telemetría
urp audit stats --by-tool --last-100-calls

# Output esperado:
# bash:        45 calls (45%)
# file_read:   30 calls (30%)
# file_write:  15 calls (15%)
# graph:        5 calls (5%)
# others:       5 calls (5%)

# Recomendación:
# If others < 10%: Deprecate them or move to future.
# If others > 30%: Keep, they're valuable.
```

### Preguntas para Discusión
1. **¿Cuál es el actual uso de tools?**
2. **¿Debemos mantener todos o deprecated algunos?**
3. **¿Hay tools que son "nice-to-have" pero expensive?**
4. **¿La complejidad del tool system justifica los beneficios?**

---

## 6. PRU NOTATION: ¿ÚTIL O ACADÉMICO?

### Definición Actual

```
D  = Domain primitives (File, Function, Class)
τ  = Temporal sequences (Commits, Events)
Φ  = Morphism / flow (Calls, Data, Energy)
⊆  = Inclusion hierarchy (File→Func→Method)
⊥  = Orthogonal conflicts (Dead code, Cycles)
P  = Projective views (Interfaces)
T  = Tensor context (Branch, Env, Session)
```

### El Problema

```
Pregunta: ¿Otros developers entienden esto?
Respuesta: Probably not.

Evidencia:
- No hay "PRU handbook"
- No aparece en prompts al agente
- No hay ejemplos claros de uso
- Matemático pero sin justificación formal

Costo:
- Conceptual overhead
- Training burden para new developers
- Maintenance de 7 primitivos en lugar de simple graphs
```

### Escenario de Comparación

**Con PRU**:
```go
// Consulta: "Find all functions called by main()"
query := `
  MATCH (main:Function {name:"main"})
  OPTIONAL MATCH (main)-[:CALLS]->(f:Function)
  RETURN f
`
// En lenguaje PRU: D(main) ⊆ Φ(calls) → D(f)
```

**Sin PRU** (simple):
```go
// Exactamente la misma query
// Pero sin notación matemática confusa
query := `
  MATCH (main:Function {name:"main"})
  OPTIONAL MATCH (main)-[:CALLS]->(f:Function)
  RETURN f
`
```

**Conclusión**: Same result, pero PRU agrega notación sin beneficio práctico.

### Preguntas para Discusión
1. **¿PRU es core a la arquitectura o decorativo?**
2. **¿Debería estar en prompts de agent o solo en docs?**
3. **¿Podemos simplificar a grafo + timestamps sin PRU?**
4. **¿Es worth mantener 7 primitivos o usar standard graph concepts?**

---

## 7. DECISIONES RECOMENDADAS

### Decision Matrix

| Decisión | Impacto | Riesgo | Esfuerzo | Recomendación |
|----------|---------|--------|----------|--------------|
| Simplify Context Compiler | HIGH | LOW | 2 days | ✅ DO |
| Consolidate Model Router | HIGH | LOW | 3 days | ✅ DO |
| Complete Master/Worker | HIGH | MEDIUM | 1 week | ⚠️ MAYBE |
| Add Learning Metrics | MEDIUM | LOW | 2 days | ✅ DO |
| Simplify PRU Notation | LOW | LOW | 1 day | ✅ DO |
| Audit Tool Usage | MEDIUM | LOW | 1 day | ✅ DO |
| Create E2E Tests | MEDIUM | LOW | 3 days | ✅ DO |

### Propuesta de Sprint Priorizado

**Sprint 1 (Week 1)**:
```
Day 1-2: Consolidate Model Router
Day 3: Simplify Context Compiler (first iteration)
Day 4: Add decision logging + audit
Day 5: E2E tests for model selection
```

**Sprint 2 (Week 2)**:
```
Day 1-2: Add Learning Loop metrics
Day 3-4: Complete Master/Worker (if approved)
Day 5: Documentation + PRU handbook
```

**Ongoing**:
```
Weekly: Monitor token budget, model costs
Bi-weekly: Audit tool usage, simplify if needed
Monthly: Refactor based on learnings
```

---

## ANEXO: PREGUNTAS PARA LINUS

(Inspiradas en la guía de CLAUDE.md)

### Pregunta 1: Data Structures
```
"Before adding Context Compiler, did you model:
- What data MUST persist between sessions?
- What is ephemeral?
- What is the minimal representation?

Or did you start with 'let's cache everything'?"
```

### Pregunta 2: Simplicity
```
"Did you consider the principle:
'If you need more than 3 levels of indentation, you're screwed'?

Apply to Model Router:
  Level 1: TaskClassifier
  Level 2: BudgetTracker
  Level 3: ModelSelection
  Level 4: ProviderFactory
  Level 5: ConfigModels

5 levels is too much."
```

### Pregunta 3: Theory vs Practice
```
"PRU notation is theoretical.
Does it make Agent decisions better?
Or is it academic naming without practical value?

If practical: Prove with metrics.
If academic: Move to roadmap or archive."
```

### Pregunta 4: Breaking Changes
```
"Master/Worker architecture suggests:
- Agent role: master vs worker
- Tool availability: restricted in master
- Permission model: explicit checks

But code doesn't enforce this.
Is this intentional (future) or oversight?"
```

---

## PRÓXIMA SESIÓN

**Formato sugerido**:
1. Vote on 3 decisions (15 min)
2. Deep dive on 1-2 topics (30 min)
3. Plan sprint (15 min)

**Preparar**:
- Métricas de uso actual (tools, tokens, models)
- E2E test scenarios
- Comparison: 5-mode vs 1-mode compiler

---

**Documento finalizado**: 2025-12-12
