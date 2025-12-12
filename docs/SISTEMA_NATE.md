# SISTEMA_NATE: URP Context Architecture

**Based on**: Nate Wildermuth's Context Engineering Framework (Dec 2025)
**Source Research**: Google ADK, Anthropic, Manus
**Date**: 2025-12-12
**Status**: Design Document (ready for implementation)

---

## Core Thesis

URP-IA has **two distinct context problems** that must be solved independently:

1. **Cross-Session Memory** (Domain Memory) - Agent knows where it left off after restart
2. **Within-Session Coherence** (Context Engineering) - Agent stays sharp for hours, not degrades after 30 minutes

**This document defines architecture for both, drawing from production systems.**

---

## The Two Context Failures URP Must Avoid

### Failure 1: Domain Memory Loss
Agent runs Task A, makes progress, session ends. Session B starts fresh, no memory of progress. Treats entire task as new.

**Current Status**: Partially addressed via knowledge base + graph storage
**Gap**: Not integrated into agent execution flow
**Solution**: Structured external records + session bootstrap

### Failure 2: Within-Session Degradation
Agent runs fine for 20 minutes. By minute 45, reasoning is circular, repeating failed approaches, forgetting early constraints.

**Current Status**: Not addressed (yet)
**Gap**: Context accumulation model (append-only transcript)
**Solution**: Compiled context views + strategic compaction

---

## SISTEMA_NATE: Four-Layer Architecture

### Layer 1: External Records (Domain Memory, Across Sessions)

**What persists between sessions:**
- Task definitions and goals
- Progress logs (what was tried, what worked)
- Validation criteria and constraints
- Learned heuristics
- Resolved decisions

**Stored in**: Memgraph (graph DB) + LanceDB (vector store)

**Technology**: Already implemented - phase 1's knowledge base + graph DB

**Retrieval**: Query-based, loaded at session start

### Layer 2: Session Log (Structured Events, Within Session)

**What captures everything that happens in one session:**
- User inputs (typed, with metadata)
- Agent decisions (decision logs from Model Router consolidation ✓)
- Tool calls and results (structured, not raw text)
- State changes (model selections, tool outcomes)
- Control signals (loops, retries, failures)

**NOT sent to model**: Stored for observability, replay, debugging

**Format**: Typed event stream (not opaque text transcript)

```go
type SessionEvent struct {
    Timestamp   time.Time
    EventType   string  // "user_input", "decision", "tool_call", "result", "error"
    Content     interface{}
    Metadata    map[string]string
}
```

**Technology to build**: Session event store (can use Memgraph events table or LanceDB)

### Layer 3: Working Context (Computed View, Current Decision)

**What the model actually sees on THIS call:**

- System prompt (stable, cached)
- Agent identity and role
- Current task from external records (retrieved)
- Relevant snippets from session history (selected by schema)
- Active constraints (from last 5 decisions, not all 50)
- Memory hits (vector search + rule-based retrieval)
- Tool results by reference, not full payload

**Token budget**: Target 10-20% of context window (not 90%)

**Computed fresh**: For every LLM call

**Formula**:
```
working_context =
    stable_prefix +
    (current_task_from_domain_memory) +
    (relevant_session_events via schema_filter) +
    (memory_retrieval_results) +
    (tool_artifacts_by_reference)
```

**Technology**: ModelService in consolidation already implements this pattern ✓

### Layer 4: Memory & Artifacts (External Storage)

**What's stored permanently but not in context:**

- Full tool results (file contents, API responses, code)
- Knowledge base entries (vector embeddings + text)
- Artifacts by reference (paths, URLs, identifiers)
- Large documents, datasets, codebases

**Access pattern**: Agent requests → fetch on demand → inject if relevant

**Never pinned in context**: Retrieved only when queried

**Technology**: Filesystem + LanceDB for vectors

---

## Nate's 9 Principles: URP Implementation Status

### ✅ Principle 1: Context Is Computed, Not Accumulated

**Status**: IMPLEMENTED in Model Router consolidation

**How**:
- `ModelService.SelectAndProvision()` computes fresh view
- `DecisionInput` specifies what's relevant NOW
- No append-only transcript

**What this means for URP**:
- Each agent call = fresh working context assembly
- Not: "dump yesterday's decisions + today's task into one growing prompt"
- Yes: "What does the agent need to decide RIGHT NOW?"

**Implementation in Phase 1**: ✓ Model Router consolidation does this

### ✅ Principle 2: Separate Storage from Presentation

**Status**: IMPLEMENTED in Model Router consolidation

**How**:
- DecisionEngine = storage/logic (durable)
- ModelService = API/presentation layer (mutable)
- Session = source of truth
- Working context = computed view

**What this means for URP**:
- Full history stays in session/graph
- Model sees curated subset
- Can optimize presentation without touching storage

**Implementation in Phase 2**: Session event store + view compiler

### ✅ Principle 3: Scope by Default

**Status**: IMPLEMENTED in Model Router consolidation

**How**:
- DecisionInput is minimal
- Must explicitly request memory, artifacts, context
- No pre-loading

**What this means for URP**:
- Don't load entire codebase by default
- Don't include all previous decisions
- Agent asks for what it needs

**Implementation**: Already built into service layer ✓

### ✅ Principle 4: Retrieval Over Pinning

**Status**: IMPLEMENTED in Model Router consolidation

**How**:
- `GetDecisionAudit()` retrieves decisions on demand
- Not: "keep all decisions in context forever"
- Yes: "query the decisions you need"

**What this means for URP**:
- Memory is searchable, not permanently pinned
- External records retrieved by query
- Session log queried for relevant events

**Implementation**: Partially done; needs memory retrieval rules

### ✅ Principle 5: Summarization Must Be Schema-Driven

**Status**: IMPLEMENTED in Model Router consolidation

**How**:
- DecisionLog has explicit fields
- Reasoning chains recorded
- Candidates scored and ranked
- Not blind compression

**What this means for URP**:
- When compacting session history, keep schema-guaranteed fields:
  - Causal steps (why did agent do X?)
  - Active constraints (rules still in effect)
  - Failures (what was tried, didn't work)
  - Open commitments (promises not yet fulfilled)

**Implementation**: DecisionLog schema serves as template

### ✅ Principle 6: Offload Heavy State to Tools and Sandboxes

**Status**: PARTIALLY IMPLEMENTED

**How**:
- ModelService delegates to provider.Factory
- Working context uses references, not full payloads

**What needs building**:
- Tool results written to filesystem, not tokenized
- Artifacts accessed by path reference
- Large search results stored externally

**Example**: When agent searches 100,000 files
- Store results in `/tmp/search_results.json`
- Context contains: `"Search results: see /tmp/search_results.json"`
- Agent fetches full results if relevant

### ❌ Principle 7: Isolate Context with Sub-Agents (Future)

**Status**: ARCHITECTURE READY, NOT YET USED

**How**:
- Planner agent (strategic layer)
- Executor agent (tactical layer)
- Validator agent (quality check)
- Each has own window

**What this means for URP**:
- Phase 1 (Ingestion): Sub-agent for ColPali + GLM-4.6V
- Phase 2 (Validation): Sub-agent for OpenPLC simulator
- Main agent: Orchestrator

**Communication**: Through artifacts + structured results, not shared context

### ❌ Principle 8: Design for Cache Stability

**Status**: NOT YET IMPLEMENTED

**What this means for URP**:
- Stable prefix: System prompt + agent identity (cache-friendly)
- Variable suffix: Current task + working context (cache-breaking)
- Deterministic serialization (JSON key ordering)
- Avoid timestamps at prompt beginning

**Implementation roadmap**:
- Identify stable/variable sections of prompts
- Apply Claude's Prompt Caching API
- Measure KV-cache hit rates
- Target: 90%+ cache hits for multi-hour sessions

### ✅ Principle 9: Let Context Evolve Through Execution

**Status**: IMPLEMENTED in framework, needs wiring

**How**:
- `RecordOutcome()` captures what worked
- Learning store accumulates strategies
- Next agent run sees improved context

**What this means for URP**:
- Session 1: Agent tries approach A, fails. Logs failure.
- Session 2: Agent sees "approach A failed on similar task, try B instead"
- Context improves without retraining

**Implementation**: Model Router's learning integration partially ready

---

## Implementation Phases

### Phase 1: Domain Memory Bootstrap (External Records)

**What**: Structured persistence across sessions

**Files**:
- `internal/memory/domain_store.go` - Task definitions, progress logs
- `internal/memory/constraints.go` - Rules that persist
- `internal/memory/learnings.go` - What worked, what didn't

**Integration**:
- Session startup queries domain memory
- Loads task + constraints into working context
- Records progress + learnings on session end

**Status**: Foundation exists, needs wiring

### Phase 2: Context Engineering (Within-Session Architecture)

**What**: Computed views, session logs, compaction

**Files**:
- `internal/session/store.go` - Session event log
- `internal/session/compiler.go` - Working context assembly
- `internal/session/compactor.go` - Schema-driven summarization

**Changes to existing code**:
- Model Router consolidation ✓ (already done)
- Agent executor: use ModelService.SelectAndProvision()
- Replace append-only transcripts with session events

**Status**: Ready to build on consolidation

### Phase 3: Production Optimization (Cache + Offloading)

**What**: Cost and latency optimization

**Files**:
- `internal/cache/kv_manager.go` - KV-cache hit tracking
- `internal/artifacts/manager.go` - Reference-based storage
- `internal/offload/compiler.go` - Large state to filesystem

**Status**: Design ready, implementation pending

---

## Decision Checkpoints

### Before Phase 1 Implementation

- [ ] Domain memory schema defined (what persists?)
- [ ] Session event schema finalized (what gets logged?)
- [ ] Retrieval patterns specified (how does agent query history?)
- [ ] Compaction schema documented (what survives summarization?)

### Before Phase 2 Integration

- [ ] Session event store tested with real agent tasks
- [ ] Working context compiler produces valid outputs
- [ ] Token reduction measured (target: 50%+ reduction)
- [ ] Accuracy validated (summarized context = full context decisions)

### Before Phase 3 Optimization

- [ ] Cache hit rates baselined
- [ ] Artifact offloading tested at scale
- [ ] Cost per hour calculated and vs. baseline
- [ ] Multi-hour agent runs successful

---

## Key Differences from Naive Approaches

### ❌ Naive: Append-Only Transcript
```
System: You are an agent
Session start: Task is X
Step 1: I will do Y
[tool result]
Step 2: Now I will...
Step 3: Let me try...
... (grows forever)
Step 50: Why did I even try Y?
```

**Problem**: By step 50, agent has lost context from step 1-10

### ✅ SISTEMA_NATE: Computed Views
```
System: You are an agent (cached prefix)
Current task: X (retrieved from domain memory)
Active constraints: [rules still in effect] (schema-preserved)
Recent decisions: [last 3 steps with reasoning] (selected by recency)
Tool state: [results by reference] (artifacts, not payloads)
```

**Benefit**: By step 50, agent sees exactly what matters NOW

---

## Metrics to Track

### Within-Session Coherence
- Agent decision quality over time (regression test at t=5min, t=30min, t=60min)
- Repetition rate (% of redundant tool calls)
- Constraint adherence (% of steps respecting earlier decisions)

### Cost Efficiency
- Tokens per working context (target: <50% of window)
- KV-cache hit rate (target: >90%)
- Cost per hour of agent runtime (target: linear or sub-linear with task duration)

### Cross-Session Memory
- Task continuity score (how well does session 2 pick up from session 1?)
- Knowledge reuse rate (% of queries hitting domain memory)
- Strategy improvement (does agent get faster/better over sessions?)

---

## Failure Modes to Prevent

### ❌ Append-Everything Trap
Symptom: Agent degrades at 30 minutes
Prevention: Implement Phase 2 (context compaction)

### ❌ Blind Summarization
Symptom: Agent forgets critical constraints
Prevention: Define summarization schema before implementing compaction

### ❌ Long-Context Delusion
Symptom: Upgrade to 200K window, performance gets worse
Prevention: Measure actual token usage; implement principle #1 (computed views)

### ❌ Tool Schema Bloat
Symptom: Agent confused, oscillates between similar tools
Prevention: Keep <20 core tools; push complexity to sandbox

### ❌ Cache Destruction
Symptom: High cost despite logic being identical
Prevention: Implement principle #8 (stable prefix, deterministic serialization)

### ❌ Static Prompts
Symptom: Agent never improves
Prevention: Implement principle #9 (evolving context through execution)

---

## Next Session Agenda

When we restart with this system:

1. **Validate Four-Layer Model**
   - Does Memgraph support typed events?
   - What's the Session event schema?
   - How do we query domain memory efficiently?

2. **Implement Phase 1 Skeleton**
   - Create domain_store.go
   - Create session/store.go
   - Wire bootstrap to load domain memory

3. **Integrate with Model Router**
   - Agent executor uses SelectAndProvision()
   - Plug in decision logs to session
   - Test end-to-end flow

4. **Measure Baseline**
   - Run complex task, measure token usage
   - Measure context degradation over time
   - Establish metrics for improvement

---

## Philosophy Summary

**SISTEMA_NATE is about three things**:

1. **Clarity**: What information flows where, and why?
2. **Efficiency**: Every token in context serves a decision
3. **Coherence**: Agents stay sharp for hours, not degrade at 30 minutes

**Core insight from Nate's research**: Bigger windows don't fix degradation. Better architecture does.

URP-IA now has the foundation (Model Router consolidation). Next phase builds the memory architecture that makes agents actually work at scale.

---

**Document Status**: Ready for implementation
**Depends On**: Model Router consolidation (✅ complete)
**Next**: Domain memory bootstrap (Phase 1)
**Timeline**: 8-10 days to full implementation
