package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/memory"
)

func memCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "Session memory commands",
		Long:  "Private cognitive space for the current session",
	}

	// Get the context
	getCtx := func() *memory.Context {
		return memory.NewContext()
	}

	// urp mem add <text>
	var kind string
	var importance int
	addCmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Add a note to session memory",
		Long:  "Remember something for this session (note, decision, observation)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			id, err := mem.Add(context.Background(), args[0], kind, importance, nil)
			if err != nil {
				fatalError(err)
			}

			fmt.Printf("Remembered: %s\n", id)
			fmt.Printf("  Kind: %s, Importance: %d\n", kind, importance)
		},
	}
	addCmd.Flags().StringVarP(&kind, "kind", "k", "note", "Memory type (note|decision|summary|observation)")
	addCmd.Flags().IntVarP(&importance, "importance", "i", 2, "Importance 1-5")

	// urp mem recall <query>
	var limit int
	recallCmd := &cobra.Command{
		Use:   "recall <query>",
		Short: "Search session memories",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			results, err := mem.Recall(context.Background(), args[0], limit, "", 1)
			if err != nil {
				fatalError(err)
			}

			if len(results) == 0 {
				fmt.Println("No matching memories found")
				return
			}

			fmt.Println("RECALL: Matching memories")
			for _, m := range results {
				fmt.Printf("  [%.0f%%] [%s] %s\n", m.Similarity*100, m.Kind, truncateStr(m.Text, 60))
			}
		},
	}
	recallCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")

	// urp mem list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all session memories",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			results, err := mem.List(context.Background())
			if err != nil {
				fatalError(err)
			}

			if len(results) == 0 {
				fmt.Println("No memories in this session")
				return
			}

			fmt.Printf("MEMORIES: %d items\n", len(results))
			for _, m := range results {
				fmt.Printf("  %s [%s] %s\n", m.MemoryID, m.Kind, truncateStr(m.Text, 50))
			}
		},
	}

	// urp mem stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show session memory statistics",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			stats, err := mem.Stats(context.Background())
			if err != nil {
				fatalError(err)
			}

			printJSON(stats)
		},
	}

	// urp mem clear
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all session memories",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			count, err := mem.Clear(context.Background())
			if err != nil {
				fatalError(err)
			}

			fmt.Printf("Cleared %d memories\n", count)
		},
	}

	cmd.AddCommand(addCmd, recallCmd, listCmd, statsCmd, clearCmd)
	return cmd
}

func kbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kb",
		Short: "Knowledge base commands",
		Long:  "Shared knowledge across sessions",
	}

	getCtx := func() *memory.Context {
		return memory.NewContext()
	}

	// urp kb store <text>
	var kind, scope string
	storeCmd := &cobra.Command{
		Use:   "store <text>",
		Short: "Store knowledge",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			id, err := kb.Store(context.Background(), args[0], kind, scope)
			if err != nil {
				fatalError(err)
			}

			fmt.Printf("Stored: %s\n", id)
			fmt.Printf("  Kind: %s, Scope: %s\n", kind, scope)
		},
	}
	storeCmd.Flags().StringVarP(&kind, "kind", "k", "rule", "Knowledge type (error|fix|rule|pattern)")
	storeCmd.Flags().StringVarP(&scope, "scope", "s", "session", "Visibility (session|instance|global)")

	// urp kb query <text>
	var limit int
	var level string
	queryCmd := &cobra.Command{
		Use:   "query <text>",
		Short: "Search knowledge",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			results, err := kb.Query(context.Background(), args[0], limit, level, "")
			if err != nil {
				fatalError(err)
			}

			if len(results) == 0 {
				fmt.Println("No matching knowledge found")
				return
			}

			fmt.Println("KNOWLEDGE: Matching entries")
			for _, k := range results {
				fmt.Printf("  [%.0f%%] [%s/%s] %s\n",
					k.Similarity*100, k.Scope, k.Kind, truncateStr(k.Text, 50))
			}
		},
	}
	queryCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")
	queryCmd.Flags().StringVarP(&level, "level", "l", "all", "Search level (session|instance|global|all)")

	// urp kb list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all knowledge",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			results, err := kb.List(context.Background(), "", "", 50)
			if err != nil {
				fatalError(err)
			}

			if len(results) == 0 {
				fmt.Println("No knowledge stored")
				return
			}

			fmt.Printf("KNOWLEDGE: %d entries\n", len(results))
			for _, k := range results {
				fmt.Printf("  %s [%s/%s] %s\n", k.KnowledgeID, k.Scope, k.Kind, truncateStr(k.Text, 40))
			}
		},
	}

	// urp kb reject <id> <reason>
	rejectCmd := &cobra.Command{
		Use:   "reject <knowledge-id> <reason>",
		Short: "Mark knowledge as not applicable",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			err := kb.Reject(context.Background(), args[0], args[1])
			if err != nil {
				fatalError(err)
			}

			fmt.Printf("Rejected: %s\n", args[0])
			fmt.Printf("  Reason: %s\n", args[1])
		},
	}

	// urp kb promote <id>
	promoteCmd := &cobra.Command{
		Use:   "promote <knowledge-id>",
		Short: "Promote knowledge to global scope",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			err := kb.Promote(context.Background(), args[0])
			if err != nil {
				fatalError(err)
			}

			fmt.Printf("Promoted to global: %s\n", args[0])
		},
	}

	// urp kb stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show knowledge statistics",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			stats, err := kb.Stats(context.Background())
			if err != nil {
				fatalError(err)
			}

			printJSON(stats)
		},
	}

	cmd.AddCommand(storeCmd, queryCmd, listCmd, rejectCmd, promoteCmd, statsCmd)
	return cmd
}
