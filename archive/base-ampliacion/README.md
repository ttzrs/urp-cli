# URP: Universal Robotic Programmer

**Project Cere**

URP is an autonomous engineering agent designed to bridge the gap between theoretical knowledge (manuals, docs) and empirical reality (simulation, execution).

## Documentation

*   **[Architecture Overview](docs/ARCHITECTURE.md):** Detailed explanation of the "Context as Compiled View", Two-Phase Learning, and Dual-LLM pipeline.
*   **[Audit Report](docs/GEMINI_MEMORY_AUDIT.md):** Summary of the architectural audit performed against original notes/research.

## Quick Start

1.  **Environment:** Copy `.env.example` to `.env` (if available) or configure your LLM/Graph credentials.
2.  **Run Master Node:**
    ```bash
    go run cmd/urp-master/main.go
    ```

## Core Concepts

*   **The Theorist:** Ingests massive documentation into a Knowledge Graph.
*   **The Empiricist:** Validates knowledge via simulation (OpenPLC, etc.).
*   **Context Compiler:** Renders a clean, noise-free prompt for the LLM using a "Sigmoid Gate" logic.
