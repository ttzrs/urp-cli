package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/session"
	urpstrings "github.com/joss/urp/internal/strings"
)

// requireDB checks if database is connected and exits with error if not.
// Use when audit event is available for logging.
func requireDB(event *audit.AuditEvent) {
	if db == nil {
		auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
		fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
		os.Exit(1)
	}
}

// requireDBSimple checks if database is connected and exits with error if not.
// Use when no audit event is available. Eliminates 40+ duplicate checks.
func requireDBSimple() {
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
		os.Exit(1)
	}
}

// exitOnError logs error to audit and stderr, then exits.
// Eliminates 30+ duplicate error handling patterns.
func exitOnError(event *audit.AuditEvent, err error) {
	auditLogger.LogError(event, err)
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// truncateStr delegates to urpstrings.Truncate for backward compatibility.
func truncateStr(s string, n int) string {
	return urpstrings.Truncate(s, n)
}

// urpDataPath returns ~/.urp-go/data/ path for knowledge persistence.
func urpDataPath() string {
	return config.GetPaths().Data
}

// urpBackupsPath returns ~/.urp-go/backups/ path.
func urpBackupsPath() string {
	return config.GetPaths().Backups
}

// urpSkillsPath returns ~/.urp-go/skills/ path.
func urpSkillsPath() string {
	return config.GetPaths().Skills
}

// urpPath returns a path under ~/.urp-go/ using config.Path.
func urpPath(subdir ...string) string {
	return config.Path(subdir...)
}

// getCwd delegates to urpstrings.GetCwd for backward compatibility.
func getCwd() string {
	return urpstrings.GetCwd()
}

// --- Session helpers (eliminate 12 graphstore+session duplicates) ---

// getSessionManager returns a session manager, requires db connection.
// Eliminates 12 duplicate `graphstore.New(db)` + `session.NewManager(store)` patterns.
func getSessionManager() *session.Manager {
	requireDBSimple()
	store := graphstore.New(db)
	return session.NewManager(store)
}

// --- JSON helpers (eliminate 13 json.MarshalIndent duplicates) ---

// prettyJSON marshals v with consistent indentation.
// Eliminates 13 duplicate `json.MarshalIndent(v, "", "  ")` patterns.
func prettyJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// printJSON marshals and prints v with consistent formatting.
func printJSON(v any) error {
	data, err := prettyJSON(v)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// --- Error helpers (for cases without audit event) ---

// fatalError prints error to stderr and exits.
// Use when no audit event is available.
func fatalError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// fatalErrorf prints formatted error to stderr and exits.
func fatalErrorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
