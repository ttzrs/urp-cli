# URP Roadmap

```
SOLID Score: 93/100
Build: ✅ passing | Tests: 446+ | LOC: ~50k
```

---

## Estado Actual ✅

Todo lo planificado para las fases P0-P5 está **completado**:

- ✅ Provider Factory unificado
- ✅ Dockerfile multi-target (minimal, base-agent, master, worker, dev-full, gpu)
- ✅ Browser Worker con go-rod
- ✅ Protocol envelope JSON-lines
- ✅ Orchestrator con DIP
- ✅ Multi-worker parallel spawn
- ✅ 40+ comandos CLI
- ✅ TUI con Bubble Tea
- ✅ Sistema de Skills
- ✅ Vector Store
- ✅ Cognitive Engine

---

## Mejoras Futuras (Nice to Have)

### TUI
- [ ] Smooth scroll en viewport
- [ ] Colores personalizables via config
- [ ] Panel de status de containers (CPU/RAM)

### Agent
- [ ] Retries automáticos con exponential backoff
- [ ] Métricas de tokens por sesión/proyecto
- [ ] Dashboard de workers activos

### Vector Store
- [ ] Code embeddings para búsqueda semántica
- [ ] Auto-index de archivos nuevos
- [ ] Búsqueda híbrida (keyword + semántica)

### CLI/UX
- [ ] bash/zsh autocompletion
- [ ] Barras de progreso para operaciones largas
- [ ] Notificaciones desktop cuando workers terminan
- [ ] Modo verbose (`-v`, `-vv`, `-vvv`)

### Testing
- [ ] E2E integration tests
- [ ] Performance benchmarks
- [ ] Coverage > 80%

### Infrastructure
- [ ] Homebrew formula
- [ ] APT/YUM repositories
- [ ] Multi-arch builds (amd64, arm64)

### Experimental
- [ ] Soporte LLM local (Ollama)
- [ ] Sistema de plugins (Go plugins o WASM)
- [ ] Event webhooks

---

## Archivo (Completado)

<details>
<summary>Fases P0-P5 SOLID Refactoring</summary>

- Graph record helpers centralizados
- Truncate centralizado
- Error handling consolidado
- God objects divididos (agent, healer, planning)
- DIP aplicado (HTTPClient, ImmuneSystem, specs.Engine)
- OCP type switches eliminados
- ISP aplicado (GraphReader/Writer, VectorSearcher/Writer)
- Store interface compliance (Ping/Close)
- Orchestrator DIP (MasterProtocol)
- Container service layer
- OpenCode Phases 1-6 (tools, commands, sessions, agents)
- Provider Factory
- Browser Worker go-rod
- Dockerfile minimization

</details>

---

*Última actualización: Auto-generado*
