package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/backup"
)

func backupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "backup",
		Aliases: []string{"bak"},
		Short:   "Knowledge backup and restore",
		Long: `Backup and restore knowledge from the graph database.

Knowledge types:
  solutions  - Learned solutions from urp think learn
  memories   - Session memories from urp mem
  knowledge  - Persistent knowledge from urp kb
  skills     - Registered skills from urp skill
  sessions   - OpenCode sessions and messages
  vectors    - Vector embeddings
  all        - All of the above

Examples:
  urp backup export                    # Export all to timestamped file
  urp backup export -o brain.tar.gz    # Export all to specific file
  urp backup export -t solutions,skills # Export only solutions and skills
  urp backup import brain.tar.gz       # Import all from backup
  urp backup import brain.tar.gz -t solutions --merge  # Merge solutions only
  urp backup list brain.tar.gz         # Show backup contents`,
	}

	cmd.AddCommand(
		backupExportCmd(),
		backupImportCmd(),
		backupListCmd(),
		backupStatsCmd(),
	)

	return cmd
}

func backupExportCmd() *cobra.Command {
	var (
		output      string
		types       []string
		description string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export knowledge to compressed backup",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "backup.export")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			// Default output path
			if output == "" {
				home, _ := os.UserHomeDir()
				dataDir := filepath.Join(home, ".urp-go", "backups")
				os.MkdirAll(dataDir, 0755)
				output = filepath.Join(dataDir, fmt.Sprintf("urp-backup-%s.tar.gz", time.Now().Format("20060102-150405")))
			}

			// Parse types
			knowledgeTypes := []backup.KnowledgeType{}
			if len(types) == 0 {
				knowledgeTypes = append(knowledgeTypes, backup.TypeAll)
			} else {
				for _, t := range types {
					knowledgeTypes = append(knowledgeTypes, backup.KnowledgeType(t))
				}
			}

			// Get data dir
			home, _ := os.UserHomeDir()
			dataDir := filepath.Join(home, ".urp-go", "data")

			mgr := backup.NewBackupManager(db, dataDir)
			meta, err := mgr.Export(context.Background(), knowledgeTypes, output, description)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

			fmt.Printf("Backup created: %s\n", output)
			fmt.Println("\nContents:")
			for t, count := range meta.Counts {
				fmt.Printf("  %-12s %d items\n", t+":", count)
			}

			// Get file size
			if info, err := os.Stat(output); err == nil {
				fmt.Printf("\nSize: %s\n", formatSize(info.Size()))
			}
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path")
	cmd.Flags().StringSliceVarP(&types, "types", "t", nil, "Types to export (solutions,memories,knowledge,skills,sessions,vectors,all)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Backup description")

	return cmd
}

func backupImportCmd() *cobra.Command {
	var (
		types []string
		merge bool
	)

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Restore knowledge from backup",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "backup.import")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			// Parse types
			knowledgeTypes := []backup.KnowledgeType{}
			for _, t := range types {
				knowledgeTypes = append(knowledgeTypes, backup.KnowledgeType(t))
			}

			home, _ := os.UserHomeDir()
			dataDir := filepath.Join(home, ".urp-go", "data")

			mgr := backup.NewBackupManager(db, dataDir)
			meta, err := mgr.Import(context.Background(), args[0], knowledgeTypes, merge)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

			mode := "replaced"
			if merge {
				mode = "merged"
			}

			fmt.Printf("Backup restored (%s)\n", mode)
			fmt.Printf("From: %s\n", args[0])
			fmt.Printf("Created: %s\n", meta.CreatedAt.Format(time.RFC3339))
			if meta.Description != "" {
				fmt.Printf("Description: %s\n", meta.Description)
			}
			fmt.Println("\nRestored:")
			for t, count := range meta.Counts {
				fmt.Printf("  %-12s %d items\n", t+":", count)
			}
		},
	}

	cmd.Flags().StringSliceVarP(&types, "types", "t", nil, "Types to import (default: all from backup)")
	cmd.Flags().BoolVar(&merge, "merge", false, "Merge with existing data instead of replacing")

	return cmd
}

func backupListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <file>",
		Short: "Show backup contents without importing",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			home, _ := os.UserHomeDir()
			dataDir := filepath.Join(home, ".urp-go", "data")

			mgr := backup.NewBackupManager(nil, dataDir)
			meta, err := mgr.List(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("BACKUP CONTENTS")
			fmt.Println(strings.Repeat("─", 40))
			fmt.Printf("Version:     %s\n", meta.Version)
			fmt.Printf("Created:     %s\n", meta.CreatedAt.Format(time.RFC3339))
			fmt.Printf("Project:     %s\n", meta.Project)
			if meta.Description != "" {
				fmt.Printf("Description: %s\n", meta.Description)
			}
			fmt.Printf("Types:       %v\n", meta.Types)
			fmt.Println("\nCounts:")
			total := 0
			for t, count := range meta.Counts {
				fmt.Printf("  %-12s %d items\n", t+":", count)
				total += count
			}
			fmt.Printf("\nTotal: %d items\n", total)

			// Get file size
			if info, err := os.Stat(args[0]); err == nil {
				fmt.Printf("Size: %s\n", formatSize(info.Size()))
			}
		},
	}
}

func backupStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show current knowledge statistics",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "backup.stats")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := context.Background()
			stats := make(map[string]int)

			queries := map[string]string{
				"solutions": "MATCH (s:Solution) RETURN count(s) as count",
				"memories":  "MATCH (m:Memo) RETURN count(m) as count",
				"knowledge": "MATCH (k:KnowledgeEntry) RETURN count(k) as count",
				"skills":    "MATCH (sk:Skill) RETURN count(sk) as count",
				"sessions":  "MATCH (s:Session:OpenCode) RETURN count(s) as count",
				"messages":  "MATCH (m:Message:OpenCode) RETURN count(m) as count",
				"vectors":   "MATCH (v:Vector) RETURN count(v) as count",
			}

			for name, query := range queries {
				records, err := db.Execute(ctx, query, nil)
				if err == nil && len(records) > 0 {
					if count, ok := records[0]["count"].(int64); ok {
						stats[name] = int(count)
					}
				}
			}

			auditLogger.LogSuccess(event)

			fmt.Println("KNOWLEDGE STATISTICS")
			fmt.Println(strings.Repeat("─", 40))

			total := 0
			for name, count := range stats {
				fmt.Printf("  %-12s %d\n", name+":", count)
				total += count
			}
			fmt.Printf("\n  Total:       %d items\n", total)

			out, _ := json.MarshalIndent(stats, "", "  ")
			_ = out // Available for --json flag
		},
	}
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
