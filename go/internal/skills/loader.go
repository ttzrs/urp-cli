package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Loader handles skill discovery and registration.
type Loader struct {
	store *Store
}

// NewLoader creates a skill loader.
func NewLoader(store *Store) *Loader {
	return &Loader{store: store}
}

// LoadFromDirectory scans a directory for skill definitions.
// Skills are markdown files with YAML frontmatter.
func (l *Loader) LoadFromDirectory(ctx context.Context, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading skill directory: %w", err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			// Subdirectories represent categories
			catDir := filepath.Join(dir, entry.Name())
			n, err := l.loadCategoryDir(ctx, catDir, entry.Name())
			if err != nil {
				continue // Skip broken categories
			}
			loaded += n
		} else if strings.HasSuffix(entry.Name(), ".md") {
			// Root-level skills
			path := filepath.Join(dir, entry.Name())
			if err := l.loadSkillFile(ctx, path, ""); err != nil {
				continue
			}
			loaded++
		}
	}

	return loaded, nil
}

func (l *Loader) loadCategoryDir(ctx context.Context, dir string, categoryHint string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := l.loadSkillFile(ctx, path, categoryHint); err != nil {
			continue
		}
		loaded++
	}
	return loaded, nil
}

func (l *Loader) loadSkillFile(ctx context.Context, path string, categoryHint string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	skill := parseSkillFile(string(content), path, categoryHint)
	if skill == nil {
		return fmt.Errorf("invalid skill file: %s", path)
	}

	// Check if exists
	existing, _ := l.store.GetByName(ctx, skill.Name)
	if existing != nil {
		// Update existing
		skill.ID = existing.ID
		skill.CreatedAt = existing.CreatedAt
		skill.UsageCount = existing.UsageCount
	}

	return l.store.Create(ctx, skill)
}

// parseSkillFile extracts skill metadata from markdown.
// Format:
// ---
// name: SkillName
// category: dev
// tags: [tag1, tag2]
// agent: optional-agent
// context:
//   - ~/.claude/context/file1.md
// ---
// Description here...
func parseSkillFile(content, path, categoryHint string) *Skill {
	name := strings.TrimSuffix(filepath.Base(path), ".md")

	// Default skill
	skill := &Skill{
		ID:         ulid.Make().String(),
		Name:       name,
		Category:   inferCategory(categoryHint, name),
		Source:     path,
		SourceType: "file",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Parse YAML frontmatter if present
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			parseFrontmatter(parts[1], skill)
			skill.Description = strings.TrimSpace(parts[2])
		}
	} else {
		skill.Description = strings.TrimSpace(content)
	}

	// Truncate description
	if len(skill.Description) > 500 {
		skill.Description = skill.Description[:500]
	}

	return skill
}

func parseFrontmatter(yaml string, skill *Skill) {
	lines := strings.Split(yaml, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			skill.Name = value
		case "category":
			skill.Category = Category(value)
		case "agent":
			skill.Agent = value
		case "version":
			skill.Version = value
		case "tags":
			skill.Tags = parseListValue(value)
		case "context":
			// Multi-line list handled separately
		}
	}

	// Parse context files (multi-line)
	inContext := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "context:") {
			inContext = true
			continue
		}
		if inContext {
			if strings.HasPrefix(strings.TrimSpace(line), "-") {
				file := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
				skill.ContextFiles = append(skill.ContextFiles, file)
			} else if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inContext = false
			}
		}
	}
}

func parseListValue(value string) []string {
	// Handle [tag1, tag2] format
	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func inferCategory(hint, name string) Category {
	hint = strings.ToLower(hint)
	name = strings.ToLower(name)

	// Map directory names to categories
	categoryMap := map[string]Category{
		// Dev
		"dev": CategoryDev, "development": CategoryDev, "automation": CategoryDev,
		"agentserver": CategoryDev, "daemon": CategoryDev, "voiceserver": CategoryDev,
		"browserautomation": CategoryDev, "createcli": CategoryDev, "createskill": CategoryDev,
		"software": CategoryDev, "parser": CategoryDev, "webassessment": CategoryDev,
		"dotfiles": CategoryDev, "cloudflare": CategoryDev, "mcp": CategoryDev,
		"frontenddesign": CategoryDev,

		// Security
		"security": CategorySecurity, "osint": CategorySecurity, "recon": CategorySecurity,
		"privateinvestigator": CategorySecurity, "redteam": CategorySecurity,
		"promptinjection": CategorySecurity, "sensitive": CategorySecurity, "backups": CategorySecurity,

		// Content
		"content": CategoryContent, "blogging": CategoryContent, "newsletter": CategoryContent,
		"article": CategoryContent, "storyexplanation": CategoryContent, "linkedinpost": CategoryContent,
		"xpost": CategoryContent, "canonicalcontent": CategoryContent, "communication": CategoryContent,
		"art": CategoryContent, "becreative": CategoryContent, "images": CategoryContent,
		"media": CategoryContent, "voicenarration": CategoryContent, "videotranscript": CategoryContent,

		// Data
		"data": CategoryData, "analytics": CategoryData, "brightdata": CategoryData,
		"datawastewaterca": CategoryData, "usmetrics": CategoryData, "extractalpha": CategoryData,
		"extracttranscript": CategoryData, "observability": CategoryData, "visualization": CategoryData,
		"evals": CategoryData,

		// Growth
		"growth": CategoryGrowth, "personal": CategoryGrowth, "philosophy": CategoryGrowth,
		"aphorisms": CategoryGrowth, "firstprinciples": CategoryGrowth, "telos": CategoryGrowth,
		"life": CategoryGrowth, "lifelog": CategoryGrowth, "human": CategoryGrowth,
		"hormozi": CategoryGrowth, "increaseentropy": CategoryGrowth,

		// Business
		"business": CategoryBusiness, "sales": CategoryBusiness, "projects": CategoryBusiness,
		"paimanagement": CategoryBusiness, "council": CategoryBusiness, "documents": CategoryBusiness,
		"news": CategoryBusiness, "research": CategoryBusiness, "anthropicchanges": CategoryBusiness,
		"fabric": CategoryBusiness,

		// Core
		"core": CategoryCore, "upgrade": CategoryCore, "upgrades": CategoryCore,
	}

	// Check hint first
	if cat, ok := categoryMap[hint]; ok {
		return cat
	}

	// Check name
	for key, cat := range categoryMap {
		if strings.Contains(name, key) {
			return cat
		}
	}

	return CategoryDev // Default
}

// RegisterBuiltins adds default skills.
func (l *Loader) RegisterBuiltins(ctx context.Context) error {
	builtins := []*Skill{
		{
			ID:          ulid.Make().String(),
			Name:        "researcher",
			Category:    CategoryBusiness,
			Description: "Research and information gathering on any topic",
			Version:     "1.0",
			SourceType:  "builtin",
			Agent:       "researcher",
			Tags:        []string{"research", "search", "information"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          ulid.Make().String(),
			Name:        "pentester",
			Category:    CategorySecurity,
			Description: "Security testing and vulnerability assessment",
			Version:     "1.0",
			SourceType:  "builtin",
			Agent:       "pentester",
			Tags:        []string{"security", "pentest", "vulnerability"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          ulid.Make().String(),
			Name:        "designer",
			Category:    CategoryContent,
			Description: "Visual testing and browser automation",
			Version:     "1.0",
			SourceType:  "builtin",
			Agent:       "designer",
			Tags:        []string{"visual", "browser", "design"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          ulid.Make().String(),
			Name:        "upgrade",
			Category:    CategoryCore,
			Description: "Plan and iterate version upgrades",
			Version:     "1.0",
			SourceType:  "builtin",
			Tags:        []string{"upgrade", "version", "iterate"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	for _, skill := range builtins {
		existing, _ := l.store.GetByName(ctx, skill.Name)
		if existing == nil {
			if err := l.store.Create(ctx, skill); err != nil {
				return err
			}
		}
	}

	return nil
}
