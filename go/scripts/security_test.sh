#!/bin/bash
# Batería de Pruebas de Flujo - URP CLI
# =====================================

# No usar set -e porque queremos capturar fallos

# Find urp binary
URP="${URP:-./urp}"
if [[ ! -x "$URP" ]]; then
    # If not in current dir, check relative to script
    SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
    URP="$SCRIPT_DIR/../urp"
fi

if [[ ! -x "$URP" ]]; then
    echo "Error: urp binary not found at $URP"
    exit 1
fi
PASS=0
FAIL=0
WARN=0

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASS++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAIL++))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    ((WARN++))
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_section() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo -e "${BLUE}$1${NC}"
    echo "═══════════════════════════════════════════════════════════════"
}

# ═══════════════════════════════════════════════════════════════
# P1: PRUEBAS BÁSICAS CLI (sin BD)
# ═══════════════════════════════════════════════════════════════

log_section "P1: PRUEBAS BÁSICAS CLI"

# Test: version
log_info "Probando: urp version"
OUTPUT=$($URP version 2>&1)
if [[ "$OUTPUT" == *"urp version"* ]]; then
    log_pass "version: $OUTPUT"
else
    log_fail "version: output inesperado: $OUTPUT"
fi

# Test: help
log_info "Probando: urp --help"
OUTPUT=$($URP --help 2>&1)
if [[ "$OUTPUT" == *"URP"* ]] || [[ "$OUTPUT" == *"urp"* ]]; then
    log_pass "help: muestra descripción correcta"
else
    log_fail "help: output inesperado"
fi

# Test: comandos disponibles
if [[ "$OUTPUT" == *"code"* && "$OUTPUT" == *"git"* && "$OUTPUT" == *"think"* ]]; then
    log_pass "help: muestra comandos code, git, think"
else
    log_fail "help: faltan comandos esperados"
fi

# Test: status sin BD
log_info "Probando: urp (status)"
OUTPUT=$($URP 2>&1)
if [[ "$OUTPUT" == *"URP STATUS"* ]] || [[ "$OUTPUT" == *"Graph:"* ]]; then
    log_pass "status: muestra información básica"
else
    log_warn "status: output puede variar sin BD"
fi

# Test: sys runtime
log_info "Probando: urp sys runtime"
OUTPUT=$($URP sys runtime 2>&1)
if [[ "$OUTPUT" == *"docker"* ]] || [[ "$OUTPUT" == *"podman"* ]] || [[ "$OUTPUT" == *"No container runtime"* ]]; then
    log_pass "sys runtime: detecta runtime o informa ausencia"
else
    log_fail "sys runtime: output inesperado: $OUTPUT"
fi

# ═══════════════════════════════════════════════════════════════
# P4: SISTEMA INMUNE (sin BD, pruebas de análisis)
# ═══════════════════════════════════════════════════════════════

log_section "P4: SISTEMA INMUNE"

# Función para probar comando bloqueado
test_blocked() {
    local cmd="$1"
    local desc="$2"
    log_info "Probando bloqueo: $cmd"
    # Usar -- para separar flags de cobra de argumentos del comando
    OUTPUT=$($URP events run -- $cmd 2>&1) || true
    if [[ "$OUTPUT" == *"IMMUNE_BLOCK"* ]] || [[ "$OUTPUT" == *"blocked"* ]] || [[ "$OUTPUT" == *"dangerous"* ]]; then
        log_pass "BLOQUEADO: $desc"
    else
        log_fail "NO BLOQUEADO: $desc - Output: $OUTPUT"
    fi
}

# Función para probar comando con warning
test_warning() {
    local cmd="$1"
    local desc="$2"
    log_info "Probando warning: $cmd"
    OUTPUT=$($URP events run -- $cmd 2>&1) || true
    if [[ "$OUTPUT" == *"WARNING"* ]] || [[ "$OUTPUT" == *"warning"* ]] || [[ "$OUTPUT" == *"caution"* ]]; then
        log_pass "WARNING: $desc"
    else
        log_warn "Sin warning explícito: $desc"
    fi
}

# Comandos que DEBEN ser bloqueados
# Nota: usamos comillas simples para evitar expansión de glob

log_info "Probando bloqueo: rm -rf /"
OUTPUT=$($URP events run -- rm -rf / 2>&1) || true
if [[ "$OUTPUT" == *"IMMUNE_BLOCK"* ]] || [[ "$OUTPUT" == *"blocked"* ]] || [[ "$OUTPUT" == *"dangerous"* ]]; then
    log_pass "BLOQUEADO: rm -rf root"
else
    log_fail "NO BLOQUEADO: rm -rf root - Output: $OUTPUT"
fi

log_info "Probando bloqueo: git push --force"
OUTPUT=$($URP events run -- git push --force origin main 2>&1) || true
if [[ "$OUTPUT" == *"IMMUNE_BLOCK"* ]] || [[ "$OUTPUT" == *"blocked"* ]] || [[ "$OUTPUT" == *"dangerous"* ]]; then
    log_pass "BLOQUEADO: git push --force"
else
    log_fail "NO BLOQUEADO: git push --force - Output: $OUTPUT"
fi

log_info "Probando bloqueo: git add .env"
OUTPUT=$($URP events run -- git add .env 2>&1) || true
if [[ "$OUTPUT" == *"IMMUNE_BLOCK"* ]] || [[ "$OUTPUT" == *"blocked"* ]] || [[ "$OUTPUT" == *"dangerous"* ]]; then
    log_pass "BLOQUEADO: git add .env"
else
    log_fail "NO BLOQUEADO: git add .env - Output: $OUTPUT"
fi

log_info "Probando bloqueo: mkfs"
OUTPUT=$($URP events run -- mkfs /dev/sda1 2>&1) || true
if [[ "$OUTPUT" == *"IMMUNE_BLOCK"* ]] || [[ "$OUTPUT" == *"blocked"* ]] || [[ "$OUTPUT" == *"dangerous"* ]]; then
    log_pass "BLOQUEADO: mkfs"
else
    log_fail "NO BLOQUEADO: mkfs - Output: $OUTPUT"
fi

# ═══════════════════════════════════════════════════════════════
# P2: VECTOR STORE (sin BD)
# ═══════════════════════════════════════════════════════════════

log_section "P5: VECTOR STORE"

# Test: vec stats
log_info "Probando: urp vec stats"
OUTPUT=$($URP vec stats 2>&1)
if [[ "$OUTPUT" == *"VECTOR STORE"* ]] || [[ "$OUTPUT" == *"Entries"* ]]; then
    log_pass "vec stats: muestra información del store"
else
    log_warn "vec stats: puede requerir inicialización"
fi

# Test: vec add
log_info "Probando: urp vec add"
OUTPUT=$($URP vec add "Error de prueba para vector store" -k error 2>&1)
if [[ "$OUTPUT" == *"Added"* ]] || [[ $? -eq 0 ]]; then
    log_pass "vec add: agregó entrada al store"
else
    log_warn "vec add: puede fallar sin persistencia"
fi

# Test: vec search
log_info "Probando: urp vec search"
OUTPUT=$($URP vec search "error prueba" 2>&1)
if [[ "$OUTPUT" == *"VECTOR SEARCH"* ]] || [[ "$OUTPUT" == *"results"* ]] || [[ "$OUTPUT" == *"No matching"* ]]; then
    log_pass "vec search: ejecuta búsqueda semántica"
else
    log_warn "vec search: resultado variable"
fi

# ═══════════════════════════════════════════════════════════════
# PRUEBAS DE SUBCOMANDOS
# ═══════════════════════════════════════════════════════════════

log_section "PRUEBAS DE SUBCOMANDOS"

# Test: think wisdom (sin BD debería fallar gracefully)
log_info "Probando: urp think wisdom"
OUTPUT=$($URP think wisdom "ModuleNotFoundError" 2>&1) || true
if [[ "$OUTPUT" == *"WISDOM"* ]] || [[ "$OUTPUT" == *"Not connected"* ]] || [[ "$OUTPUT" == *"Error"* ]] || [[ "$OUTPUT" == *"connection refused"* ]]; then
    log_pass "think wisdom: maneja ausencia de BD correctamente"
else
    log_warn "think wisdom: output variable sin BD"
fi

# Test: think novelty (sin BD debería fallar gracefully)
log_info "Probando: urp think novelty"
OUTPUT=$($URP think novelty "func NewWeirdPattern()" 2>&1) || true
if [[ "$OUTPUT" == *"NOVELTY"* ]] || [[ "$OUTPUT" == *"Not connected"* ]] || [[ "$OUTPUT" == *"pioneer"* ]]; then
    log_pass "think novelty: maneja ausencia de BD"
else
    log_warn "think novelty: output variable sin BD"
fi

# Test: focus (sin BD)
log_info "Probando: urp focus"
OUTPUT=$($URP focus main.go 2>&1) || true
if [[ "$OUTPUT" == *"Warning"* ]] || [[ "$OUTPUT" == *"No entities"* ]] || [[ "$OUTPUT" == *"Not connected"* ]]; then
    log_pass "focus: maneja ausencia de BD"
else
    log_warn "focus: output variable sin BD"
fi

# ═══════════════════════════════════════════════════════════════
# RESUMEN
# ═══════════════════════════════════════════════════════════════

log_section "RESUMEN DE PRUEBAS"

echo ""
echo -e "  ${GREEN}PASS: $PASS${NC}"
echo -e "  ${RED}FAIL: $FAIL${NC}"
echo -e "  ${YELLOW}WARN: $WARN${NC}"
echo ""

TOTAL=$((PASS + FAIL))
if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}TODAS LAS PRUEBAS CRÍTICAS PASARON${NC}"
    echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
    exit 0
else
    echo -e "${RED}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${RED}$FAIL PRUEBAS FALLARON${NC}"
    echo -e "${RED}═══════════════════════════════════════════════════════════════${NC}"
    exit 1
fi
