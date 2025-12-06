package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/query"
)

func gitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git history commands",
		Long:  "Load and query git history (τ primitive)",
	}

	// urp git ingest [path]
	var maxCommits int
	ingestCmd := &cobra.Command{
		Use:   "ingest [path]",
		Short: "Load git history into graph",
		Long:  "Load git history into graph. If no path is provided, uses current directory.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryGit, "ingest")

			requireDB(event)

			// Default to current directory
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			loader := ingest.NewGitLoader(db, path)
			stats, err := loader.LoadHistory(context.Background(), maxCommits)
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(stats)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}
	ingestCmd.Flags().IntVarP(&maxCommits, "max", "m", 500, "Max commits to load")

	// urp git history <file>
	var limit int
	historyCmd := &cobra.Command{
		Use:   "history <file>",
		Short: "Show file change history",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryGit, "history")

			requireDB(event)

			q := query.NewQuerier(db)
			history, err := q.GetHistory(context.Background(), args[0], limit)
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(history)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}
	historyCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Max commits")

	cmd.AddCommand(ingestCmd, historyCmd, linkCmd())
	return cmd
}

// urp git link
// Generates co-evolution weights
func linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link",
		Short: "Generate co-evolution weights",
		Long:  "Analyzes commit history to link files that change together.",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryGit, "link")
			requireDB(event)

			loader := ingest.NewGitLoader(db, ".")
			if err := loader.GenerateCoEvolutionWeights(context.Background()); err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)
			fmt.Println("✓ Co-evolution weights generated successfully.")
		},
	}
}
