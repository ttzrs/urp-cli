// Package memory provides session context and identity.
package memory

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"
)

// Context represents the identity of a URP session.
type Context struct {
	InstanceID       string   `json:"instance_id"`
	SessionID        string   `json:"session_id"`
	UserID           string   `json:"user_id"`
	Scope            string   `json:"scope"` // "session" | "instance" | "global"
	ContextSignature string   `json:"context_signature"`
	Path             string   `json:"path"` // Working directory path
	Project          string   `json:"project"`
	Tags             []string `json:"tags"`
	StartedAt        string   `json:"started_at"`
}

// NewContext creates a new context from environment variables.
func NewContext() *Context {
	// Detect project from URP_PROJECT or cwd
	project := os.Getenv("URP_PROJECT")
	if project == "" || project == "unknown" {
		cwd, _ := os.Getwd()
		project = detectProjectName(cwd)
	}

	// Instance = hostname (shared across sessions on same machine)
	instanceID := os.Getenv("URP_INSTANCE_ID")
	if instanceID == "" {
		hostname, _ := os.Hostname()
		instanceID = hostname
	}

	// Session = project + path hash (persists across urp invocations)
	sessionID := os.Getenv("URP_SESSION_ID")
	if sessionID == "" {
		cwd, _ := os.Getwd()
		sessionID = projectSessionID(project, cwd)
	}

	userID := os.Getenv("URP_USER_ID")
	if userID == "" {
		userID = "default"
	}

	// Build context signature
	ctxSig := os.Getenv("URP_CONTEXT_SIGNATURE")
	if ctxSig == "" {
		ctxSig = BuildSignature(
			project,
			os.Getenv("URP_DATASET"),
			os.Getenv("URP_BRANCH"),
			os.Getenv("URP_ENV"),
		)
	}

	// Build tags
	var tags []string
	if p := os.Getenv("URP_PROJECT"); p != "" {
		tags = append(tags, p)
	}
	if d := os.Getenv("URP_DATASET"); d != "" {
		tags = append(tags, d)
	}
	if e := os.Getenv("URP_ENV"); e != "" {
		tags = append(tags, e)
	}
	if len(tags) == 0 {
		tags = []string{"urp-cli", "local"}
	}

	cwd, _ := os.Getwd()

	return &Context{
		InstanceID:       instanceID,
		SessionID:        sessionID,
		UserID:           userID,
		Scope:            "session",
		ContextSignature: ctxSig,
		Path:             cwd,
		Project:          project,
		Tags:             tags,
		StartedAt:        time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildSignature creates a context signature from components.
// Example: 'urp-cli|UNSW-NB15|master|fedora-41'
func BuildSignature(project, dataset, branch, env string) string {
	if project == "" {
		project = "urp-cli"
	}
	if branch == "" {
		branch = "master"
	}
	if env == "" {
		env = "local"
	}

	parts := []string{project}
	if dataset != "" {
		parts = append(parts, dataset)
	}
	parts = append(parts, branch, env)
	return strings.Join(parts, "|")
}

// SignatureHash returns a short hash of the signature for IDs.
func SignatureHash(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return fmt.Sprintf("%x", h[:4])
}

// IsCompatible checks if two signatures are compatible.
// Strict: must match exactly. Loose: share project name.
func IsCompatible(sigA, sigB string, strict bool) bool {
	if strict {
		return sigA == sigB
	}

	partsA := strings.Split(sigA, "|")
	partsB := strings.Split(sigB, "|")

	if len(partsA) == 0 || len(partsB) == 0 {
		return false
	}

	return partsA[0] == partsB[0]
}

// detectProjectName extracts project name from path.
// Looks for git root or uses directory name.
func detectProjectName(path string) string {
	// Try to find .git directory going up
	dir := path
	for dir != "/" && dir != "." {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			// Found git root, use its name
			parts := strings.Split(dir, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
		// Go up one level
		lastSlash := strings.LastIndex(dir, "/")
		if lastSlash <= 0 {
			break
		}
		dir = dir[:lastSlash]
	}

	// Fallback: use last component of path
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return "unknown"
}

// projectSessionID creates a deterministic session ID.
// Prefers simple project name, adds path hash only on collision.
// Collision detection happens at runtime via checkSessionCollision().
func projectSessionID(project, path string) string {
	// Start with simple project name
	// Collision handling is done in session.go when writing to DB
	return project
}

// sessionIDWithPath creates a unique session ID when there's a name collision.
func sessionIDWithPath(project, path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s-%x", project, h[:4])
}
