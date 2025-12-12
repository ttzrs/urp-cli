# URP Documentation Index

Complete guide to URP CLI documentation. Start here!

---

## 🚀 Getting Started (Read First)

| Document | Purpose | Read Time |
|----------|---------|-----------|
| [QUICKSTART.md](QUICKSTART.md) | Installation, configuration, first commands | 10 min |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System design, components, data flow | 15 min |
| [COMMANDS.md](COMMANDS.md) | Complete command reference | 20 min |

---

## 📚 Core Documentation (By Role)

### For Users

Start with QUICKSTART.md, then reference COMMANDS.md as needed.

```bash
# Setup (5 min)
Go 1.24.0+ → Build → Configure API key → urp doctor

# First run (5 min)
urp launch /path/to/project
urp code ingest .
urp code stats

# Common tasks (see QUICKSTART)
- Bug fixes with Master/Worker
- Feature development with specs
- Code analysis
- Error learning
```

### For Developers

Read in this order:

1. **QUICKSTART.md** - Get running locally
2. **TESTING.md** - Understand test infrastructure
3. **../CLAUDE.md** - Architecture, packages, development patterns
4. **ARCHITECTURE.md** - System design and PRU model
5. **COMMANDS.md** - Command implementation reference

Key files:
- `../CLAUDE.md` - Comprehensive development guide (717 lines)
- `TESTING.md` - Run tests, write tests, debug (578 lines)
- `ARCHITECTURE.md` - System design, graph schema
- `ARCHITECTURE_FLOW.md` - Execution flow diagrams

### For Contributors

1. Read `../CLAUDE.md` section "Architecture: Code Organization"
2. Review `TESTING.md` section "Writing Tests"
3. Check `ARCHITECTURE.md` for design patterns
4. Run tests before committing: `go test ./...`

---

## 📋 Complete Document List

### Active Documentation

| File | Purpose | Status | Size |
|------|---------|--------|------|
| **QUICKSTART.md** | Installation & first commands | ✅ Current | 7.3K |
| **ARCHITECTURE.md** | System design, components | ✅ Current | 15K |
| **COMMANDS.md** | Command reference (40+ commands) | ✅ Current | 11K |
| **TESTING.md** | Testing guide, how to run tests | ✅ Current | 12K |
| **ARCHITECTURE_FLOW.md** | Execution flow diagrams | ✅ Current | 39K |
| **progress.md** | Development history, session notes | ✅ Current | 29K |

### Reference Documentation

| File | Purpose | Status | Size |
|------|---------|--------|------|
| **EXECUTIVE_SUMMARY.md** | High-level system assessment | ✅ Reference | 7.4K |
| **SISTEMA_NATE.md** | Message compaction & E2E validation | ✅ Reference | 15K |
| **README_ANALYSIS.md** | Analysis of existing README | 📋 Archive | 6.7K |

### Legacy / Archive

| File | Purpose | Status | Size |
|------|---------|--------|------|
| ANALYSIS_AND_LEARNINGS.md | Detailed system analysis | 🗂️ Archive | 25K |
| CONSOLIDATION_SUMMARY.md | Architecture consolidation notes | 🗂️ Archive | 13K |
| DISCUSSION_AGENDA.md | Meeting notes from 2024-12-02 | 🗂️ Archive | 14K |
| MODEL_SELECTION_MIGRATION.md | v1→v2 provider migration | 🗂️ Archive | 12K |
| MODEL_SELECTION_EXAMPLES.md | v1→v2 examples | 🗂️ Archive | 3.9K |
| AFTER_TESTING.md | Proxy testing notes | 🗂️ Archive | 6.5K |
| AUDIT_COMPLETE.md | Audit log (single entry) | 🗂️ Archive | 5.9K |
| session-learnings-2024-12-02.md | Session notes | 🗂️ Archive | 3.9K |

---

## 🔍 Find What You Need

### "How do I...?"

| Task | Document | Section |
|------|----------|---------|
| Set up URP locally? | QUICKSTART.md | Prerequisites, Installation, Configuration |
| Run my first command? | QUICKSTART.md | First Run |
| Fix a bug with AI? | QUICKSTART.md | Common Workflows → Bug Fix |
| Develop features in parallel? | QUICKSTART.md | Common Workflows → Multi-Task Feature Dev |
| List all commands? | COMMANDS.md | (full reference) |
| Understand the architecture? | ARCHITECTURE.md | (full overview) |
| See data flow diagrams? | ARCHITECTURE_FLOW.md | (ASCII flow diagrams) |
| Write tests? | TESTING.md | Writing Tests section |
| Run tests and debug? | TESTING.md | Running Tests, Debugging |
| Configure LLM providers? | QUICKSTART.md | Configuration section |
| Use Master/Worker pattern? | QUICKSTART.md | Common Workflows |
| Learn system internals? | ../CLAUDE.md | (developer guide) |

### "What's the status of...?"

| Feature | Document |
|---------|----------|
| Current architecture SOLID score | EXECUTIVE_SUMMARY.md |
| Test coverage | TESTING.md → Test Coverage table |
| Development progress | progress.md |
| Known issues | TESTING.md → Troubleshooting |

---

## 📖 Reading Paths

### Path 1: Quick Start (30 minutes)
```
1. QUICKSTART.md (10 min)
   - Prerequisites
   - Installation
   - Configuration
   - First Run

2. COMMANDS.md (20 min)
   - Skim command list
   - Focus on: launch, spawn, ask
```

### Path 2: User Workflows (1 hour)
```
1. QUICKSTART.md (complete, 30 min)
2. COMMANDS.md (reference section, 20 min)
3. ARCHITECTURE.md (overview, 10 min)
```

### Path 3: Developer Setup (2 hours)
```
1. QUICKSTART.md (installation, 10 min)
2. ../CLAUDE.md (architecture & packages, 40 min)
3. TESTING.md (test infrastructure, 30 min)
4. ARCHITECTURE.md (system design, 20 min)
```

### Path 4: Deep Dive (4+ hours)
```
1. QUICKSTART.md (complete, 30 min)
2. ../CLAUDE.md (complete, 60 min)
3. TESTING.md (complete, 60 min)
4. ARCHITECTURE.md (complete, 30 min)
5. ARCHITECTURE_FLOW.md (diagrams, 20 min)
6. progress.md (skim development history, 10 min)
```

---

## 🎯 Quick Reference

### Installation
```bash
git clone https://github.com/ttzrs/urp-cli
cd urp-cli/go && go build -o urp ./cmd/urp
echo "ANTHROPIC_API_KEY=sk-ant-..." > ~/.urp-go/.env
urp doctor
```

### First Commands
```bash
urp launch /path/to/project    # Start interactive session
urp code ingest .              # Analyze project
urp code stats                 # Show metrics
urp think wisdom "error msg"   # Find similar solutions
```

### Master/Worker
```bash
urp launch .                   # Master (read-only)
urp spawn                      # Worker (read-write)
urp ask urp-proj-w1 "task"    # Send instruction
urp workers                    # List active workers
urp kill urp-proj-w1          # Stop worker
```

### Testing
```bash
cd go
go test ./...                  # All tests
go test -v ./...              # Verbose
go test -cover ./...          # Coverage
go test -race ./...           # Race detection
```

---

## 📝 Document Standards

All documentation follows these conventions:

- **Code examples**: Bash unless otherwise specified
- **Paths**: Absolute paths preferred, relative OK in context
- **Status badges**: ✅ Current, ⚠️ Needs Update, 🗂️ Archive, 📋 Reference
- **Tables**: Used for quick reference
- **Sections**: H2 (`##`) for main sections, H3 (`###`) for subsections
- **Links**: Internal docs use relative paths, external use full URLs

---

## 🔄 Documentation Maintenance

### Last Updated
- **QUICKSTART.md**: 2025-12-12 (Go version, provider config)
- **COMMANDS.md**: 2025-12-12 (added compile, router, serve, models)
- **TESTING.md**: 2025-12-12 (created)
- **CLAUDE.md**: 2025-12-12 (package structure, vector store)
- **ARCHITECTURE.md**: 2025-12-06
- **progress.md**: 2025-12-03

### Validation
- All commands in COMMANDS.md exist in code ✅
- All packages in CLAUDE.md documented ✅
- Examples are runnable ✅
- Links are functional ✅

### Contributing
When adding new features, update:
1. COMMANDS.md (new commands)
2. QUICKSTART.md (new workflows)
3. TESTING.md (new tests)
4. ../CLAUDE.md (architecture changes)
5. This README (if adding new doc)

---

## 📞 Getting Help

### Error Messages
1. Check TESTING.md → Troubleshooting
2. Search COMMANDS.md for command syntax
3. Check progress.md for known issues
4. Run `urp doctor` for environment diagnostics

### Testing Issues
See: [TESTING.md](TESTING.md) → Debugging Test Failures

### Architecture Questions
See: [ARCHITECTURE.md](ARCHITECTURE.md) or [../CLAUDE.md](../CLAUDE.md)

### Command Usage
See: [COMMANDS.md](COMMANDS.md) → search for command name

---

## 🗂️ Archive Directory

The `/docs/archive/` directory contains legacy documentation:
- Old provider migration guides
- Deprecated testing procedures
- Historical analysis documents

These are kept for reference but not actively maintained.

---

**Documentation Status**: ✅ Comprehensive (18 documents)
**Last Audit**: 2025-12-12
**Coverage**: Users ✅ | Developers ✅ | Contributors ✅
