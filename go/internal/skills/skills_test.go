package skills

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Domain Tests ---

func TestCategories(t *testing.T) {
	// All categories should have info
	cats := []Category{CategoryDev, CategorySecurity, CategoryContent, CategoryData, CategoryGrowth, CategoryBusiness, CategoryCore}
	for _, cat := range cats {
		info, ok := Categories[cat]
		assert.True(t, ok, "category %s should exist", cat)
		assert.NotEmpty(t, info.Title)
		assert.NotEmpty(t, info.Icon)
	}
}

func TestSkillFields(t *testing.T) {
	skill := &Skill{
		ID:          "skill-123",
		Name:        "test-skill",
		Category:    CategoryDev,
		Description: "A test skill",
		Version:     "1.0",
		Source:      "/path/to/skill.md",
		SourceType:  "file",
		Tags:        []string{"test", "example"},
	}

	assert.Equal(t, "skill-123", skill.ID)
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, CategoryDev, skill.Category)
	assert.Len(t, skill.Tags, 2)
}

// --- Loader Tests ---

func TestParseSkillFile(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		path         string
		categoryHint string
		wantName     string
		wantCategory Category
		wantAgent    string
		wantTags     []string
	}{
		{
			name: "frontmatter with all fields",
			content: `---
name: CustomName
category: security
agent: pentester
version: 2.0
tags: [osint, recon]
---
This is the description.`,
			path:         "/skills/test.md",
			categoryHint: "",
			wantName:     "CustomName",
			wantCategory: CategorySecurity,
			wantAgent:    "pentester",
			wantTags:     []string{"osint", "recon"},
		},
		{
			name:         "no frontmatter",
			content:      "Just a description without frontmatter.",
			path:         "/skills/dev/myskill.md",
			categoryHint: "dev",
			wantName:     "myskill",
			wantCategory: CategoryDev,
		},
		{
			name:         "category from hint",
			content:      "Description",
			path:         "/skills/security/scanner.md",
			categoryHint: "security",
			wantName:     "scanner",
			wantCategory: CategorySecurity,
		},
		{
			name: "context files",
			content: `---
name: WithContext
context:
  - ~/.claude/file1.md
  - ~/.claude/file2.md
---
Has context files.`,
			path:         "/skills/test.md",
			wantName:     "WithContext",
			wantCategory: CategoryDev, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := parseSkillFile(tt.content, tt.path, tt.categoryHint)
			require.NotNil(t, skill)

			assert.Equal(t, tt.wantName, skill.Name)
			assert.Equal(t, tt.wantCategory, skill.Category)
			if tt.wantAgent != "" {
				assert.Equal(t, tt.wantAgent, skill.Agent)
			}
			if tt.wantTags != nil {
				assert.Equal(t, tt.wantTags, skill.Tags)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	yaml := `
name: TestSkill
category: data
agent: analyzer
version: 1.5
tags: [analytics, metrics]
`
	skill := &Skill{}
	parseFrontmatter(yaml, skill)

	assert.Equal(t, "TestSkill", skill.Name)
	assert.Equal(t, CategoryData, skill.Category)
	assert.Equal(t, "analyzer", skill.Agent)
	assert.Equal(t, "1.5", skill.Version)
	assert.Equal(t, []string{"analytics", "metrics"}, skill.Tags)
}

func TestParseListValue(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"[tag1, tag2, tag3]", []string{"tag1", "tag2", "tag3"}},
		{"[single]", []string{"single"}},
		{"tag1, tag2", []string{"tag1", "tag2"}},
		{"[]", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseListValue(tt.input)
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestInferCategory(t *testing.T) {
	tests := []struct {
		hint    string
		name    string
		wantCat Category
	}{
		{"dev", "anything", CategoryDev},
		{"security", "scanner", CategorySecurity},
		{"", "osint-tool", CategorySecurity},
		{"", "blogging-helper", CategoryContent},
		{"", "brightdata-scraper", CategoryData},
		{"", "philosophy-notes", CategoryGrowth},
		{"", "sales-tracker", CategoryBusiness},
		{"", "upgrade-system", CategoryCore},
		{"", "unknown-name", CategoryDev}, // default
	}

	for _, tt := range tests {
		t.Run(tt.hint+"_"+tt.name, func(t *testing.T) {
			got := inferCategory(tt.hint, tt.name)
			assert.Equal(t, tt.wantCat, got)
		})
	}
}

// --- Store Tests (with mock) ---

type mockDriver struct {
	records     []graph.Record
	executeErr  error
	writeErr    error
	lastQuery   string
	lastParams  map[string]any
	writeCalled bool
}

func (m *mockDriver) Execute(ctx context.Context, query string, params map[string]any) ([]graph.Record, error) {
	m.lastQuery = query
	m.lastParams = params
	return m.records, m.executeErr
}

func (m *mockDriver) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	m.writeCalled = true
	m.lastQuery = query
	m.lastParams = params
	return m.writeErr
}

func (m *mockDriver) Close() error { return nil }

func (m *mockDriver) Ping(ctx context.Context) error { return nil }

func TestStoreCreate(t *testing.T) {
	mock := &mockDriver{}
	store := NewStore(mock)

	skill := &Skill{
		ID:       "skill-123",
		Name:     "test-skill",
		Category: CategoryDev,
	}

	err := store.Create(context.Background(), skill)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "MERGE")
	assert.Equal(t, "test-skill", mock.lastParams["name"])
}

func TestStoreGet(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"sk.id":          "skill-123",
				"sk.name":        "test-skill",
				"sk.category":    "dev",
				"sk.description": "A test",
				"sk.version":     "1.0",
				"sk.source":      "/path/file.md",
				"sk.source_type": "file",
				"sk.created_at":  int64(1700000000),
				"sk.updated_at":  int64(1700000100),
				"sk.usage_count": int64(5),
			},
		},
	}
	store := NewStore(mock)

	skill, err := store.Get(context.Background(), "skill-123")
	require.NoError(t, err)
	require.NotNil(t, skill)

	assert.Equal(t, "skill-123", skill.ID)
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, CategoryDev, skill.Category)
	assert.Equal(t, 5, skill.UsageCount)
}

func TestStoreGetNotFound(t *testing.T) {
	mock := &mockDriver{records: []graph.Record{}}
	store := NewStore(mock)

	_, err := store.Get(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStoreList(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{"sk.id": "1", "sk.name": "skill1", "sk.category": "dev"},
			{"sk.id": "2", "sk.name": "skill2", "sk.category": "dev"},
		},
	}
	store := NewStore(mock)

	skills, err := store.List(context.Background(), CategoryDev)
	require.NoError(t, err)
	assert.Len(t, skills, 2)
	assert.Contains(t, mock.lastQuery, "category: $category")
}

func TestStoreListAll(t *testing.T) {
	mock := &mockDriver{records: []graph.Record{}}
	store := NewStore(mock)

	_, err := store.List(context.Background(), "")
	require.NoError(t, err)
	assert.NotContains(t, mock.lastQuery, "$category")
}

func TestStoreSearch(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{"sk.id": "1", "sk.name": "test-skill", "sk.category": "dev"},
		},
	}
	store := NewStore(mock)

	skills, err := store.Search(context.Background(), "test")
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Contains(t, mock.lastQuery, "CONTAINS")
	assert.Equal(t, "test", mock.lastParams["pattern"])
}

func TestStoreIncrementUsage(t *testing.T) {
	mock := &mockDriver{}
	store := NewStore(mock)

	err := store.IncrementUsage(context.Background(), "skill-123")
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "usage_count + 1")
}

func TestStoreDelete(t *testing.T) {
	mock := &mockDriver{}
	store := NewStore(mock)

	err := store.Delete(context.Background(), "skill-123")
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "DELETE")
}

func TestStoreLogExecution(t *testing.T) {
	mock := &mockDriver{}
	store := NewStore(mock)

	exec := &SkillExecution{
		ID:        "exec-123",
		SkillID:   "skill-123",
		SessionID: "sess-123",
		Input:     "test input",
		Output:    "test output",
		Success:   true,
	}

	err := store.LogExecution(context.Background(), exec)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "EXECUTED")
}

func TestStoreStats(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"total":       int64(10),
				"total_usage": int64(100),
				"executions":  int64(50),
			},
		},
	}
	store := NewStore(mock)

	stats, err := store.Stats(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(10), stats["total_skills"])
	assert.Equal(t, int64(100), stats["total_usage"])
	assert.Equal(t, int64(50), stats["total_executions"])
}
