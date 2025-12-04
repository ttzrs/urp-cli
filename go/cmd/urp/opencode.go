package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/session"
)

func opencodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oc",
		Short: "OpenCode working memory and sessions",
		Long: `OpenCode integration for working memory management.

Sessions persist in the graph database, enabling:
- Conversation history across AI sessions
- Token usage tracking
- Context compaction for long conversations`,
	}

	cmd.AddCommand(
		ocSessionCmd(),
		ocMessageCmd(),
		ocUsageCmd(),
	)

	return cmd
}

// --- Session commands ---

func ocSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage OpenCode sessions",
	}

	// oc session list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			dir, _ := os.Getwd()
			sessions, err := mgr.List(context.Background(), dir, 20)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			if len(sessions) == 0 {
				fmt.Println("No sessions found")
				return
			}

			fmt.Printf("%-26s %-30s %-20s\n", "ID", "TITLE", "UPDATED")
			for _, s := range sessions {
				fmt.Printf("%-26s %-30s %-20s\n",
					s.ID[:26],
					truncate(s.Title, 30),
					s.UpdatedAt.Format("2006-01-02 15:04"),
				)
			}
		},
	}

	// oc session new [title]
	newCmd := &cobra.Command{
		Use:   "new [title]",
		Short: "Create a new session",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			dir, _ := os.Getwd()
			sess, err := mgr.Create(context.Background(), dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			if len(args) > 0 {
				sess.Title = args[0]
				mgr.Update(context.Background(), sess)
			}

			fmt.Printf("Created session: %s\n", sess.ID)
		},
	}

	// oc session show <id>
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show session details",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			sess, err := mgr.Get(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Printf("ID:        %s\n", sess.ID)
			fmt.Printf("Project:   %s\n", sess.ProjectID)
			fmt.Printf("Directory: %s\n", sess.Directory)
			fmt.Printf("Title:     %s\n", sess.Title)
			fmt.Printf("Created:   %s\n", sess.CreatedAt.Format(time.RFC3339))
			fmt.Printf("Updated:   %s\n", sess.UpdatedAt.Format(time.RFC3339))
			if sess.ParentID != "" {
				fmt.Printf("Parent:    %s\n", sess.ParentID)
			}
			if sess.Summary != nil {
				fmt.Printf("Changes:   +%d -%d (%d files)\n",
					sess.Summary.Additions, sess.Summary.Deletions, len(sess.Summary.Files))
			}

			// Message count
			messages, _ := mgr.GetMessages(context.Background(), sess.ID)
			fmt.Printf("Messages:  %d\n", len(messages))
		},
	}

	// oc session fork <id>
	forkCmd := &cobra.Command{
		Use:   "fork <id>",
		Short: "Fork a session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			forked, err := mgr.Fork(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Printf("Forked session: %s\n", forked.ID)
		},
	}

	// oc session delete <id>
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			if err := mgr.Delete(context.Background(), args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Println("Deleted")
		},
	}

	cmd.AddCommand(listCmd, newCmd, showCmd, forkCmd, deleteCmd)
	return cmd
}

// --- Message commands ---

func ocMessageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "msg",
		Short: "Manage session messages",
	}

	// oc msg list <session-id>
	listCmd := &cobra.Command{
		Use:   "list <session-id>",
		Short: "List messages in a session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			messages, err := mgr.GetMessages(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			for _, m := range messages {
				fmt.Printf("[%s] %s\n", m.Role, m.Timestamp.Format("15:04:05"))
				for _, p := range m.Parts {
					switch pt := p.(type) {
					case domain.TextPart:
						fmt.Printf("  %s\n", truncate(pt.Text, 100))
					case domain.ToolCallPart:
						fmt.Printf("  🔧 %s\n", pt.Name)
					case domain.ReasoningPart:
						fmt.Printf("  💭 %s\n", truncate(pt.Text, 50))
					}
				}
			}
		},
	}

	// oc msg add <session-id> <role> <text>
	addCmd := &cobra.Command{
		Use:   "add <session-id> <role> <text>",
		Short: "Add a message to session",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			msg := &domain.Message{
				ID:        ulid.Make().String(),
				SessionID: args[0],
				Role:      domain.Role(args[1]),
				Parts:     []domain.Part{domain.TextPart{Text: args[2]}},
				Timestamp: time.Now(),
			}

			if err := mgr.AddMessage(context.Background(), msg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Printf("Added message: %s\n", msg.ID)
		},
	}

	cmd.AddCommand(listCmd, addCmd)
	return cmd
}

// --- Usage commands ---

func ocUsageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Token usage statistics",
	}

	// oc usage session <session-id>
	sessionCmd := &cobra.Command{
		Use:   "session <session-id>",
		Short: "Show session usage",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			usage, err := mgr.GetUsage(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Printf("Session:  %s\n", usage.SessionID)
			fmt.Printf("Provider: %s\n", usage.ProviderID)
			fmt.Printf("Model:    %s\n", usage.ModelID)
			fmt.Printf("Input:    %s tokens ($%.4f)\n",
				domain.FormatTokens(usage.Usage.InputTokens), usage.Usage.InputCost)
			fmt.Printf("Output:   %s tokens ($%.4f)\n",
				domain.FormatTokens(usage.Usage.OutputTokens), usage.Usage.OutputCost)
			fmt.Printf("Total:    %s\n", domain.FormatCost(usage.Usage.TotalCost))

			if usage.Usage.CacheRead > 0 || usage.Usage.CacheWrite > 0 {
				fmt.Printf("Cache:    %s read, %s write\n",
					domain.FormatTokens(usage.Usage.CacheRead),
					domain.FormatTokens(usage.Usage.CacheWrite))
			}
		},
	}

	// oc usage total
	totalCmd := &cobra.Command{
		Use:   "total",
		Short: "Show total usage across all sessions",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Database not connected")
				return
			}

			store := graphstore.New(db)
			mgr := session.NewManager(store)

			usage, err := mgr.GetTotalUsage(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}

			fmt.Printf("Total Usage\n")
			fmt.Printf("-----------\n")
			fmt.Printf("Input:    %s tokens ($%.4f)\n",
				domain.FormatTokens(usage.InputTokens), usage.InputCost)
			fmt.Printf("Output:   %s tokens ($%.4f)\n",
				domain.FormatTokens(usage.OutputTokens), usage.OutputCost)
			fmt.Printf("Total:    %s\n", domain.FormatCost(usage.TotalCost))

			if usage.CacheRead > 0 || usage.CacheWrite > 0 {
				fmt.Printf("Cache:    %s read, %s write\n",
					domain.FormatTokens(usage.CacheRead),
					domain.FormatTokens(usage.CacheWrite))
			}
		},
	}

	cmd.AddCommand(sessionCmd, totalCmd)
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
