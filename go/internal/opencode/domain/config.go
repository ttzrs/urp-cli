package domain

// Config represents the application configuration
type Config struct {
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Agent      string            `json:"agent"`
	APIKeys    map[string]string `json:"apiKeys,omitempty"`
	MCPServers []MCPServer       `json:"mcpServers,omitempty"`
}

type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
