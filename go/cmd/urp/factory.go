package main

import (
	"github.com/joss/urp/internal/audit"
	"github.com/spf13/cobra"
)

// CommandFunc defines the function signature for command execution.
type CommandFunc func(cmd *cobra.Command, args []string) error

// CommandConfig holds configuration for creating standardized commands.
type CommandConfig struct {
	Use        string
	Short      string
	Long       string
	Args       cobra.PositionalArgs
	Category   audit.Category
	Action     string
	RunFunc    CommandFunc
	PreRun     func(cmd *cobra.Command, args []string)
	PostRun    func(cmd *cobra.Command, args []string)
	Example    string
	Aliases    []string
	Deprecated string
}

// newCommand creates a standardized Cobra command with audit logging and error handling.
// Eliminates boilerplate across 40+ commands.
func newCommand(cfg CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:        cfg.Use,
		Short:      cfg.Short,
		Long:       cfg.Long,
		Args:       cfg.Args,
		Example:    cfg.Example,
		Aliases:    cfg.Aliases,
		Deprecated: cfg.Deprecated,
		PreRun: func(cmd *cobra.Command, args []string) {
			if cfg.PreRun != nil {
				cfg.PreRun(cmd, args)
			}
			// Default pre-run: ensure database is available
			requireDBSimple()
		},
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(cfg.Category, cfg.Action)

			if err := cfg.RunFunc(cmd, args); err != nil {
				exitOnError(event, err)
				return
			}

			auditLogger.LogSuccess(event)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			if cfg.PostRun != nil {
				cfg.PostRun(cmd, args)
			}
		},
	}

	return cmd
}

// newSimpleCommand creates a command without database requirement.
// Use for commands that don't need graph connection.
func newSimpleCommand(cfg CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:        cfg.Use,
		Short:      cfg.Short,
		Long:       cfg.Long,
		Args:       cfg.Args,
		Example:    cfg.Example,
		Aliases:    cfg.Aliases,
		Deprecated: cfg.Deprecated,
		PreRun: func(cmd *cobra.Command, args []string) {
			if cfg.PreRun != nil {
				cfg.PreRun(cmd, args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(cfg.Category, cfg.Action)

			if err := cfg.RunFunc(cmd, args); err != nil {
				exitOnError(event, err)
				return
			}

			auditLogger.LogSuccess(event)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			if cfg.PostRun != nil {
				cfg.PostRun(cmd, args)
			}
		},
	}

	return cmd
}

// newOptionalDBCommand creates a command that works with or without database.
// Use for commands that have enhanced functionality with DB but can work offline.
func newOptionalDBCommand(cfg CommandConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:        cfg.Use,
		Short:      cfg.Short,
		Long:       cfg.Long,
		Args:       cfg.Args,
		Example:    cfg.Example,
		Aliases:    cfg.Aliases,
		Deprecated: cfg.Deprecated,
		PreRun: func(cmd *cobra.Command, args []string) {
			if cfg.PreRun != nil {
				cfg.PreRun(cmd, args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(cfg.Category, cfg.Action)

			if err := cfg.RunFunc(cmd, args); err != nil {
				exitOnError(event, err)
				return
			}

			auditLogger.LogSuccess(event)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			if cfg.PostRun != nil {
				cfg.PostRun(cmd, args)
			}
		},
	}

	return cmd
}
