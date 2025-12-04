# P1: Reto Básico CLI

## Objetivo
Verificar que el CLI funciona correctamente sin conexión a base de datos.

## Pruebas

### 1. Versión
```bash
./urp version
```
**Esperado**: `urp version 0.1.0`

### 2. Help
```bash
./urp --help
```
**Esperado**: Lista de comandos disponibles

### 3. Status sin BD
```bash
./urp
```
**Esperado**: Muestra status, conexión=false

### 4. Sistema Inmune - Comandos seguros
```bash
./urp events run ls -la
./urp events run echo "test"
```
**Esperado**: Ejecuta correctamente

### 5. Runtime detection
```bash
./urp sys runtime
```
**Esperado**: Detecta docker/podman o "No container runtime detected"
