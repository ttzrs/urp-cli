# PCx - Performance Comparison eXperiment

Framework de pruebas para validar el sistema de optimización de contexto.

## Arquitectura

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PCx FRAMEWORK                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        EXPERIMENT RUNNER                              │   │
│  │  pcx_runner.py                                                        │   │
│  │  - Spawns 4 containers (none/semi/auto/hybrid)                       │   │
│  │  - Executes identical workload in each                               │   │
│  │  - Collects metrics: tokens, time, errors, quality                   │   │
│  │  - Syncs to Memgraph                                                 │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                              │                                               │
│       ┌──────────────────────┼──────────────────────┐                       │
│       ▼                      ▼                      ▼                       │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐                 │
│  │  WORKLOAD   │      │  WORKLOAD   │      │  WORKLOAD   │                 │
│  │   SIMPLE    │      │   MEDIUM    │      │   COMPLEX   │                 │
│  │ ─────────── │      │ ─────────── │      │ ─────────── │                 │
│  │ 5 files     │      │ 20 files    │      │ 100 files   │                 │
│  │ 1 error     │      │ 5 errors    │      │ 20 errors   │                 │
│  │ 10 commands │      │ 50 commands │      │ 200 commands│                 │
│  └─────────────┘      └─────────────┘      └─────────────┘                 │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        METRICS COLLECTOR                              │   │
│  │  - tokens_consumed: Total tokens used                                │   │
│  │  - tokens_saved: Tokens freed by optimization                        │   │
│  │  - context_hits: Times context was useful                            │   │
│  │  - context_misses: Times needed context was evicted                  │   │
│  │  - execution_time_ms: Total task time                                │   │
│  │  - error_recovery_rate: Errors resolved vs total                     │   │
│  │  - code_quality_score: Linter + test pass rate                       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                              │                                               │
│                              ▼                                               │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                         MEMGRAPH STORAGE                              │   │
│  │  (:PCxExperiment)-[:TESTED]->(:PCxResult)                            │   │
│  │  (:PCxResult)-[:MEASURED]->(:PCxMetric)                              │   │
│  │  (:PCxResult)-[:COMPARED_TO]->(:PCxResult)                           │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Workloads

### 1. Simple (Baseline)
- Crear 5 archivos Python básicos
- Introducir 1 error de sintaxis
- Ejecutar 10 comandos git/test

### 2. Medium (Realistic)
- Refactorizar módulo de 20 archivos
- Resolver 5 errores encadenados
- Ciclo completo: edit → test → fix → commit

### 3. Complex (Stress Test)
- Proyecto multi-módulo (100 archivos)
- 20 errores interdependientes
- Múltiples rondas de debugging
- Context window exhaustion forzado

## Métricas Clave

| Métrica | Descripción | Objetivo |
|---------|-------------|----------|
| `efficiency_ratio` | tokens_saved / tokens_consumed | > 0.3 |
| `context_retention` | context_hits / (hits + misses) | > 0.8 |
| `error_recovery` | errors_fixed / errors_total | > 0.9 |
| `time_overhead` | time_with_opt / time_without | < 1.1 |

## Uso

```bash
# Ejecutar experimento simple
pcx run simple

# Ejecutar todos los workloads
pcx run all

# Ver resultados
pcx results

# Comparar modos
pcx compare

# Exportar a CSV
pcx export
```
