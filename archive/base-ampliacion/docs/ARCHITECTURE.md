# URP (Universal Robotic Programmer) Architecture

## 1. Core Philosophy: Context as a Compiled View
URP moves away from the traditional "Chat History" paradigm (concatenating user/assistant messages) towards a **State-Based Paradigm**. 

The Context (Prompt) sent to the LLM is not a log; it is a **Computed View** rendered freshly for each execution step. This view is "compiled" from a ground-truth state stored in a graph database, ensuring the agent operates on the *current reality* rather than a decaying textual history.

> **Analogy:** We do not send the LLM a "chat log" of the game; we render the "current frame" of the game state.

## 2. The Dual-LLM Pipeline
To optimize for both cost and reasoning quality, URP employs a heterogeneous pipeline of models, aligning with NeurIPS 2025 trends ("Gated Attention" and "Model Convergence").

### Component A: The Gate (Context Compiler)
*   **Role:** Noise Filter & Semantic Router.
*   **Model:** `Qwen 2.5 Coder` (Local/Cloud) or `GLM-4.6V` (if multimodal).
*   **Function:** Implements a "Sigmoid Attention Gate" logic. It processes raw logs, long documentation, and previous steps to strictly filter out noise.
*   **Output:** Returns highly dense, relevant context or an empty string (Sparsity).

### Component B: The Master (Reasoning Core)
*   **Role:** Complex Planning & Code Generation.
*   **Model:** `DeepSeek-V3`, `Claude 3.5 Sonnet`, or `GPT-4o`.
*   **Function:** Receives the clean, compiled context and executes high-level engineering tasks.

---

## 3. Two-Phase Knowledge Acquisition
URP separates knowledge into "Theory" (unverified information from manuals) and "Experience" (verified facts from simulation/execution).

### Phase 1: "The Theorist" (Ingestion)
*   **Objective:** Ingest massive technical documentation (PDFs, Videos) into the Knowledge Graph.
*   **Status:** Facts are stored as `status: "THEORETICAL"`.
*   **Technology:** 
    *   **Multi-Vector Retrieval (ColPali):** Uses patch-level embeddings for images and video keyframes to maintain spatial resolution (e.g., distinguishing "Terminal 1" from "Terminal 2" in a wiring diagram).
    *   **Causal Extraction:** Maps relationships (e.g., `IF LI1=True THEN Motor=Run`).

### Phase 2: "The Empiricist" (Validation)
*   **Objective:** Validate theoretical rules against physical/logical simulators.
*   **Mechanism:** 
    *   The agent generates test programs (e.g., OpenPLC Structured Text) based on theoretical rules.
    *   It runs them in a sandbox (The Worker).
*   **Outcome:** 
    *   **Success:** Promotes the node to `status: "VALIDATED"`.
    *   **Failure:** Learns a constraint (e.g., `REQUIRES Jumper J2`).
    *   **Refinement:** The "Context Compiler" prefers VALIDATED nodes over THEORETICAL ones.

---

## 4. Data Architecture (Memgraph + LanceDB)

The system uses a hybrid storage approach to model the "World State".

### Memgraph Schema (The Graph)
Separates Ephemeral Events from Durable State.

```cypher
// --- DURABLE STATE (The "Desk") ---
(:Session {id: "sess_01", goal: "Refactor Auth"})
(:File {path: "/src/auth.go", status: "modified", content_hash: "..."})
(:Error {msg: "Connection Refused", status: "active"}) 
(:Artifact {path: "/logs/debug.log", type: "file_ref"})

// --- EPHEMERAL EVENTS (The "Log") ---
(:Step {seq: 10, thought: "Checking logs", tool: "exec"})
(:Step)-[:PRODUCED]->(:Artifact)
(:Step)-[:MODIFIED]->(:File)

// --- KNOWLEDGE (The "Brain") ---
(:Rule {proposition: "IF LI1=TRUE THEN Motor=RUN", status: "VALIDATED"})
```

### LanceDB (The Vector Store)
Stores embeddings for semantic search, specifically:
*   **Patch-Level Embeddings:** For diagrams and visual manual parts.
*   **Code Embeddings:** For semantic code search.

---

## 5. The Context Compilation Algorithm

The `ContextCompiler` constructs the prompt in layers:

1.  **Stable Prefix (Cache Anchor):**
    *   Immutable system instructions and goal definition.
    *   Optimized for KV-Cache hits (reducing latency/cost by ~90%).

2.  **Computed State (The "Desk"):**
    *   Queries Memgraph for *active* errors and *currently* modified files.
    *   If an error is fixed, it disappears from this view (preventing "Context Rot").

3.  **Relevant Knowledge (Retrieval):**
    *   Fetches `VALIDATED` rules relevant to the current sub-goal.

4.  **Offloaded History (Artifacts):**
    *   Tool outputs > 500 chars are offloaded to files.
    *   The prompt only sees `[Tool Output Saved to: /tmp/output.log]`.
    *   The agent must explicitly `read` or `grep` the artifact if needed.

5.  **Gated Logs (Noise Filter):**
    *   Raw logs are passed through the "Gate" (Local LLM) to extract only critical lines.
