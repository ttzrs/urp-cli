package agent

import (
	"github.com/joss/urp/internal/opencode/domain"
)

// Registry manages available agent configurations
type Registry struct {
	agents map[string]domain.Agent
}

// NewRegistry creates a new agent registry
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]domain.Agent),
	}
}

// Register adds an agent to the registry
func (r *Registry) Register(agent domain.Agent) {
	r.agents[agent.Name] = agent
}

// Get returns an agent by name
func (r *Registry) Get(name string) (domain.Agent, bool) {
	agent, ok := r.agents[name]
	return agent, ok
}

// List returns all registered agents
func (r *Registry) List() []domain.Agent {
	result := make([]domain.Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		result = append(result, agent)
	}
	return result
}

// Names returns all agent names
func (r *Registry) Names() []string {
	result := make([]string, 0, len(r.agents))
	for name := range r.agents {
		result = append(result, name)
	}
	return result
}

// DefaultRegistry returns a registry with built-in agents
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Primary coding agent
	r.Register(domain.Agent{
		Name:        "code",
		Description: "Full-featured coding assistant with all tools",
		Mode:        domain.AgentModePrimary,
		BuiltIn:     true,
		Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
		Tools: map[string]bool{
			"bash":       true,
			"read":       true,
			"write":      true,
			"edit":       true,
			"multi_edit": true,
			"glob":       true,
			"grep":       true,
			"ls":         true,
			"web_fetch":  true,
			"web_search": true,
			"patch":      true,
			"todo_write": true,
			"todo_read":  true,
			"task":       true,
		},
		Permissions: domain.AgentPermissions{
			Edit:        domain.PermissionAllow,
			ExternalDir: domain.PermissionAsk,
			Bash: map[string]domain.Permission{
				"*": domain.PermissionAsk,
			},
		},
	})

	// Explore agent (read-only)
	r.Register(domain.Agent{
		Name:        "explore",
		Description: "Fast codebase exploration (read-only)",
		Mode:        domain.AgentModeSubagent,
		BuiltIn:     true,
		Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
		Tools: map[string]bool{
			"read": true,
			"glob": true,
			"grep": true,
			"ls":   true,
		},
		Permissions: domain.AgentPermissions{
			Edit: domain.PermissionDeny,
		},
	})

	// Plan agent (architecture)
	r.Register(domain.Agent{
		Name:        "plan",
		Description: "Software architect for design and planning",
		Mode:        domain.AgentModeSubagent,
		BuiltIn:     true,
		Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
		Prompt: `You are a software architect. Analyze requirements and create implementation plans.
Focus on:
- Breaking down complex tasks into steps
- Identifying dependencies and risks
- Suggesting best patterns and approaches
Do NOT write code - only plan and design.`,
		Tools: map[string]bool{
			"read":       true,
			"glob":       true,
			"grep":       true,
			"ls":         true,
			"todo_write": true,
		},
		Permissions: domain.AgentPermissions{
			Edit: domain.PermissionDeny,
		},
	})

	// Build agent (full power)
	r.Register(domain.Agent{
		Name:        "build",
		Description: "Execute coding tasks with full tool access",
		Mode:        domain.AgentModeSubagent,
		BuiltIn:     true,
		Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
		Tools: map[string]bool{
			"bash":       true,
			"read":       true,
			"write":      true,
			"edit":       true,
			"multi_edit": true,
			"glob":       true,
			"grep":       true,
			"ls":         true,
			"patch":      true,
		},
		Permissions: domain.AgentPermissions{
			Edit:        domain.PermissionAllow,
			ExternalDir: domain.PermissionAllow,
			Bash: map[string]domain.Permission{
				"*": domain.PermissionAllow,
			},
		},
	})

	// Review agent (code review)
	r.Register(domain.Agent{
		Name:        "review",
		Description: "Code reviewer focused on quality and security",
		Mode:        domain.AgentModeSubagent,
		BuiltIn:     true,
		Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
		Prompt: `You are a senior code reviewer. Review code for:
- Bugs and logic errors
- Security vulnerabilities
- Performance issues
- Code style and maintainability
- Missing error handling
Be specific and constructive.`,
		Tools: map[string]bool{
			"read": true,
			"glob": true,
			"grep": true,
			"ls":   true,
			"bash": true, // For git diff
		},
		Permissions: domain.AgentPermissions{
			Edit: domain.PermissionDeny,
			Bash: map[string]domain.Permission{
				"git": domain.PermissionAllow,
			},
		},
	})

	return r
}
