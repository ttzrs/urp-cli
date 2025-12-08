# URP Roadmap

```
SOLID Score: 93/100 (2025-12-08)
Build: passing | Tests: 446+ | LOC: ~43k
```

---

## Backlog

### P1 - Provider Factory
- [ ] Create `internal/opencode/provider/factory.go`
- [ ] Unify provider creation logic (Anthropic, OpenAI, OpenRouter)

### P2 - God Objects (optional)
- [ ] Split `tui/agent.go` if >700 LOC
- [ ] Further split `orchestrator.go` if needed

---

## Improvements

### TUI
- [ ] Smooth scroll in viewport
- [ ] Customizable colors via config
- [ ] Real-time container status panel (CPU/RAM)
- [ ] Session list preview

### OpenCode Agent
- [ ] Multiple parallel workers
- [ ] Active workers dashboard
- [ ] Automatic retries with exponential backoff
- [ ] Token usage metrics per session/project

### Vector Store
- [ ] Code embeddings for semantic search
- [ ] Auto-index new files
- [ ] Hybrid search (keyword + semantic)
- [ ] Embedding cache

### Memory
- [ ] Auto-learn successful patterns
- [ ] Proactive suggestions based on context
- [ ] Export/import memory between projects
- [ ] Auto-cleanup obsolete memories

---

## Nice to Have

### CLI/UX
- [ ] bash/zsh autocompletion
- [ ] Progress bars for long operations
- [ ] Desktop notifications when workers finish
- [ ] Verbose mode (`-v`, `-vv`, `-vvv`)

### Security
- [ ] Additional sandboxing (gVisor, Kata)
- [ ] Encrypted audit logs
- [ ] Secret management (Vault)
- [ ] LLM API rate limiting

### Testing
- [ ] E2E integration tests
- [ ] Performance benchmarks
- [ ] Load tests for multi-worker
- [ ] Fuzzing for code parsers
- [ ] Coverage > 80%

### Observability
- [ ] Prometheus/Grafana integration
- [ ] Distributed tracing (OpenTelemetry)
- [ ] Worker failure alerts
- [ ] Web monitoring dashboard

---

## Infrastructure

### Builds
- [ ] Multi-arch (amd64, arm64)
- [ ] Truly portable static binaries
- [ ] Reduce binary size (~25MB currently)

### Distribution
- [ ] Homebrew formula
- [ ] APT/YUM repositories
- [ ] Docker Hub automated builds

### CI/CD
- [ ] Auto tests on PRs
- [ ] Security scanning (Snyk, Trivy)
- [ ] Dependabot
- [ ] Auto release notes

---

## Experimental

### AI/ML
- [ ] Local LLM support (Ollama, llama.cpp)
- [ ] Local embeddings for privacy
- [ ] Model fine-tuning for URP

### Extensibility
- [ ] Plugin system (Go plugins or WASM)
- [ ] Custom LLM providers
- [ ] Custom agent tools
- [ ] Event webhooks

---

## Completed (archive)

<details>
<summary>Phase 1-12 SOLID Refactoring (2025-12)</summary>

- Graph record helpers centralized
- Truncate centralized
- Error handling consolidated
- God objects split (agent, healer, planning)
- DIP applied (HTTPClient, ImmuneSystem, specs.Engine)
- OCP type switches eliminated
- ISP applied (GraphReader/Writer, VectorSearcher/Writer)
- Store interface compliance (Ping/Close)
- Orchestrator DIP (MasterProtocol)
- Container service layer
- OpenCode Phases 1-6 (tools, commands, sessions, agents)

</details>

---

*Last updated: 2025-12-08*
