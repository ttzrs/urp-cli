package domain

// Agent represents an AI agent configuration
type Agent struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Mode        AgentMode        `json:"mode"`
	BuiltIn     bool             `json:"builtIn"`
	Model       *ModelConfig     `json:"model,omitempty"`
	Prompt      string           `json:"prompt,omitempty"`
	Tools       map[string]bool  `json:"tools"`
	Permissions AgentPermissions `json:"permissions"`
}

type AgentMode string

const (
	AgentModePrimary  AgentMode = "primary"
	AgentModeSubagent AgentMode = "subagent"
)

type ModelConfig struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type AgentPermissions struct {
	Edit        Permission            `json:"edit"`
	Bash        map[string]Permission `json:"bash"`
	WebFetch    Permission            `json:"webFetch,omitempty"`
	ExternalDir Permission            `json:"externalDir,omitempty"`
}

type Permission string

const (
	PermissionAllow Permission = "allow"
	PermissionDeny  Permission = "deny"
	PermissionAsk   Permission = "ask"
)
