# Documentación del Sistema URP (V2)

## Versión Actual
- **Versión:** 0.1.0
- **Arquitectura:** V2 (Context Compiler Architecture)
- **Fecha de Lanzamiento:** Diciembre 2025

## Arquitectura V2: Context Compiler

### Componentes Principales

#### 1. Context Compiler
El compilador de contexto transforma el estado del grafo en vistas lineales para los LLMs:
- **Entradas:**
  - Estado del grafo (Memgraph)
  - Logs filtrados por el Gate
  - Conocimiento relevante
  - Estrategias aprendidas
- **Salida:** Vista computada optimizada para el LLM

#### 2. Sistema de Puerta (Gate)
- LLM rápido (Qwen/GLM) que filtra ruido
- Elimina logs irrelevantes antes de enviar al LLM principal
- Mejora eficiencia y reduce costos

#### 3. Pipeline Dual-LLM
- **Gate LLM (rápido):** Filtrado de ruido
- **Master LLM (inteligente):** Razonamiento y toma de decisiones

### Memoria Extendida
- **Base de Datos de Grafos (Memgraph):** Relaciones de código, historia de git, soluciones
- **Almacenamiento Vectorial (LanceDB):** Búsqueda semántica sobre código y estrategias aprendidas
- **Estrategias Aprendidas:** Almacenadas al final de cada ciclo para reutilización

### Orquestación Maestro/Trabajador
- **Maestro:** Acceso de solo lectura al proyecto, coordinación
- **Trabajadores:** Acceso de lectura/escritura para ejecución de tareas
- **Aislamiento:** Ejecución segura en contenedores

## Sistema de Aprendizaje Avanzado

### Learning Agent
- Recuerda estrategias exitosas de tareas anteriores
- Recupera tareas similares para mejorar eficacia
- Ajusta comportamiento basado en resultados previos

### Optimización de Contexto
- **5 modos de contexto:**
  - `ModeFull`: Todo el contexto (exploración inicial)
  - `ModeFocused`: Archivo actual + dependencias directas
  - `ModeMinimal`: Solo la función siendo editada
  - `ModeDelta`: Solo cambios + contexto circundante
  - `ModeMemory`: Estado + código minimal (confía en memoria)
- Adaptación automática basada en situación actual
- Control de presupuesto de tokens

### Comprensión de Tareas
- Seguimiento de estado multi-turno
- Métricas de desempeño
- Detección de desviaciones de alcance
- Recuperación de estrategias exitosas

## Interfaz de Usuario Terminal (TUI)

### Características Avanzadas
- Panel de monitoreo cognitivo (Brain Monitor)
- Visualización en tiempo real del estado del agente
- Panel de depuración con eventos de LLM y herramientas
- Navegación estilo Vim
- Sistema de comandos slash (/help, /clear, etc.)

### Controles Principales
- `Enter`: Enviar mensaje
- `Ctrl+H`: Ayuda
- `Ctrl+D`: Panel de depuración
- `Ctrl+S`: Búsqueda en salida
- `Ctrl+T`: Expandir/contraer herramientas
- `Ctrl+A`: Adjuntar archivo
- `Ctrl+N`: Cambiar agente

## Configuración de Modelos

### Variables de Configuración
- `URP_MASTER_MODEL_ID`: Modelo principal de razonamiento
- `URP_GATE_MODEL_ID`: Modelo para filtrado de ruido
- `URP_WORKER_MODEL_ID`: Modelo para tareas de ejecución
- `URP_CODING_MODEL_ID`: Modelo optimizado para tareas de codificación
- `URP_REASONING_MODEL_ID`: Modelo para razonamiento complejo
- `URP_FAST_MODEL_ID`: Modelo rápido para respuestas rápidas
- `URP_VISION_MODEL_ID`: Modelo con capacidades de visión
- `URP_LONG_CONTEXT_MODEL_ID`: Modelo para tareas con contexto largo

### Sistema de Fallback
- Configuración jerárquica de modelos
- Mecanismos de fallback automáticos
- URLs y claves API personalizadas por modelo

## Comandos Principales

### Infraestructura
- `urp infra start/stop`: Gestión de infraestructura
- `urp launch`: Lanzar contenedor maestro
- `urp spawn`: Generar trabajadores
- `urp doctor`: Verificación de salud

### Análisis
- `urp code ingest/deps/impact`: Análisis de código
- `urp git ingest/history`: Análisis de historia git
- `urp focus <target>`: Carga de contexto enfocado

### Cognitivo
- `urp think wisdom/novelty/learn`: Habilidades cognitivas
- `urp mem add/recall`: Memoria de sesión
- `urp kb store/query`: Base de conocimiento
- `urp models list/resolve`: Gestión de modelos

### Interfaz
- `urp tui`: Interfaz TUI avanzada
- `urp serve`: Servidor HTTP para GUI

## Configuración por Defecto
- Todos los modelos apuntan al proxy: `http://tizz.win:8317/v1`
- Modelo por defecto: `zai-glm-4.6`
- Clave API por defecto: `sk-urp-proxy-key`

## Requisitos del Sistema
- Docker (requerido)
- Memgraph (base de datos de grafos)
- Go 1.24+
- Acceso a API de LLMs (o proxy local)

## Casos de Uso Recomendados
- Desarrollo asistido por IA con memoria persistente
- Análisis y refactorización de codebases complejas
- Ejecución segura de tareas de IA en código
- Aprendizaje adaptativo de procesos de desarrollo