package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joss/urp/internal/audit"
)

// requireDB checks if database is connected and exits with error if not.
// Eliminates 67+ duplicate db-nil checks across commands.
func requireDB(event *audit.AuditEvent) {
	if db == nil {
		auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
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

// truncateStr truncates string to n characters with ellipsis.
// Consolidates 3 duplicate truncate functions.
func truncateStr(s string, n int) string {
	if n < 4 {
		n = 4
	}
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// urpPath returns a path under ~/.urp-go/
// Consolidates 5-6 duplicate home directory path constructions.
func urpPath(subdir ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	parts := append([]string{home, ".urp-go"}, subdir...)
	return filepath.Join(parts...)
}

// urpDataPath returns ~/.urp-go/data/ path for knowledge persistence.
func urpDataPath() string {
	return urpPath("data")
}

// urpBackupsPath returns ~/.urp-go/backups/ path.
func urpBackupsPath() string {
	return urpPath("backups")
}

// urpSkillsPath returns ~/.urp-go/skills/ path.
func urpSkillsPath() string {
	return urpPath("skills")
}

// urpEnvPath returns ~/.urp-go/.env path.
func urpEnvPath() string {
	return urpPath(".env")
}

// getCwd returns current working directory or "unknown".
func getCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return cwd
}
