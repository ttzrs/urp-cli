package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/cognitive"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/vector"
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

	// urp think evaluate
	var useLLM bool
	evaluateCmd := &cobra.Command{
		Use:   "evaluate",
		Short: "Evaluate recent errors and propose fixes",
		Long:  "Analyzes recent audit errors, finds correlated code, and suggests improvements using LLM.",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "evaluate")
			requireDB(event)

			ctx := context.Background()

			// 1. Get recent errors
			auditStore := audit.NewStore(db, config.Env().SessionID)
			auditSvc := audit.NewService(auditStore, auditLogger)
			errors, err := auditSvc.GetErrors(ctx, time.Time{}, "", 10)
			if err != nil {
				exitOnError(event, err)
			}

			if len(errors) == 0 {
				fmt.Println("✅ No recent errors found. System is healthy.")
				auditLogger.LogSuccess(event)
				return
			}

			fmt.Printf("🔍 Analyzing %d recent errors...\n", len(errors))

			// 2. Initialize LLM evaluator if requested
			var evaluator *cognitive.Evaluator
			if useLLM {
				evaluator, err = cognitive.DefaultEvaluator()
				if err != nil {
					fmt.Printf("⚠️  LLM not available: %v (using basic analysis)\n", err)
					useLLM = false
				} else {
					fmt.Println("🤖 LLM analysis enabled")
				}
			}

			// 3. Group by error message
			grouped := make(map[string][]audit.AuditEvent)
			for _, e := range errors {
				msg := e.ErrorMessage
				if len(msg) > 50 {
					msg = msg[:47] + "..."
				}
				grouped[msg] = append(grouped[msg], e)
			}

			// 4. Dependencies for context
			vecStore := vector.Default()
			embedder := vector.GetDefaultEmbedder()
			optimizer := cognitive.NewContextOptimizer(db, vecStore)

			// 5. Process each error pattern
			for msg, evts := range grouped {
				fmt.Printf("\n🔴 Pattern: %s (Count: %d)\n", msg, len(evts))

				// Embed error message to find relevant code
				vec, err := embedder.Embed(ctx, evts[0].ErrorMessage)
				if err != nil {
					fmt.Printf("   ⚠️ Failed to embed error: %v\n", err)
					continue
				}

				contextFiles, err := optimizer.GetOptimizedContext(ctx, vec)
				if err != nil {
					fmt.Printf("   ⚠️ Failed to find context: %v\n", err)
					continue
				}

				if len(contextFiles) > 0 {
					fmt.Println("   Possibly related files (High Energy):")
					displayFiles := contextFiles
					if len(displayFiles) > 3 {
						displayFiles = displayFiles[:3]
					}
					for _, f := range displayFiles {
						fmt.Printf("   - [%.2f] %s\n", f.Energy, f.Path)
					}

					// Call LLM for fix proposal if enabled
					if useLLM && evaluator != nil {
						errCtx := cognitive.ErrorContext{
							ErrorMessage: evts[0].ErrorMessage,
							ErrorCount:   len(evts),
							Category:     string(evts[0].Category),
							Operation:    evts[0].Operation,
							Files:        contextFiles,
							Timestamp:    evts[0].StartedAt,
						}

						proposal, err := evaluator.ProposeFix(ctx, errCtx)
						if err != nil {
							fmt.Printf("   ⚠️ LLM analysis failed: %v\n", err)
						} else {
							fmt.Printf("\n   💡 ROOT CAUSE: %s\n", proposal.Analysis)
							fmt.Printf("   🔧 PROPOSAL: %s\n", proposal.Proposal)
							if len(proposal.Files) > 0 {
								fmt.Printf("   📁 FILES TO MODIFY: %v\n", proposal.Files)
							}
							fmt.Printf("   🎯 CONFIDENCE: %s\n", proposal.Confidence)
						}
					} else {
						fmt.Println("   💡 Proposal: Review these files for logic errors handling this condition.")
						fmt.Println("   (Use --llm flag for AI-powered analysis)")
					}
				} else {
					fmt.Println("   No structural correlation found in graph.")
				}
			}

			auditLogger.LogSuccess(event)
		},
	}
	evaluateCmd.Flags().BoolVar(&useLLM, "llm", false, "Use LLM for AI-powered fix proposals")

	cmd.AddCommand(wisdomCmd, noveltyCmd, learnCmd, contextCmd(), evaluateCmd)
	return cmd
}

// urp think context <prompt>
func contextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <prompt>",
		Short: "Find optimized context",
		Long:  "Uses Hybrid Search + Spreading Activation to find relevant code.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryKnowledge, "context")
			requireDB(event)

			// Initialize dependencies
			// We need a VectorSearcher. Using the default one.
			vecStore := vector.Default()

			// Use configured embedder (TEI/OpenAI/Local based on URP_EMBEDDING_PROVIDER)
			embedder := vector.GetDefaultEmbedder()

			promptVec, err := embedder.Embed(context.Background(), args[0])
			if err != nil {
				exitOnError(event, fmt.Errorf("embed prompt: %w", err))
			}

			optimizer := cognitive.NewContextOptimizer(db, vecStore)
			optimized, err := optimizer.GetOptimizedContext(context.Background(), promptVec)
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			fmt.Println("OPTIMIZED CONTEXT:")
			for i, file := range optimized {
				fmt.Printf("%d. [%.2f] %s\n", i+1, file.Energy, file.Path)
			}
		},
	}
}
