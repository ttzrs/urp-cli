# P2: Reto Análisis de Código

## Objetivo
Verificar que el análisis de código funciona correctamente.

## Código de Prueba
`main.go` contiene:
- 3 funciones: GetUser, ValidateUser, ProcessUser
- 1 función muerta: unusedFunction
- Llamadas: ProcessUser → GetUser, ProcessUser → ValidateUser

## Pruebas (requiere Memgraph)

### 1. Ingerir código
```bash
./urp code ingest ./P2
```
**Esperado**: Stats con files=1, functions>=4

### 2. Encontrar dependencias
```bash
./urp code deps ProcessUser
```
**Esperado**: GetUser, ValidateUser como dependencias

### 3. Encontrar impacto
```bash
./urp code impact GetUser
```
**Esperado**: ProcessUser como afectado

### 4. Detectar código muerto
```bash
./urp code dead
```
**Esperado**: unusedFunction en la lista

### 5. Estadísticas
```bash
./urp code stats
```
**Esperado**: Contadores de nodos y relaciones
