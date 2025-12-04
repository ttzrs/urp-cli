#!/usr/bin/env python3
"""
Tutorial Interactivo URP-CLI

Guía paso a paso para aprender el sistema de optimización de contexto.
"""
import os
import sys
import time
import subprocess
from typing import Optional

# Colors
GREEN = '\033[92m'
YELLOW = '\033[93m'
RED = '\033[91m'
CYAN = '\033[96m'
BOLD = '\033[1m'
RESET = '\033[0m'

# Progress tracking
PROGRESS_FILE = os.path.expanduser("~/.urp_tutorial_progress")


def clear_screen():
    os.system('clear' if os.name != 'nt' else 'cls')


def print_header(title: str):
    print(f"\n{CYAN}{'═' * 60}{RESET}")
    print(f"{BOLD}{title.center(60)}{RESET}")
    print(f"{CYAN}{'═' * 60}{RESET}\n")


def print_step(num: int, text: str):
    print(f"{GREEN}[{num}]{RESET} {text}")


def print_info(text: str):
    print(f"{CYAN}ℹ{RESET}  {text}")


def print_warning(text: str):
    print(f"{YELLOW}⚠{RESET}  {text}")


def print_success(text: str):
    print(f"{GREEN}✓{RESET}  {text}")


def print_error(text: str):
    print(f"{RED}✗{RESET}  {text}")


def print_code(code: str):
    print(f"\n{YELLOW}  $ {code}{RESET}\n")


def wait_for_enter(prompt: str = "Presiona ENTER para continuar..."):
    input(f"\n{CYAN}→{RESET} {prompt}")


def run_command(cmd: str, show_output: bool = True) -> tuple:
    """Run command and return (exit_code, output)."""
    print_code(cmd)
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=True, text=True, timeout=30
        )
        if show_output and result.stdout:
            print(result.stdout)
        if show_output and result.stderr:
            print(f"{RED}{result.stderr}{RESET}")
        return result.returncode, result.stdout
    except subprocess.TimeoutExpired:
        print_error("Comando timeout")
        return 1, ""
    except Exception as e:
        print_error(f"Error: {e}")
        return 1, ""


def ask_question(question: str, options: list) -> int:
    """Ask multiple choice question, return selected index."""
    print(f"\n{BOLD}{question}{RESET}\n")
    for i, opt in enumerate(options, 1):
        print(f"  {i}. {opt}")

    while True:
        try:
            choice = input(f"\n{CYAN}Selecciona (1-{len(options)}): {RESET}")
            idx = int(choice) - 1
            if 0 <= idx < len(options):
                return idx
        except ValueError:
            pass
        print_error("Opción inválida")


def save_progress(level: int, step: int):
    """Save tutorial progress."""
    with open(PROGRESS_FILE, 'w') as f:
        f.write(f"{level},{step}")


def load_progress() -> tuple:
    """Load tutorial progress."""
    try:
        with open(PROGRESS_FILE, 'r') as f:
            level, step = f.read().strip().split(',')
            return int(level), int(step)
    except:
        return 1, 0


# ═══════════════════════════════════════════════════════════════════════════════
# NIVEL 1: FUNDAMENTOS
# ═══════════════════════════════════════════════════════════════════════════════

def level1_intro():
    clear_screen()
    print_header("NIVEL 1: FUNDAMENTOS")

    print("""
    En este nivel aprenderás:

    • Verificar el estado del sistema
    • Usar comandos de percepción (pain, recent, vitals)
    • Consultar el grafo de conocimiento

    Tiempo estimado: 15 minutos
    """)

    wait_for_enter()


def level1_step1():
    """Verificar estado del sistema."""
    clear_screen()
    print_header("1.1 Verificar Estado del Sistema")

    print_step(1, "Primero, verifica que URP está activo:")
    run_command("python3 -c \"from context_manager import get_current_mode; print(f'URP activo - Modo: {get_current_mode()}')\"")

    print_info("Deberías ver el modo actual de optimización (hybrid por defecto).")

    wait_for_enter()

    print_step(2, "Ahora veamos el estado de tokens:")
    run_command("python3 -c \"from token_tracker import get_remaining; import json; print(json.dumps(get_remaining(), indent=2))\"")

    print_info("Esto muestra tu presupuesto de tokens y uso actual.")

    wait_for_enter()
    save_progress(1, 1)


def level1_step2():
    """Percepción del sistema."""
    clear_screen()
    print_header("1.2 Percepción: Sentir el Sistema")

    print("""
    URP te da "sentidos" para percibir qué está pasando:

    • pain     → Ver errores recientes
    • recent   → Ver comandos ejecutados
    • vitals   → Ver recursos (CPU/RAM)
    """)

    wait_for_enter()

    print_step(1, "Vamos a provocar un error y luego verlo con 'pain':")
    print_code("python3 -c 'import nonexistent'  # Esto fallará")

    subprocess.run(
        "python3 -c 'import nonexistent' 2>/dev/null",
        shell=True, capture_output=True
    )

    print_success("Error provocado. Ahora veamos el dolor:")

    # Simulate pain output
    print(f"""
{RED}[✗] LATEST: python3 -c 'import nonexistent'
    Error: ModuleNotFoundError: No module named 'nonexistent'{RESET}
    """)

    print_info("En el contenedor real, 'pain' mostraría todos los errores recientes.")

    wait_for_enter()
    save_progress(1, 2)


def level1_step3():
    """Consultas al grafo."""
    clear_screen()
    print_header("1.3 Consultas al Grafo de Conocimiento")

    print("""
    El grafo almacena relaciones entre tu código:

    • urp impact <sig>  → ¿Qué se rompe si cambio esto?
    • urp deps <sig>    → ¿De qué depende esto?
    • urp history <f>   → Historia de cambios
    • urp hotspots      → Archivos más modificados
    • urp dead          → Código muerto
    """)

    print_step(1, "Ejemplo: Ver historia de un archivo")
    print_code("urp history context_manager.py")

    print(f"""
{CYAN}Historial de context_manager.py:{RESET}
  2025-12-02 14:00  [+600 -0]  Add mode_none() function
  2025-12-02 12:30  [+200 -50] Implement LRU eviction
  2025-12-01 16:00  [+500 -0]  Initial creation
    """)

    print_info("Esto ayuda a entender la evolución del código.")

    wait_for_enter()
    save_progress(1, 3)


def level1_quiz():
    """Quiz nivel 1."""
    clear_screen()
    print_header("QUIZ NIVEL 1")

    score = 0

    # Question 1
    q1 = ask_question(
        "¿Qué comando muestra los errores recientes?",
        ["recent", "pain", "vitals", "history"]
    )
    if q1 == 1:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. La respuesta es 'pain'.")

    # Question 2
    q2 = ask_question(
        "¿Qué muestra 'urp hotspots'?",
        ["Errores frecuentes", "Archivos más modificados", "Uso de CPU", "Tokens consumidos"]
    )
    if q2 == 1:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. Muestra archivos más modificados (alto riesgo).")

    # Question 3
    q3 = ask_question(
        "¿Cuál es el modo de optimización por defecto?",
        ["none", "semi", "auto", "hybrid"]
    )
    if q3 == 3:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. El modo por defecto es 'hybrid'.")

    print(f"\n{BOLD}Resultado: {score}/3{RESET}")

    if score >= 2:
        print_success("¡Nivel 1 completado! Puedes avanzar al Nivel 2.")
        save_progress(2, 0)
        return True
    else:
        print_warning("Repasa los conceptos antes de continuar.")
        return False


# ═══════════════════════════════════════════════════════════════════════════════
# NIVEL 2: OPTIMIZACIÓN
# ═══════════════════════════════════════════════════════════════════════════════

def level2_intro():
    clear_screen()
    print_header("NIVEL 2: OPTIMIZACIÓN DE CONTEXTO")

    print("""
    En este nivel aprenderás:

    • Entender la economía de tokens
    • Usar los comandos cc-* para optimización
    • Cambiar entre modos de optimización
    • Gestionar la working memory con focus/unfocus

    Tiempo estimado: 30 minutos
    """)

    wait_for_enter()


def level2_step1():
    """Token economy."""
    clear_screen()
    print_header("2.1 Entender Token Economy")

    print("""
    Tu contexto tiene un presupuesto de ~50,000 tokens/hora.

    Cada cosa que "recuerdas" consume tokens:
    • Archivos leídos
    • Comandos ejecutados
    • Errores encontrados
    • Contexto de focus

    La optimización ayuda a usar esos tokens eficientemente.
    """)

    print_step(1, "Ver estado de tokens:")
    run_command("python3 -c \"from token_tracker import get_remaining; r = get_remaining(); print(f'Usado: {r[\\\"used\\\"]:,} / {r[\\\"budget\\\"]:,} ({r[\\\"usage_pct\\\"]}%)')\"")

    wait_for_enter()
    save_progress(2, 1)


def level2_step2():
    """cc-* commands."""
    clear_screen()
    print_header("2.2 Comandos de Optimización (cc-*)")

    print("""
    Los comandos cc-* controlan la optimización:

    cc-status   → Ver estado actual
    cc-noise    → Detectar tokens "ruidosos"
    cc-compact  → Ejecutar optimización
    cc-clean    → Limpiar working memory
    cc-mode     → Ver/cambiar modo
    """)

    print_step(1, "Ver estado de optimización:")
    run_command("python3 context_manager.py status")

    wait_for_enter()

    print_step(2, "Detectar ruido:")
    run_command("python3 context_manager.py detect-noise")

    print_info("""
Tipos de ruido:
  • old_items: Contexto > 30 min sin acceso
  • unused: Items con bajo access_count
  • duplicate_basenames: Múltiples archivos con mismo nombre
    """)

    wait_for_enter()
    save_progress(2, 2)


def level2_step3():
    """Modos de optimización."""
    clear_screen()
    print_header("2.3 Modos de Optimización")

    print(f"""
    {BOLD}4 MODOS DISPONIBLES:{RESET}

    ┌────────┬────────────┬───────────┬──────────────────────────┐
    │ Modo   │ Ahorro     │ Retención │ Cuándo usar              │
    ├────────┼────────────┼───────────┼──────────────────────────┤
    │ none   │ 0%         │ 50%       │ Testing A/B              │
    │ semi   │ 10%        │ 63%       │ Debug crítico            │
    │ auto   │ 40%        │ 51%       │ Sesiones largas          │
    │ {GREEN}hybrid{RESET} │ {GREEN}30%{RESET}        │ {GREEN}63%{RESET}       │ {GREEN}Uso diario (recomendado){RESET} │
    └────────┴────────────┴───────────┴──────────────────────────┘
    """)

    print_step(1, "Ver modo actual:")
    run_command("python3 context_manager.py mode")

    print_step(2, "Cambiar modo (ejemplo):")
    print_code("cc-auto    # Modo agresivo")
    print_code("cc-smart   # Volver a hybrid")

    print_info("El modo persiste entre sesiones.")

    wait_for_enter()
    save_progress(2, 3)


def level2_step4():
    """Working memory."""
    clear_screen()
    print_header("2.4 Working Memory (Focus)")

    print("""
    Puedes controlar qué está en tu "memoria de trabajo":

    focus <target>     → Cargar contexto específico
    unfocus <target>   → Quitar de memoria
    clear-context      → Limpiar todo
    """)

    print_step(1, "Ejemplo de focus:")
    print_code("focus runner.py --depth 2")

    print(f"""
{CYAN}Cargando contexto:{RESET}
  runner.py
  ├── _calculate_eviction_score()
  ├── add_to_focus()
  └── get_accumulated_context()

  Dependencias (depth 2):
  ├── context_manager.py
  └── brain_render.py
    """)

    print_info("--depth controla cuántos niveles de dependencias cargar.")

    wait_for_enter()
    save_progress(2, 4)


def level2_quiz():
    """Quiz nivel 2."""
    clear_screen()
    print_header("QUIZ NIVEL 2")

    score = 0

    q1 = ask_question(
        "¿Qué modo tiene mejor balance entre ahorro y retención?",
        ["none", "semi", "auto", "hybrid"]
    )
    if q1 == 3:
        print_success("¡Correcto! Hybrid es el balance óptimo.")
        score += 1
    else:
        print_error("Incorrecto. Hybrid tiene 30% ahorro + 63% retención.")

    q2 = ask_question(
        "¿Qué comando detecta tokens que no aportan valor?",
        ["cc-status", "cc-noise", "cc-compact", "cc-clean"]
    )
    if q2 == 1:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. 'cc-noise' detecta patrones de ruido.")

    q3 = ask_question(
        "¿Qué hace 'focus --depth 2'?",
        [
            "Carga 2 archivos",
            "Carga target + dependencias directas",
            "Busca en 2 directorios",
            "Espera 2 segundos"
        ]
    )
    if q3 == 1:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. --depth 2 carga target + dependencias directas.")

    print(f"\n{BOLD}Resultado: {score}/3{RESET}")

    if score >= 2:
        print_success("¡Nivel 2 completado! Puedes avanzar al Nivel 3.")
        save_progress(3, 0)
        return True
    else:
        print_warning("Repasa los conceptos antes de continuar.")
        return False


# ═══════════════════════════════════════════════════════════════════════════════
# NIVEL 3: A/B TESTING Y PCx
# ═══════════════════════════════════════════════════════════════════════════════

def level3_intro():
    clear_screen()
    print_header("NIVEL 3: A/B TESTING Y PCx")

    print("""
    En este nivel aprenderás:

    • Registrar métricas de calidad
    • Ejecutar tests A/B con múltiples contenedores
    • Usar PCx para experimentos de rendimiento
    • Analizar resultados y optimizar

    Tiempo estimado: 45 minutos
    """)

    wait_for_enter()


def level3_step1():
    """Métricas."""
    clear_screen()
    print_header("3.1 Sistema de Métricas")

    print("""
    El sistema aprende de tu feedback:

    cc-quality N   → Registrar satisfacción (1-5)
    cc-error       → Reportar pérdida de contexto
    cc-stats       → Ver estadísticas por modo
    cc-recommend   → Obtener recomendación
    """)

    print_step(1, "Ver estadísticas actuales:")
    run_command("python3 context_manager.py stats")

    print_step(2, "Obtener recomendación:")
    run_command("python3 context_manager.py recommend")

    print_info("Más datos = mejor recomendación. Usa cc-quality regularmente.")

    wait_for_enter()
    save_progress(3, 1)


def level3_step2():
    """A/B Testing."""
    clear_screen()
    print_header("3.2 A/B Testing con ab-*")

    print("""
    A/B testing ejecuta el mismo trabajo en 4 contenedores paralelos,
    cada uno con un modo diferente:

    ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
    │  none   │  │  semi   │  │  auto   │  │ hybrid  │
    │ branch: │  │ branch: │  │ branch: │  │ branch: │
    │ab/*/none│  │ab/*/semi│  │ab/*/auto│  │ab/*/hyb │
    └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
         │            │            │            │
         └────────────┴────────────┴────────────┘
                          │
                    Comparar métricas
    """)

    print_step(1, "Comando de ejemplo:")
    print_code('ab-test "Refactorizar módulo" "pytest tests/"')

    print_info("""
Esto:
  1. Crea 4 contenedores
  2. Crea 4 ramas git
  3. Ejecuta pytest en cada uno
  4. Compara resultados
  5. Guarda en Memgraph
    """)

    wait_for_enter()
    save_progress(3, 2)


def level3_step3():
    """PCx."""
    clear_screen()
    print_header("3.3 PCx - Performance Comparison eXperiment")

    print("""
    PCx genera workloads sintéticos para testing:

    ┌─────────────┬─────────────┬─────────────┐
    │   SIMPLE    │   MEDIUM    │   COMPLEX   │
    ├─────────────┼─────────────┼─────────────┤
    │ 6 archivos  │ 20 archivos │ 100 archivos│
    │ 1 error     │ 5 errores   │ 20 errores  │
    │ 5 fases     │ 10 fases    │ 16 fases    │
    └─────────────┴─────────────┴─────────────┘
    """)

    print_step(1, "Ejecutar experimento simple:")
    run_command("python3 pcx/pcx_runner.py run simple 2>&1 | tail -20")

    print_step(2, "Comparar resultados:")
    run_command("python3 pcx/pcx_runner.py compare")

    wait_for_enter()
    save_progress(3, 3)


def level3_step4():
    """Análisis avanzado."""
    clear_screen()
    print_header("3.4 Análisis de Resultados")

    print("""
    Interpreta los resultados:

    Efficiency:  tokens_saved / tokens_consumed
                 → Más alto = mejor ahorro

    Retention:   context_hits / total_queries
                 → Más alto = preserva contexto útil

    Recovery:    errors_resolved / errors_total
                 → Más alto = resuelve más problemas

    El mejor modo equilibra las 3 métricas.
    """)

    print_step(1, "Exportar datos para análisis:")
    run_command("python3 pcx/pcx_runner.py export -o /tmp/pcx_analysis.csv && head -5 /tmp/pcx_analysis.csv")

    print_info("Puedes analizar los CSV en pandas, Excel, o cualquier herramienta.")

    wait_for_enter()
    save_progress(3, 4)


def level3_quiz():
    """Quiz nivel 3."""
    clear_screen()
    print_header("QUIZ NIVEL 3")

    score = 0

    q1 = ask_question(
        "¿Qué mide 'efficiency_ratio'?",
        [
            "Tiempo de ejecución",
            "tokens_saved / tokens_consumed",
            "Errores por minuto",
            "Archivos modificados"
        ]
    )
    if q1 == 1:
        print_success("¡Correcto!")
        score += 1
    else:
        print_error("Incorrecto. Es tokens_saved / tokens_consumed.")

    q2 = ask_question(
        "¿Cuántos contenedores crea ab-test?",
        ["1", "2", "3", "4"]
    )
    if q2 == 3:
        print_success("¡Correcto! Uno por cada modo.")
        score += 1
    else:
        print_error("Incorrecto. Crea 4 (none, semi, auto, hybrid).")

    q3 = ask_question(
        "¿Qué workload de PCx es mejor para stress testing?",
        ["simple", "medium", "complex", "all"]
    )
    if q3 == 2:
        print_success("¡Correcto! Complex tiene 100 archivos y 20 errores.")
        score += 1
    else:
        print_error("Incorrecto. 'complex' es el stress test (100 archivos, 20 errores).")

    print(f"\n{BOLD}Resultado: {score}/3{RESET}")

    if score >= 2:
        print_success("¡TUTORIAL COMPLETADO!")
        save_progress(4, 0)
        return True
    else:
        print_warning("Repasa los conceptos antes de finalizar.")
        return False


def show_completion():
    """Show completion message."""
    clear_screen()
    print_header("¡TUTORIAL COMPLETADO!")

    print(f"""
    {GREEN}╔═══════════════════════════════════════════════════════════════╗
    ║                                                               ║
    ║   ¡Felicidades! Has completado el tutorial de URP-CLI.       ║
    ║                                                               ║
    ║   Ahora sabes:                                                ║
    ║   ✓ Monitorear el sistema (pain, recent, vitals)             ║
    ║   ✓ Optimizar contexto (cc-*)                                 ║
    ║   ✓ Gestionar working memory (focus/unfocus)                  ║
    ║   ✓ Ejecutar A/B tests y experimentos PCx                     ║
    ║   ✓ Analizar resultados y optimizar                          ║
    ║                                                               ║
    ╚═══════════════════════════════════════════════════════════════╝{RESET}

    {BOLD}PRÓXIMOS PASOS:{RESET}

    1. Usa el sistema diariamente
    2. Ejecuta 'pcx-all' semanalmente
    3. Revisa 'cc-recommend' mensualmente
    4. Contribuye soluciones con 'learn'

    {CYAN}Comando rápido para empezar:{RESET}

      cc-status && tokens-status

    """)

    wait_for_enter("Presiona ENTER para salir...")


def show_menu():
    """Show main menu."""
    clear_screen()
    print_header("TUTORIAL INTERACTIVO URP-CLI")

    level, step = load_progress()

    print(f"""
    {BOLD}NIVELES:{RESET}

    {GREEN if level > 1 else ''}  1. Fundamentos      {'✓ Completado' if level > 1 else '(15 min)'}{RESET}
    {GREEN if level > 2 else ''}  2. Optimización     {'✓ Completado' if level > 2 else '(30 min)'}{RESET}
    {GREEN if level > 3 else ''}  3. A/B Testing      {'✓ Completado' if level > 3 else '(45 min)'}{RESET}

    {CYAN}Progreso actual: Nivel {level}{RESET}
    """)

    choice = ask_question(
        "¿Qué quieres hacer?",
        [
            f"Continuar desde Nivel {min(level, 3)}",
            "Empezar desde el principio",
            "Ir a un nivel específico",
            "Salir"
        ]
    )

    return choice, level


def main():
    """Main tutorial loop."""
    while True:
        choice, current_level = show_menu()

        if choice == 0:  # Continue
            level = min(current_level, 3)
        elif choice == 1:  # Start over
            level = 1
            save_progress(1, 0)
        elif choice == 2:  # Specific level
            level = ask_question(
                "¿A qué nivel quieres ir?",
                ["Nivel 1: Fundamentos", "Nivel 2: Optimización", "Nivel 3: A/B Testing"]
            ) + 1
        else:  # Exit
            print("\n¡Hasta pronto!\n")
            sys.exit(0)

        # Run selected level
        if level == 1:
            level1_intro()
            level1_step1()
            level1_step2()
            level1_step3()
            if level1_quiz():
                continue

        elif level == 2:
            level2_intro()
            level2_step1()
            level2_step2()
            level2_step3()
            level2_step4()
            if level2_quiz():
                continue

        elif level == 3:
            level3_intro()
            level3_step1()
            level3_step2()
            level3_step3()
            level3_step4()
            if level3_quiz():
                show_completion()
                break


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\nTutorial interrumpido. Tu progreso ha sido guardado.\n")
        sys.exit(0)
