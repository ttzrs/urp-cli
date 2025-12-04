package skills

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// Store manages skills in the graph database.
type Store struct {
	db graph.Driver
}

// NewStore creates a skill store.
func NewStore(db graph.Driver) *Store {
	return &Store{db: db}
}

// Create adds a new skill.
func (s *Store) Create(ctx context.Context, skill *Skill) error {
	query := `
		CREATE (sk:Skill {
			id: $id,
			name: $name,
			category: $category,
			description: $description,
			version: $version,
			source: $source,
			source_type: $source_type,
			context_files: $context_files,
			agent: $agent,
			tags: $tags,
			created_at: $created_at,
			updated_at: $updated_at,
			usage_count: 0
		})
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":            skill.ID,
		"name":          skill.Name,
		"category":      string(skill.Category),
		"description":   skill.Description,
		"version":       skill.Version,
		"source":        skill.Source,
		"source_type":   skill.SourceType,
		"context_files": skill.ContextFiles,
		"agent":         skill.Agent,
		"tags":          skill.Tags,
		"created_at":    skill.CreatedAt.Unix(),
		"updated_at":    skill.UpdatedAt.Unix(),
	})
}

// Get retrieves a skill by ID.
func (s *Store) Get(ctx context.Context, id string) (*Skill, error) {
	query := `
		MATCH (sk:Skill {id: $id})
		RETURN sk.id, sk.name, sk.category, sk.description, sk.version,
		       sk.source, sk.source_type, sk.context_files, sk.agent, sk.tags,
		       sk.created_at, sk.updated_at, sk.usage_count
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("skill not found: %s", id)
	}
	return recordToSkill(records[0]), nil
}

// GetByName retrieves a skill by name.
func (s *Store) GetByName(ctx context.Context, name string) (*Skill, error) {
	query := `
		MATCH (sk:Skill {name: $name})
		RETURN sk.id, sk.name, sk.category, sk.description, sk.version,
		       sk.source, sk.source_type, sk.context_files, sk.agent, sk.tags,
		       sk.created_at, sk.updated_at, sk.usage_count
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"name": name})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return recordToSkill(records[0]), nil
}

// List returns all skills, optionally filtered by category.
func (s *Store) List(ctx context.Context, category Category) ([]*Skill, error) {
	var query string
	params := map[string]any{}

	if category != "" {
		query = `
			MATCH (sk:Skill {category: $category})
			RETURN sk.id, sk.name, sk.category, sk.description, sk.version,
			       sk.source, sk.source_type, sk.context_files, sk.agent, sk.tags,
			       sk.created_at, sk.updated_at, sk.usage_count
			ORDER BY sk.usage_count DESC, sk.name
		`
		params["category"] = string(category)
	} else {
		query = `
			MATCH (sk:Skill)
			RETURN sk.id, sk.name, sk.category, sk.description, sk.version,
			       sk.source, sk.source_type, sk.context_files, sk.agent, sk.tags,
			       sk.created_at, sk.updated_at, sk.usage_count
			ORDER BY sk.category, sk.usage_count DESC, sk.name
		`
	}

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	skills := make([]*Skill, 0, len(records))
	for _, r := range records {
		skills = append(skills, recordToSkill(r))
	}
	return skills, nil
}

// Search finds skills by tag or name pattern.
func (s *Store) Search(ctx context.Context, pattern string) ([]*Skill, error) {
	query := `
		MATCH (sk:Skill)
		WHERE sk.name CONTAINS $pattern
		   OR any(t IN sk.tags WHERE t CONTAINS $pattern)
		   OR sk.description CONTAINS $pattern
		RETURN sk.id, sk.name, sk.category, sk.description, sk.version,
		       sk.source, sk.source_type, sk.context_files, sk.agent, sk.tags,
		       sk.created_at, sk.updated_at, sk.usage_count
		ORDER BY sk.usage_count DESC
		LIMIT 20
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"pattern": pattern})
	if err != nil {
		return nil, err
	}

	skills := make([]*Skill, 0, len(records))
	for _, r := range records {
		skills = append(skills, recordToSkill(r))
	}
	return skills, nil
}

// IncrementUsage bumps the usage counter.
func (s *Store) IncrementUsage(ctx context.Context, id string) error {
	query := `
		MATCH (sk:Skill {id: $id})
		SET sk.usage_count = sk.usage_count + 1,
		    sk.updated_at = $now
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":  id,
		"now": time.Now().Unix(),
	})
}

// Delete removes a skill.
func (s *Store) Delete(ctx context.Context, id string) error {
	query := `MATCH (sk:Skill {id: $id}) DETACH DELETE sk`
	return s.db.ExecuteWrite(ctx, query, map[string]any{"id": id})
}

// LogExecution records a skill execution.
func (s *Store) LogExecution(ctx context.Context, exec *SkillExecution) error {
	query := `
		MATCH (sk:Skill {id: $skill_id})
		CREATE (e:SkillExecution {
			id: $id,
			session_id: $session_id,
			input: $input,
			output: $output,
			duration_ms: $duration,
			success: $success,
			error: $error,
			timestamp: $timestamp
		})
		CREATE (sk)-[:EXECUTED]->(e)
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":         exec.ID,
		"skill_id":   exec.SkillID,
		"session_id": exec.SessionID,
		"input":      truncate(exec.Input, 1000),
		"output":     truncate(exec.Output, 2000),
		"duration":   exec.Duration,
		"success":    exec.Success,
		"error":      exec.Error,
		"timestamp":  exec.Timestamp.Unix(),
	})
}

// Stats returns skill statistics.
func (s *Store) Stats(ctx context.Context) (map[string]any, error) {
	query := `
		MATCH (sk:Skill)
		WITH count(sk) as total,
		     sum(sk.usage_count) as total_usage
		OPTIONAL MATCH (e:SkillExecution)
		WITH total, total_usage, count(e) as executions
		RETURN total, total_usage, executions
	`
	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	stats := map[string]any{
		"total_skills":     0,
		"total_usage":      0,
		"total_executions": 0,
	}

	if len(records) > 0 {
		if v, ok := records[0]["total"].(int64); ok {
			stats["total_skills"] = v
		}
		if v, ok := records[0]["total_usage"].(int64); ok {
			stats["total_usage"] = v
		}
		if v, ok := records[0]["executions"].(int64); ok {
			stats["total_executions"] = v
		}
	}

	// Category breakdown
	catQuery := `
		MATCH (sk:Skill)
		RETURN sk.category as category, count(sk) as count
		ORDER BY count DESC
	`
	catRecords, err := s.db.Execute(ctx, catQuery, nil)
	if err == nil {
		categories := make(map[string]int)
		for _, r := range catRecords {
			cat := getString(r, "category")
			cnt := getInt(r, "count")
			categories[cat] = cnt
		}
		stats["by_category"] = categories
	}

	return stats, nil
}

func recordToSkill(r graph.Record) *Skill {
	return &Skill{
		ID:           getString(r, "sk.id"),
		Name:         getString(r, "sk.name"),
		Category:     Category(getString(r, "sk.category")),
		Description:  getString(r, "sk.description"),
		Version:      getString(r, "sk.version"),
		Source:       getString(r, "sk.source"),
		SourceType:   getString(r, "sk.source_type"),
		ContextFiles: getStringSlice(r, "sk.context_files"),
		Agent:        getString(r, "sk.agent"),
		Tags:         getStringSlice(r, "sk.tags"),
		CreatedAt:    time.Unix(getInt64(r, "sk.created_at"), 0),
		UpdatedAt:    time.Unix(getInt64(r, "sk.updated_at"), 0),
		UsageCount:   getInt(r, "sk.usage_count"),
	}
}

func getString(r graph.Record, key string) string {
	if v, ok := r[key].(string); ok {
		return v
	}
	return ""
}

func getInt(r graph.Record, key string) int {
	if v, ok := r[key].(int64); ok {
		return int(v)
	}
	return 0
}

func getInt64(r graph.Record, key string) int64 {
	if v, ok := r[key].(int64); ok {
		return v
	}
	return 0
}

func getStringSlice(r graph.Record, key string) []string {
	if v, ok := r[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
