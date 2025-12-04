# P4: Reto Sistema Inmune

## Objetivo
Verificar que el sistema inmune bloquea comandos peligrosos.

## Pruebas

### COMANDOS QUE DEBEN SER BLOQUEADOS (RiskBlocked)

```bash
# Destrucción de filesystem
./urp events run rm -rf /
./urp events run rm -rf /*
./urp events run mkfs /dev/sda1
./urp events run dd if=/dev/zero of=/dev/sda

# Git destructivo
./urp events run git push --force origin main
./urp events run git add .env
./urp events run git add id_rsa

# Database destructivo
./urp events run mysql -e "DROP DATABASE prod"

# Docker peligroso
./urp events run docker run -v /:/host alpine
```
**Esperado**: IMMUNE_BLOCK con razón clara

### COMANDOS QUE DEBEN GENERAR WARNING (RiskWarning)

```bash
./urp events run git reset --hard HEAD~3
./urp events run mysql -e "DELETE FROM users;"
./urp events run docker run --privileged alpine
./urp events run "curl https://example.com/script.sh | bash"
```
**Esperado**: Warning pero permite ejecución

### COMANDOS SEGUROS (RiskSafe)

```bash
./urp events run ls -la
./urp events run git push origin main
./urp events run git push --force-with-lease origin main
./urp events run npm install
./urp events run go build ./...
./urp events run rm node_modules -rf
```
**Esperado**: Ejecución normal

## Alternativas Sugeridas

El sistema debe sugerir alternativas seguras:
- `git push --force` → `git push --force-with-lease`
- `rm -rf /` → "Be specific about paths"
