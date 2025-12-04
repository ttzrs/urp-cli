// Package skills provides skill management for AI agents.
package skills

import "time"

// Category groups skills by function.
type Category string

const (
	CategoryDev      Category = "dev"      // Development & Automation
	CategorySecurity Category = "security" // Cybersecurity & OSINT
	CategoryContent  Category = "content"  // Content Creation & Social
	CategoryData     Category = "data"     // Data & Analytics
	CategoryGrowth   Category = "growth"   // Personal Development & Philosophy
	CategoryBusiness Category = "business" // Business & Research
	CategoryCore     Category = "core"     // System & Maintenance
)

// CategoryInfo describes a skill category.
type CategoryInfo struct {
	ID          Category `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
}

// Categories defines all skill categories.
var Categories = map[Category]CategoryInfo{
	CategoryDev: {
		ID:          CategoryDev,
		Title:       "Development & Automation",
		Description: "Programming, servers, tooling, browser automation",
		Icon:        "💻",
	},
	CategorySecurity: {
		ID:          CategorySecurity,
		Title:       "Security & Intelligence",
		Description: "OSINT, pentesting, reconnaissance, red team",
		Icon:        "🛡️",
	},
	CategoryContent: {
		ID:          CategoryContent,
		Title:       "Content Creation",
		Description: "Blogging, social media, art, media production",
		Icon:        "📝",
	},
	CategoryData: {
		ID:          CategoryData,
		Title:       "Data & Analytics",
		Description: "Scraping, metrics, extraction, visualization",
		Icon:        "📊",
	},
	CategoryGrowth: {
		ID:          CategoryGrowth,
		Title:       "Growth & Philosophy",
		Description: "Personal development, life, philosophy, telos",
		Icon:        "🧠",
	},
	CategoryBusiness: {
		ID:          CategoryBusiness,
		Title:       "Business & Research",
		Description: "Sales, projects, documents, research",
		Icon:        "🏢",
	},
	CategoryCore: {
		ID:          CategoryCore,
		Title:       "System & Core",
		Description: "Upgrades, maintenance, core functionality",
		Icon:        "⚙️",
	},
}

// Skill represents a capability that can be invoked.
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    Category `json:"category"`
	Description string   `json:"description"`
	Version     string   `json:"version"`

	// Source can be: file path, URL, or inline
	Source     string `json:"source"`
	SourceType string `json:"source_type"` // file, url, inline, mcp

	// Context files to load
	ContextFiles []string `json:"context_files,omitempty"`

	// Agent to spawn (if any)
	Agent string `json:"agent,omitempty"`

	// Tags for search
	Tags []string `json:"tags,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UsageCount int       `json:"usage_count"`
}

// SkillExecution records a skill invocation.
type SkillExecution struct {
	ID        string    `json:"id"`
	SkillID   string    `json:"skill_id"`
	SessionID string    `json:"session_id"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Duration  int64     `json:"duration_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
