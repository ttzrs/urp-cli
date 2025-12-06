package permission

import (
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/stretchr/testify/assert"
)

// --- Check Tests ---

func TestCheckBashAllow(t *testing.T) {
	perms := domain.AgentPermissions{
		Bash: map[string]domain.Permission{
			"git *": domain.PermissionAllow,
			"ls *":  domain.PermissionAllow,
			"*":     domain.PermissionAsk,
		},
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	// Allowed patterns
	perm, _ := m.Check(Request{Tool: "bash", Command: "git status"})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "bash", Command: "git push"})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "bash", Command: "ls -la"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

func TestCheckBashAsk(t *testing.T) {
	perms := domain.AgentPermissions{
		Bash: map[string]domain.Permission{
			"git *": domain.PermissionAllow,
			"*":     domain.PermissionAsk,
		},
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	// Should ask for unknown commands
	perm, reason := m.Check(Request{Tool: "bash", Command: "rm -rf /"})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "rm -rf")
}

func TestCheckBashDeny(t *testing.T) {
	perms := domain.AgentPermissions{
		Bash: map[string]domain.Permission{
			"rm -rf *": domain.PermissionDeny,
			"*":        domain.PermissionAllow,
		},
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	// Denied patterns take precedence
	perm, _ := m.Check(Request{Tool: "bash", Command: "rm -rf /tmp"})
	assert.Equal(t, domain.PermissionDeny, perm)

	// But other commands are allowed
	perm, _ = m.Check(Request{Tool: "bash", Command: "echo hello"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

func TestCheckBashNilMap(t *testing.T) {
	perms := domain.AgentPermissions{
		Bash: nil,
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, reason := m.Check(Request{Tool: "bash", Command: "any command"})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "any command")
}

// --- Edit Tests ---

func TestCheckEditInsideWorkDir(t *testing.T) {
	perms := domain.AgentPermissions{
		Edit: domain.PermissionAllow,
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, _ := m.Check(Request{Tool: "edit", Path: "/workspace/file.go"})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "write", Path: "/workspace/subdir/file.go"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

func TestCheckEditOutsideWorkDir(t *testing.T) {
	perms := domain.AgentPermissions{
		Edit:        domain.PermissionAllow,
		ExternalDir: domain.PermissionAsk,
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, reason := m.Check(Request{Tool: "edit", Path: "/etc/passwd"})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "outside project")
}

func TestCheckEditExternalAllowed(t *testing.T) {
	perms := domain.AgentPermissions{
		Edit:        domain.PermissionAllow,
		ExternalDir: domain.PermissionAllow,
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, _ := m.Check(Request{Tool: "edit", Path: "/tmp/file.txt"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

// --- Read Tests ---

func TestCheckRead(t *testing.T) {
	perms := domain.AgentPermissions{}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	// Reading is generally allowed
	perm, _ := m.Check(Request{Tool: "read", Path: "/workspace/file.go"})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "glob", Path: "/workspace"})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "grep", Path: "/workspace"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

// --- WebFetch Tests ---

func TestCheckWebFetch(t *testing.T) {
	perms := domain.AgentPermissions{
		WebFetch: domain.PermissionAllow,
	}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, _ := m.Check(Request{Tool: "web_fetch"})
	assert.Equal(t, domain.PermissionAllow, perm)
}

func TestCheckWebFetchDefault(t *testing.T) {
	perms := domain.AgentPermissions{}
	m := NewManagerWithoutPersistence(perms, "/workspace")

	perm, reason := m.Check(Request{Tool: "web_fetch"})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "web")
}

// --- Computer Tests ---

func TestCheckComputerSafeActions(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	// Safe actions should be allowed
	perm, _ := m.Check(Request{Tool: "computer", Args: map[string]any{"action": "mouse_position"}})
	assert.Equal(t, domain.PermissionAllow, perm)

	perm, _ = m.Check(Request{Tool: "computer", Args: map[string]any{"action": "screenshot"}})
	assert.Equal(t, domain.PermissionAllow, perm)
}

func TestCheckComputerDangerousActions(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	// Dangerous actions should ask
	perm, reason := m.Check(Request{Tool: "computer", Args: map[string]any{"action": "click", "x": 100.0, "y": 200.0}})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "click")
	assert.Contains(t, reason, "100")

	perm, reason = m.Check(Request{Tool: "computer", Args: map[string]any{"action": "type", "text": "hello world"}})
	assert.Equal(t, domain.PermissionAsk, perm)
	assert.Contains(t, reason, "type")
	assert.Contains(t, reason, "hello")
}

// --- Allow/Deny Tests ---

func TestAllowDeny(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	// Initially nothing is allowed/denied
	assert.False(t, m.IsAllowed("git push"))
	assert.False(t, m.IsDenied("rm -rf"))

	// Allow a pattern
	m.Allow("git push")
	assert.True(t, m.IsAllowed("git push"))
	assert.False(t, m.IsDenied("git push"))

	// Deny a pattern
	m.Deny("rm -rf")
	assert.True(t, m.IsDenied("rm -rf"))
	assert.False(t, m.IsAllowed("rm -rf"))

	// Allow overrides deny
	m.Allow("rm -rf")
	assert.True(t, m.IsAllowed("rm -rf"))
	assert.False(t, m.IsDenied("rm -rf"))
}

func TestClear(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	m.Allow("pattern1")
	m.Deny("pattern2")
	assert.True(t, m.IsAllowed("pattern1"))
	assert.True(t, m.IsDenied("pattern2"))

	m.Clear()
	assert.False(t, m.IsAllowed("pattern1"))
	assert.False(t, m.IsDenied("pattern2"))
}

func TestAllowedDeniedPatterns(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	m.Allow("pattern1")
	m.Allow("pattern2")
	m.Deny("pattern3")

	allowed := m.AllowedPatterns()
	assert.Len(t, allowed, 2)
	assert.Contains(t, allowed, "pattern1")
	assert.Contains(t, allowed, "pattern2")

	denied := m.DeniedPatterns()
	assert.Len(t, denied, 1)
	assert.Contains(t, denied, "pattern3")
}

// --- Helper Tests ---

func TestMatchCommand(t *testing.T) {
	tests := []struct {
		command string
		pattern string
		want    bool
	}{
		{"git status", "git *", true},
		{"git push origin main", "git *", true},
		{"git", "git *", true},
		{"gitx status", "git *", false},
		{"ls -la", "ls *", true},
		{"echo hello", "echo hello", true},
		{"echo world", "echo hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.command+"_"+tt.pattern, func(t *testing.T) {
			got := matchCommand(tt.command, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsInsideWorkDir(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	assert.True(t, m.isInsideWorkDir("/workspace/file.go"))
	assert.True(t, m.isInsideWorkDir("/workspace/sub/dir/file.go"))
	assert.True(t, m.isInsideWorkDir("")) // empty path is considered inside
	assert.False(t, m.isInsideWorkDir("/etc/passwd"))
	assert.False(t, m.isInsideWorkDir("/tmp/file.txt"))
}

// --- Default Tool Tests ---

func TestCheckDefaultTool(t *testing.T) {
	m := NewManagerWithoutPersistence(domain.AgentPermissions{}, "/workspace")

	// Unknown tools default to allow
	perm, _ := m.Check(Request{Tool: "unknown_tool"})
	assert.Equal(t, domain.PermissionAllow, perm)
}
