// Package main terminal events and session commands.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
)

func eventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Terminal event commands",
		Long:  "Query and manage terminal events (τ + Φ primitives)",
	}

	// urp events run <cmd>
	runCmd := &cobra.Command{
		Use:   "run [command...]",
		Short: "Execute command and log to graph",
		Long:  "Run a command transparently, logging execution to the knowledge graph",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.StartWithCommand(audit.CategoryEvents, "run", strings.Join(args, " "))

			executor := runner.NewExecutor(db)
			result := executor.Run(context.Background(), args)

			event.ExitCode = result.ExitCode
			event.OutputSize = len(result.Stdout) + len(result.Stderr)
			if result.ExitCode != 0 {
				event.ErrorMessage = result.Stderr
				auditLogger.LogError(event, fmt.Errorf("exit code %d", result.ExitCode))
			} else {
				auditLogger.LogSuccess(event)
			}
			os.Exit(result.ExitCode)
		},
	}

	// urp events list
	var limit int
	var project string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Show recent commands",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryEvents, "list")

			requireDB(event)

			store := runner.NewEventStore(db)
			events, err := store.ListRecent(context.Background(), limit, project)
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			r := render.New(pretty)
			fmt.Print(r.Events(events))
		},
	}
	listCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Number of events to show")
	listCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	// urp events errors
	var minutes int
	errorsCmd := &cobra.Command{
		Use:   "errors",
		Short: "Show recent errors (pain)",
		Long:  "Show recent command failures (⊥ conflicts)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryEvents, "errors")

			requireDB(event)

			store := runner.NewEventStore(db)
			conflicts, err := store.ListErrors(context.Background(), minutes, project)
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			r := render.New(pretty)
			title := fmt.Sprintf("Errors in last %d minutes", minutes)
			fmt.Print(r.Errors(conflicts, title))
		},
	}
	errorsCmd.Flags().IntVarP(&minutes, "minutes", "m", 5, "Look back N minutes")
	errorsCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	cmd.AddCommand(runCmd, listCmd, errorsCmd)
	return cmd
}

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Session management",
	}

	// urp session id
	idCmd := &cobra.Command{
		Use:   "id",
		Short: "Show current session identity",
		Run: func(cmd *cobra.Command, args []string) {
			project := config.Env().Project
			if project == "" {
				project = "unknown"
			}
			hostname, _ := os.Hostname()

			fmt.Println("IDENTITY / CONTEXT")
			fmt.Println()
			fmt.Printf("  Project:  %s\n", project)
			fmt.Printf("  Host:     %s\n", hostname)
			fmt.Printf("  CWD:      %s\n", getCwd())
		},
	}

	cmd.AddCommand(idCmd)
	return cmd
}
