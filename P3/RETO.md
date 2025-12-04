# P3: Reto Memoria y Conocimiento

## Objetivo
Verificar el sistema de memoria de sesión y base de conocimiento.

## Pruebas (requiere Memgraph)

### 1. Agregar memoria
```bash
./urp mem add "El puerto 8080 está ocupado por nginx"
./urp mem add "SELinux requiere contexto correcto para docker.sock" -k observation
./urp mem add "Usar --force-with-lease en vez de --force" -k decision -i 5
```
**Esperado**: IDs de memoria devueltos

### 2. Listar memorias
```bash
./urp mem list
```
**Esperado**: Las 3 memorias agregadas

### 3. Buscar en memorias
```bash
./urp mem recall "puerto ocupado"
./urp mem recall "SELinux"
```
**Esperado**: Resultados relevantes con score > 0

### 4. Estadísticas de memoria
```bash
./urp mem stats
```
**Esperado**: total=3, by_kind con conteo

### 5. Almacenar conocimiento
```bash
./urp kb store "ModuleNotFoundError se resuelve con pip install" -k fix
./urp kb store "Siempre verificar permisos antes de docker run" -k rule
```
**Esperado**: IDs de conocimiento

### 6. Buscar conocimiento
```bash
./urp kb query "error módulo no encontrado"
```
**Esperado**: Match con el fix almacenado

### 7. Promover conocimiento
```bash
./urp kb promote <id-del-fix>
```
**Esperado**: Promovido a scope=global

### 8. Limpiar sesión
```bash
./urp mem clear
```
**Esperado**: Memorias eliminadas
