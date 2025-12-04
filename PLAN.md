# OpenCode Evolution Strategy: The "Total Orchestrator"

## Vision
Transition `urp-cli` from a human-driven tool (Master/Worker) to an autonomous AI system (`OpenCode`) that orchestrates the entire development lifecycle:
1.  **Input:** High-level Specs (from Spec-Kit).
2.  **Memory:** Persistent Graph + Vector Context.
3.  **Output:** Validated Code via Workers.

## The "Winning Strategy": Strangler Fig Pattern
We will not break the current system. We will build the new system alongside it, sharing the same brain (Memgraph) but using a superior cognitive loop.

### Phase 1: Hybrid Infrastructure (Current Step)
**Goal:** Duplicate infrastructure to allow parallel development of the new core.
- [x] Define `opencode-master` and `opencode-worker` in `docker-compose.yml`.
- [ ] These services act as the "Lab" for the new logic.

### Phase 2: Core Transplant (Integration)
**Goal:** Port the advanced logic from `opencode-go` reference into `urp/internal/opencode`.
- [ ] **Agents:** Port `internal/agent` (Build, Plan, Explore). These replace the simple "ask" command.
- [ ] **Providers:** Port `internal/provider` to support multi-LLM routing natively (Anthropic/OpenAI/Google).
- [ ] **TUI:** Port `internal/tui` for a better interactive experience in the Master container.

### Phase 3: Spec-Kit Integration ( The "Contract")
**Goal:** Define the "Rules of Engagement".
- [ ] Implement `specs` package based on GitHub Spec-Kit.
- [ ] **Workflow:**
    1.  `urp spec new "Feature X"` -> Generates Spec MD.
    2.  `urp spec plan` -> AI generates Tasks from Spec.
    3.  `urp spec run` -> OpenCode Orchestrator executes Tasks via Workers.
- [ ] **Constitution:** Enforce global rules (e.g., "Always add tests", "Use Clean Arch").

### Phase 4: Unification (The Swap)
**Goal:** `OpenCode` becomes the default.
- [ ] The `urp` binary defaults to OpenCode mode.
- [ ] `urp launch` starts the OpenCode Orchestrator.
- [ ] Legacy manual commands become sub-tools of the Orchestrator.

## Immediate Next Steps
1.  Use `opencode-master` to develop the new `internal/opencode/orchestrator`.
2.  Port the `agent` package from the reference.
