package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/cognitive"
)

func thinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "think",
		Short: "Cognitive skills",
		Long:  "AI-like reasoning: wisdom, novelty, learning",
	}

	// urp think wisdom <error>
	var threshold float64
	var project string
	wisdomCmd := &cobra.Command{
		Use:   "wisdom <error-message>",
		Short: "Find similar past errors",
		Long:  "Search for similar errors and their solutions",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "wisdom")

			requireDB(event)

			svc := cognitive.NewWisdomService(db)
			matches, err := svc.ConsultWisdom(context.Background(), args[0], threshold, project)
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if len(matches) == 0 {
				fmt.Println("WISDOM: No similar past errors found")
				fmt.Println("RECOMMENDATION: This may be a new type of error. Proceed with investigation.")
				return
			}

			fmt.Println("WISDOM: Similar past errors found")
			fmt.Println()
			for i, m := range matches {
				fmt.Printf("%d. [%.0f%%] %s\n", i+1, m.Similarity*100, m.Command)
				fmt.Printf("   Error: %s\n", truncateStr(m.Error, 80))
				if m.Solution != "" {
					fmt.Printf("   Solution: %s\n", m.Solution)
				}
				fmt.Println()
			}
		},
	}
	wisdomCmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.3, "Similarity threshold (0-1)")
	wisdomCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	// urp think novelty <code>
	noveltyCmd := &cobra.Command{
		Use:   "novelty <code-or-pattern>",
		Short: "Check if code is novel/unusual",
		Long:  "Analyze how novel a piece of code is compared to the codebase",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "novelty")

			requireDB(event)

			svc := cognitive.NewNoveltyService(db)
			result, err := svc.CheckNovelty(context.Background(), args[0])
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			// Visual indicator
			indicator := "🟢"
			if result.Level == "moderate" {
				indicator = "🟡"
			} else if result.Level == "high" || result.Level == "pioneer" {
				indicator = "🔴"
			}

			fmt.Printf("NOVELTY: %.0f%% %s\n", result.Novelty*100, indicator)
			fmt.Printf("Level: %s\n", result.Level)
			fmt.Printf("Message: %s\n", result.Message)
			if result.Matches > 0 {
				fmt.Printf("Similar patterns found: %d\n", result.Matches)
			}
		},
	}

	// urp think learn <description>
	var minutes int
	learnCmd := &cobra.Command{
		Use:   "learn [description]",
		Short: "Consolidate recent success into knowledge",
		Long:  "Create a Solution node linking recent successful commands",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "learn")

			requireDB(event)

			description := "Solution validated"
			if len(args) > 0 {
				description = args[0]
			}

			svc := cognitive.NewLearningService(db)
			result, err := svc.ConsolidateLearning(context.Background(), description, minutes)
			if err != nil {
				exitOnError(event, err)
			}

			if !result.Success {
				auditLogger.LogError(event, fmt.Errorf("%s", result.Error))
				fmt.Printf("Learning failed: %s\n", result.Error)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

			fmt.Printf("LEARNED: %s\n", result.Description)
			fmt.Printf("  Solution ID: %s\n", result.SolutionID)
			fmt.Printf("  Commands linked: %d\n", result.CommandsLinked)
			if result.ConflictsResolved > 0 {
				fmt.Printf("  Conflicts resolved: %d\n", result.ConflictsResolved)
			}
			fmt.Println("Knowledge crystallized. Future wisdom queries will find this solution.")
		},
	}
	learnCmd.Flags().IntVarP(&minutes, "minutes", "m", 10, "Look back N minutes")

	cmd.AddCommand(wisdomCmd, noveltyCmd, learnCmd)
	return cmd
}
