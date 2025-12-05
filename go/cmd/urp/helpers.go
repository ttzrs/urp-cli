package main

import (
	"fmt"
	"os"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
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

// urpEnvPath returns ~/.urp-go/.env path.
func urpEnvPath() string {
	return config.GetPaths().EnvFile
}

// urpPath returns a path under ~/.urp-go/ using config.Path.
func urpPath(subdir ...string) string {
	return config.Path(subdir...)
}

// getCwd delegates to urpstrings.GetCwd for backward compatibility.
func getCwd() string {
	return urpstrings.GetCwd()
}
