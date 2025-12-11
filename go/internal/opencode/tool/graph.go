// Package tool provides graph/memory tools for the agent.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/cognitive"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/vector"
)

// graphDB is the shared graph connection for tools.
var graphDB graph.Driver

// SetGraphDB sets the graph database connection for all graph tools.
func SetGraphDB(db graph.Driver) {
	graphDB = db
}

// ------------------------------------------------------------------
// GraphSearch: Search codebase structure via graph
// ------------------------------------------------------------------

type GraphSearch struct{}

func NewGraphSearch() *GraphSearch { return &GraphSearch{} }

func (g *GraphSearch) Info() domain.Tool {
	return domain.Tool{
		ID:          "graph_search",
		Name:        "graph_search",
		Description: "Search the codebase graph for files, functions, classes, and their relationships. Use this BEFORE reading files to find what's relevant.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "What to search for: file names, function names, class names",
				},
				"type": map[string]any{
					"type":        "string",
					"description": "Node type: file, function, class, or all",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 10)",
				},
				"root_path": map[string]any{
					"type":        "string",
					"description": "Filter results to this directory path",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (g *GraphSearch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Graph not connected. Use grep/glob instead."}, nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return &Result{Error: fmt.Errorf("query is required")}, nil
	}

	nodeType, _ := args["type"].(string)
	rootPath, _ := args["root_path"].(string)
	
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	var cypher string
	params := map[string]any{
		"pattern": "(?i).*" + query + ".*",
		"limit":   limit,
		"root":    rootPath,
	}
	
	// Where clause for path filtering
	wherePath := ""
	if rootPath != "" {
		wherePath = "AND file.path STARTS WITH $root"
	}

	switch nodeType {
	case "function":
		cypher = fmt.Sprintf(`
			MATCH (f:Function)
			WHERE f.name =~ $pattern
			MATCH (file:File)-[:CONTAINS]->(f)
			WHERE 1=1 %s
			RETURN f.name as name, 'function' as type, file.path as file, f.line as line
			ORDER BY f.name LIMIT $limit
		`, wherePath)
	case "class":
		cypher = fmt.Sprintf(`
			MATCH (c:Class)
			WHERE c.name =~ $pattern
			MATCH (file:File)-[:CONTAINS]->(c)
			WHERE 1=1 %s
			RETURN c.name as name, 'class' as type, file.path as file, c.line as line
			ORDER BY c.name LIMIT $limit
		`, wherePath)
	case "file":
		fileWhere := ""
		if rootPath != "" {
			fileWhere = "AND f.path STARTS WITH $root"
		}
		cypher = fmt.Sprintf(`
			MATCH (f:File)
			WHERE f.path =~ $pattern %s
			RETURN f.path as name, 'file' as type, f.path as file, 0 as line
			ORDER BY f.path LIMIT $limit
		`, fileWhere)
	default:
		cypher = fmt.Sprintf(`
			MATCH (n) WHERE (n:File OR n:Function OR n:Class)
			  AND (n.name =~ $pattern OR n.path =~ $pattern)
			MATCH (file:File)-[:CONTAINS]->(n)
			WHERE 1=1 %s
			RETURN COALESCE(n.name, n.path) as name,
				CASE WHEN n:File THEN 'file' WHEN n:Function THEN 'function' WHEN n:Class THEN 'class' END as type,
				COALESCE(file.path, n.path) as file, COALESCE(n.line, 0) as line
			ORDER BY name LIMIT $limit
		`, wherePath)
	}

	records, err := graphDB.Execute(ctx, cypher, params)
	if err != nil {
		return &Result{Error: fmt.Errorf("graph query failed: %w", err)}, nil
	}

	if len(records) == 0 {
		return &Result{Output: fmt.Sprintf("No matches for '%s' in graph.", query)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matches:\n", len(records)))
	for _, r := range records {
		name := graph.GetString(r, "name")
		ntype := graph.GetString(r, "type")
		file := graph.GetString(r, "file")
		line := graph.GetInt(r, "line")
		if line > 0 {
			sb.WriteString(fmt.Sprintf("  [%s] %s -> %s:%d\n", ntype, name, file, line))
		} else {
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", ntype, name))
		}
	}

	return &Result{Title: fmt.Sprintf("graph_search: %s", query), Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// MemoryRecall: Search session memories
// ------------------------------------------------------------------

type MemoryRecall struct{}

func NewMemoryRecall() *MemoryRecall { return &MemoryRecall{} }

func (m *MemoryRecall) Info() domain.Tool {
	return domain.Tool{
		ID:          "memory_recall",
		Name:        "memory_recall",
		Description: "Search session memories for previous notes, decisions, and observations.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Keywords or description of what to recall",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Filter: note, decision, observation, or empty for all",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (m *MemoryRecall) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Memory not available (graph not connected)."}, nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return &Result{Error: fmt.Errorf("query is required")}, nil
	}

	kind, _ := args["kind"].(string)
	memCtx := memory.NewContext()
	sessionMem := memory.NewSessionMemory(graphDB, memCtx)

	results, err := sessionMem.Recall(ctx, query, 10, kind, 1)
	if err != nil {
		return &Result{Error: fmt.Errorf("recall failed: %w", err)}, nil
	}

	if len(results) == 0 {
		return &Result{Output: "No matching memories found."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n", len(results)))
	for _, r := range results {
		sim := ""
		if r.Similarity > 0 {
			sim = fmt.Sprintf(" (%.0f%%)", r.Similarity*100)
		}
		sb.WriteString(fmt.Sprintf("  [%s]%s %s\n", r.Kind, sim, r.Text))
	}

	return &Result{Title: "memory_recall", Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// Wisdom: Find similar past errors and solutions
// ------------------------------------------------------------------

type Wisdom struct{}

func NewWisdom() *Wisdom { return &Wisdom{} }

func (w *Wisdom) Info() domain.Tool {
	return domain.Tool{
		ID:          "wisdom",
		Name:        "wisdom",
		Description: "Search for similar past errors and their solutions. Use this when you encounter an error.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type":        "string",
					"description": "The error message to search for",
				},
				"threshold": map[string]any{
					"type":        "number",
					"description": "Minimum similarity 0-1 (default 0.3)",
				},
			},
			"required": []string{"error"},
		},
	}
}

func (w *Wisdom) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Wisdom not available (graph not connected)."}, nil
	}

	errorMsg, _ := args["error"].(string)
	if errorMsg == "" {
		return &Result{Error: fmt.Errorf("error message is required")}, nil
	}

	threshold := 0.3
	if t, ok := args["threshold"].(float64); ok && t > 0 {
		threshold = t
	}

	svc := cognitive.NewWisdomService(graphDB)
	matches, err := svc.ConsultWisdom(ctx, errorMsg, threshold, "")
	if err != nil {
		return &Result{Error: fmt.Errorf("wisdom search failed: %w", err)}, nil
	}

	if len(matches) == 0 {
		return &Result{Output: "No similar past errors found. This may be a new type of error."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d similar past errors:\n", len(matches)))
	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("\n%d. [%.0f%% match]\n", i+1, m.Similarity*100))
		sb.WriteString(fmt.Sprintf("   Command: %s\n", m.Command))
		sb.WriteString(fmt.Sprintf("   Error: %s\n", truncateStr(m.Error, 100)))
		if m.Solution != "" {
			sb.WriteString(fmt.Sprintf("   SOLUTION: %s\n", m.Solution))
		}
	}

	return &Result{Title: "wisdom", Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// ContextSearch: Hybrid vector + graph search
// ------------------------------------------------------------------

type ContextSearch struct{}

func NewContextSearch() *ContextSearch { return &ContextSearch{} }

func (c *ContextSearch) Info() domain.Tool {
	return domain.Tool{
		ID:          "context_search",
		Name:        "context_search",
		Description: "Find relevant code using semantic search. Use this to find where to start on a task.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Describe what you're trying to find",
				},
			},
			"required": []string{"prompt"},
		},
	}
}

func (c *ContextSearch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Context search not available (graph not connected)."}, nil
	}

	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return &Result{Error: fmt.Errorf("prompt is required")}, nil
	}

	embedder := vector.GetDefaultEmbedder()
	vecStore := vector.Default()

	if embedder.Dimensions() == 0 {
		return &Result{Output: "Vector embeddings not configured. Use graph_search instead."}, nil
	}

	promptVec, err := embedder.Embed(ctx, prompt)
	if err != nil {
		return &Result{Error: fmt.Errorf("embedding failed: %w", err)}, nil
	}

	optimizer := cognitive.NewContextOptimizer(graphDB, vecStore)
	optimized, err := optimizer.GetOptimizedContext(ctx, promptVec)
	if err != nil {
		return &Result{Error: fmt.Errorf("context search failed: %w", err)}, nil
	}

	if len(optimized) == 0 {
		return &Result{Output: "No relevant files found. Try graph_search for specific terms."}, nil
	}

	var sb strings.Builder
	sb.WriteString("Most relevant files:\n")
	for i, f := range optimized {
		sb.WriteString(fmt.Sprintf("  %d. [%.2f] %s\n", i+1, f.Energy, f.Path))
	}

	return &Result{Title: "context_search", Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// KnowledgeQuery: Search shared knowledge base
// ------------------------------------------------------------------

type KnowledgeQuery struct{}

func NewKnowledgeQuery() *KnowledgeQuery { return &KnowledgeQuery{} }

func (k *KnowledgeQuery) Info() domain.Tool {
	return domain.Tool{
		ID:          "knowledge_query",
		Name:        "knowledge_query",
		Description: "Search the knowledge base for rules, patterns, fixes, and insights from past sessions.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "What to search for",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Filter: error, fix, rule, pattern, insight",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (k *KnowledgeQuery) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Knowledge base not available (graph not connected)."}, nil
	}

	query, _ := args["query"].(string)
	if query == "" {
		return &Result{Error: fmt.Errorf("query is required")}, nil
	}

	kind, _ := args["kind"].(string)
	memCtx := memory.NewContext()
	kb := memory.NewKnowledgeStore(graphDB, memCtx)

	results, err := kb.Query(ctx, query, 10, "all", kind)
	if err != nil {
		return &Result{Error: fmt.Errorf("knowledge query failed: %w", err)}, nil
	}

	if len(results) == 0 {
		return &Result{Output: "No matching knowledge found."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d knowledge entries:\n", len(results)))
	for _, r := range results {
		sim := ""
		if r.Similarity > 0 {
			sim = fmt.Sprintf(" (%.0f%%)", r.Similarity*100)
		}
		sb.WriteString(fmt.Sprintf("  [%s/%s]%s %s\n", r.Scope, r.Kind, sim, truncateStr(r.Text, 80)))
	}

	return &Result{Title: "knowledge_query", Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// CodeDependencies: Find callers and callees
// ------------------------------------------------------------------

type CodeDependencies struct{}

func NewCodeDependencies() *CodeDependencies { return &CodeDependencies{} }

func (c *CodeDependencies) Info() domain.Tool {
	return domain.Tool{
		ID:          "code_deps",
		Name:        "code_deps",
		Description: "Find dependencies: what calls a function and what it calls.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "File path or function name",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "callers, callees, or both (default)",
				},
				"depth": map[string]any{
					"type":        "integer",
					"description": "Levels to traverse 1-3 (default 1)",
				},
				"root_path": map[string]any{
					"type":        "string",
					"description": "Filter results to this directory path",
				},
			},
			"required": []string{"target"},
		},
	}
}

func (c *CodeDependencies) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Graph not connected."}, nil
	}

	target, _ := args["target"].(string)
	if target == "" {
		return &Result{Error: fmt.Errorf("target is required")}, nil
	}

	direction, _ := args["direction"].(string)
	if direction == "" {
		direction = "both"
	}
	
	rootPath, _ := args["root_path"].(string)

	depth := 1
	if d, ok := args["depth"].(float64); ok && d >= 1 && d <= 3 {
		depth = int(d)
	}

	var sb strings.Builder
	params := map[string]any{
		"pattern": "(?i).*" + target + ".*",
		"root":    rootPath,
	}
	
	wherePath := ""
	if rootPath != "" {
		wherePath = "AND file.path STARTS WITH $root"
	}

	if direction == "callers" || direction == "both" {
		cypher := fmt.Sprintf(`
			MATCH (caller)-[:CALLS*1..%d]->(target)
			WHERE target.name =~ $pattern OR target.path =~ $pattern
			MATCH (file:File)-[:CONTAINS]->(caller)
			WHERE 1=1 %s
			RETURN DISTINCT caller.name as name, file.path as file, caller.line as line
			LIMIT 20
		`, depth, wherePath)

		records, err := graphDB.Execute(ctx, cypher, params)
		if err == nil && len(records) > 0 {
			sb.WriteString("Called by:\n")
			for _, r := range records {
				name := graph.GetString(r, "name")
				file := graph.GetString(r, "file")
				line := graph.GetInt(r, "line")
				if file != "" && line > 0 {
					sb.WriteString(fmt.Sprintf("  %s -> %s:%d\n", name, file, line))
				} else {
					sb.WriteString(fmt.Sprintf("  %s\n", name))
				}
			}
		}
	}

	if direction == "callees" || direction == "both" {
		cypher := fmt.Sprintf(`
			MATCH (source)-[:CALLS*1..%d]->(callee)
			WHERE source.name =~ $pattern OR source.path =~ $pattern
			MATCH (file:File)-[:CONTAINS]->(callee)
			WHERE 1=1 %s
			RETURN DISTINCT callee.name as name, file.path as file, callee.line as line
			LIMIT 20
		`, depth, wherePath)

		records, err := graphDB.Execute(ctx, cypher, params)
		if err == nil && len(records) > 0 {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("Calls:\n")
			for _, r := range records {
				name := graph.GetString(r, "name")
				file := graph.GetString(r, "file")
				line := graph.GetInt(r, "line")
				if file != "" && line > 0 {
					sb.WriteString(fmt.Sprintf("  %s -> %s:%d\n", name, file, line))
				} else {
					sb.WriteString(fmt.Sprintf("  %s\n", name))
				}
			}
		}
	}

	if sb.Len() == 0 {
		return &Result{Output: fmt.Sprintf("No dependencies found for '%s'.", target)}, nil
	}

	return &Result{Title: fmt.Sprintf("code_deps: %s", target), Output: sb.String()}, nil
}

// ------------------------------------------------------------------
// MemoryAdd: Store a note
// ------------------------------------------------------------------

type MemoryAdd struct{}

func NewMemoryAdd() *MemoryAdd { return &MemoryAdd{} }

func (m *MemoryAdd) Info() domain.Tool {
	return domain.Tool{
		ID:          "memory_add",
		Name:        "memory_add",
		Description: "Store a note, decision, or observation for later recall.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "What to remember",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Type: note, decision, observation, summary",
				},
				"importance": map[string]any{
					"type":        "integer",
					"description": "Importance 1-5 (default 2)",
				},
			},
			"required": []string{"text"},
		},
	}
}

func (m *MemoryAdd) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Memory not available (graph not connected)."}, nil
	}

	text, _ := args["text"].(string)
	if text == "" {
		return &Result{Error: fmt.Errorf("text is required")}, nil
	}

	kind := "note"
	if k, ok := args["kind"].(string); ok && k != "" {
		kind = k
	}

	importance := 2
	if i, ok := args["importance"].(float64); ok && i >= 1 && i <= 5 {
		importance = int(i)
	}

	memCtx := memory.NewContext()
	sessionMem := memory.NewSessionMemory(graphDB, memCtx)

	id, err := sessionMem.Add(ctx, text, kind, importance, nil)
	if err != nil {
		return &Result{Error: fmt.Errorf("failed to store memory: %w", err)}, nil
	}

	return &Result{
		Title:  "memory_add",
		Output: fmt.Sprintf("Remembered [%s]: %s (id: %s)", kind, truncateStr(text, 60), id),
	}, nil
}

// ------------------------------------------------------------------
// GraphStats: Summary stats
// ------------------------------------------------------------------

type GraphStats struct{}

func NewGraphStats() *GraphStats { return &GraphStats{} }

func (g *GraphStats) Info() domain.Tool {
	return domain.Tool{
		ID:          "graph_stats",
		Name:        "graph_stats",
		Description: "Get codebase statistics: file count, function count, etc.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"root_path": map[string]any{
					"type":        "string",
					"description": "Filter statistics to this directory path",
				},
			},
		},
	}
}

func (g *GraphStats) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	if graphDB == nil {
		return &Result{Output: "Graph not connected."}, nil
	}
	
	rootPath, _ := args["root_path"].(string)
	
	var query string
	params := map[string]any{"root": rootPath}
	
	if rootPath != "" {
		query = `
			MATCH (n)
			WHERE n.path STARTS WITH $root OR EXISTS {
				MATCH (file:File)-[:CONTAINS]->(n)
				WHERE file.path STARTS WITH $root
			}
			WITH labels(n)[0] as label
			RETURN label, count(*) as count
			ORDER BY count DESC
		`
	} else {
		query = `
			MATCH (n)
			WITH labels(n)[0] as label
			RETURN label, count(*) as count
			ORDER BY count DESC
		`
	}

	records, err := graphDB.Execute(ctx, query, params)
	if err != nil {
		return &Result{Error: fmt.Errorf("stats query failed: %w", err)}, nil
	}

	stats := make(map[string]int)
	for _, r := range records {
		label := graph.GetString(r, "label")
		count := graph.GetInt(r, "count")
		stats[label] = count
	}

	data, _ := json.MarshalIndent(stats, "", "  ")
	return &Result{Title: "graph_stats", Output: string(data)}, nil
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// Verify interfaces
var (
	_ Executor = (*GraphSearch)(nil)
	_ Executor = (*MemoryRecall)(nil)
	_ Executor = (*Wisdom)(nil)
	_ Executor = (*ContextSearch)(nil)
	_ Executor = (*KnowledgeQuery)(nil)
	_ Executor = (*CodeDependencies)(nil)
	_ Executor = (*MemoryAdd)(nil)
	_ Executor = (*GraphStats)(nil)
)
