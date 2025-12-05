package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joss/urp/internal/opencode/config"
	"github.com/joss/urp/internal/opencode/domain"
	urpstrings "github.com/joss/urp/internal/strings"
)

// Request represents a permission request
type Request struct {
	Tool    string
	Command string // for bash
	Path    string // for file ops
	Args    map[string]any
}

// Response from permission check
type Response struct {
	Allowed bool
	Reason  string
}

// Manager handles permission checks
type Manager struct {
	permissions domain.AgentPermissions
	workDir     string
	allowed     map[string]bool // cache of allowed patterns
	denied      map[string]bool // cache of denied patterns
	mu          sync.RWMutex    // protects allowed/denied maps
	persist     bool            // whether to persist to disk
}

func NewManager(perms domain.AgentPermissions, workDir string) *Manager {
	m := &Manager{
		permissions: perms,
		workDir:     workDir,
		allowed:     make(map[string]bool),
		denied:      make(map[string]bool),
		persist:     true,
	}
	m.load() // Load persisted permissions
	return m
}

// NewManagerWithoutPersistence creates a manager that doesn't persist to disk
// Useful for testing
func NewManagerWithoutPersistence(perms domain.AgentPermissions, workDir string) *Manager {
	return &Manager{
		permissions: perms,
		workDir:     workDir,
		allowed:     make(map[string]bool),
		denied:      make(map[string]bool),
		persist:     false,
	}
}

// Check returns permission status for a tool request
func (m *Manager) Check(req Request) (domain.Permission, string) {
	switch req.Tool {
	case "bash":
		return m.checkBash(req.Command)
	case "write", "edit":
		return m.checkEdit(req.Path)
	case "web_fetch":
		return m.checkWebFetch()
	case "read", "glob", "grep", "ls":
		return m.checkRead(req.Path)
	case "computer":
		return m.checkComputer(req.Args)
	default:
		return domain.PermissionAllow, ""
	}
}

func (m *Manager) checkBash(command string) (domain.Permission, string) {
	if m.permissions.Bash == nil {
		return domain.PermissionAsk, "bash command: " + urpstrings.Truncate(command, 60)
	}

	// Check specific patterns first, then wildcards
	for pattern, perm := range m.permissions.Bash {
		if pattern == "*" {
			continue
		}
		if matchCommand(command, pattern) {
			return perm, ""
		}
	}

	// Check wildcard
	if perm, ok := m.permissions.Bash["*"]; ok {
		return perm, "bash command: " + urpstrings.Truncate(command, 60)
	}

	return domain.PermissionAsk, "bash command: " + urpstrings.Truncate(command, 60)
}

func (m *Manager) checkEdit(path string) (domain.Permission, string) {
	// Check if path is outside work directory
	if !m.isInsideWorkDir(path) {
		if m.permissions.ExternalDir == "" {
			return domain.PermissionAsk, "edit file outside project: " + path
		}
		return m.permissions.ExternalDir, "edit file outside project: " + path
	}

	return m.permissions.Edit, "edit file: " + path
}

func (m *Manager) checkRead(path string) (domain.Permission, string) {
	// Reading is generally allowed, but external files may need permission
	if !m.isInsideWorkDir(path) {
		if m.permissions.ExternalDir == "" {
			return domain.PermissionAllow, "" // reading external is usually ok
		}
		return m.permissions.ExternalDir, ""
	}
	return domain.PermissionAllow, ""
}

func (m *Manager) checkWebFetch() (domain.Permission, string) {
	if m.permissions.WebFetch == "" {
		return domain.PermissionAsk, "fetch from web"
	}
	return m.permissions.WebFetch, ""
}

func (m *Manager) isInsideWorkDir(path string) bool {
	if path == "" {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absWorkDir, err := filepath.Abs(m.workDir)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absWorkDir)
}

// Allow permanently allows a pattern
func (m *Manager) Allow(pattern string) {
	m.mu.Lock()
	m.allowed[pattern] = true
	delete(m.denied, pattern)
	m.mu.Unlock()
	m.save()
}

// Deny permanently denies a pattern
func (m *Manager) Deny(pattern string) {
	m.mu.Lock()
	m.denied[pattern] = true
	delete(m.allowed, pattern)
	m.mu.Unlock()
	m.save()
}

// IsAllowed checks if a pattern was previously allowed
func (m *Manager) IsAllowed(pattern string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.allowed[pattern]
}

// IsDenied checks if a pattern was previously denied
func (m *Manager) IsDenied(pattern string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.denied[pattern]
}

// persistedData is the structure saved to disk
type persistedData struct {
	Allowed []string `json:"allowed"`
	Denied  []string `json:"denied"`
}

// permissionFile returns the path to the permissions file
func permissionFile() string {
	return filepath.Join(config.DataDir(), "permissions.json")
}

// save persists the current permissions to disk
func (m *Manager) save() {
	if !m.persist {
		return
	}

	m.mu.RLock()
	data := persistedData{
		Allowed: make([]string, 0, len(m.allowed)),
		Denied:  make([]string, 0, len(m.denied)),
	}
	for pattern := range m.allowed {
		data.Allowed = append(data.Allowed, pattern)
	}
	for pattern := range m.denied {
		data.Denied = append(data.Denied, pattern)
	}
	m.mu.RUnlock()

	path := permissionFile()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return // Silent fail - permissions still work in memory
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, jsonData, 0600) // 0600 for privacy
}

// load reads persisted permissions from disk
func (m *Manager) load() {
	if !m.persist {
		return
	}

	path := permissionFile()
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return // File doesn't exist or can't be read
	}

	var data persistedData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pattern := range data.Allowed {
		m.allowed[pattern] = true
	}
	for _, pattern := range data.Denied {
		m.denied[pattern] = true
	}
}

// Clear removes all persisted permissions
func (m *Manager) Clear() {
	m.mu.Lock()
	m.allowed = make(map[string]bool)
	m.denied = make(map[string]bool)
	m.mu.Unlock()
	m.save()
}

// AllowedPatterns returns a copy of all allowed patterns
func (m *Manager) AllowedPatterns() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	patterns := make([]string, 0, len(m.allowed))
	for p := range m.allowed {
		patterns = append(patterns, p)
	}
	return patterns
}

// DeniedPatterns returns a copy of all denied patterns
func (m *Manager) DeniedPatterns() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	patterns := make([]string, 0, len(m.denied))
	for p := range m.denied {
		patterns = append(patterns, p)
	}
	return patterns
}

// matchCommand checks if a command matches a pattern
func matchCommand(command, pattern string) bool {
	// Handle wildcard patterns like "git *", "ls *"
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return strings.HasPrefix(command, prefix+" ") || command == prefix
	}
	// Exact match
	return command == pattern
}

// checkComputer handles permission for computer interaction tool
// Safe actions (mouse_position, screenshot) are allowed
// Dangerous actions (click, type, key, etc.) always require confirmation
func (m *Manager) checkComputer(args map[string]any) (domain.Permission, string) {
	action, _ := args["action"].(string)

	// Safe read-only actions
	switch action {
	case "mouse_position", "screenshot":
		return domain.PermissionAllow, ""
	}

	// All other actions are dangerous and require explicit permission
	reason := "computer interaction: " + action
	if x, ok := args["x"].(float64); ok {
		if y, ok := args["y"].(float64); ok {
			reason += fmt.Sprintf(" at (%d,%d)", int(x), int(y))
		}
	}
	if text, ok := args["text"].(string); ok && text != "" {
		preview := text
		if len(preview) > 20 {
			preview = preview[:20] + "..."
		}
		reason += fmt.Sprintf(" text=%q", preview)
	}

	return domain.PermissionAsk, reason
}
