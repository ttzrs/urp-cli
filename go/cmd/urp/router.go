package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/model"
)

func routerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "router",
		Short: "Model routing commands",
		Long:  "Intelligent model selection and routing configuration",
	}

	// Get data paths
	home, _ := os.UserHomeDir()
	learningPath := filepath.Join(home, ".urp", "routing", "learning.json")
	budgetPath := filepath.Join(home, ".urp", "routing", "budget.json")

	// urp router status
	statusCmd := newCommand(CommandConfig{
		Use:      "status",
		Short:    "Show routing status and configuration",
		Category: audit.CategoryCognitive,
		Action:   "status",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			router := agent.DefaultModelRouter
			config := router.GetConfig()

			fmt.Println("MODEL ROUTER STATUS")
			fmt.Println("-------------------")
			if router.IsEnabled() {
				fmt.Println("Status: ENABLED ✓")
			} else {
				fmt.Println("Status: DISABLED ✗")
			}

			fmt.Printf("\nBudget Limits:\n")
			fmt.Printf("  Daily:   $%.2f\n", config.Budget.DailyLimit)
			fmt.Printf("  Session: $%.2f\n", config.Budget.SessionLimit)
			fmt.Printf("  Per-Task: $%.2f\n", config.Budget.MaxPerTask)

			fmt.Printf("\nRouting Weights:\n")
			fmt.Printf("  Quality: %.0f%%\n", config.Weights.Quality*100)
			fmt.Printf("  Cost:    %.0f%%\n", config.Weights.Cost*100)
			fmt.Printf("  Speed:   %.0f%%\n", config.Weights.Speed*100)

			fmt.Printf("\nActive Rules: %d\n", len(config.Rules))
			fmt.Printf("Fallback Chain: %s\n", strings.Join(config.FallbackChain, " → "))

			return nil
		},
	})

	// urp router rules
	rulesCmd := newCommand(CommandConfig{
		Use:      "rules",
		Short:    "List active routing rules",
		Category: audit.CategoryCognitive,
		Action:   "rules",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			router := agent.DefaultModelRouter
			config := router.GetConfig()

			fmt.Println("ROUTING RULES (priority order)")
			fmt.Println("-------------------------------")
			for _, rule := range config.Rules {
				fmt.Printf("  [%2d] %-20s → %-30s if %s\n",
					rule.Priority, rule.Name, rule.Model, rule.Condition)
			}
			return nil
		},
	})

	// urp router models
	modelsCmd := newCommand(CommandConfig{
		Use:      "models",
		Short:    "List available models with costs",
		Category: audit.CategoryCognitive,
		Action:   "models",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			registry := model.DefaultModelRegistry
			models := registry.ListEnabled()

			// Sort by tier then cost
			sort.Slice(models, func(i, j int) bool {
				if models[i].QualityTier != models[j].QualityTier {
					return models[i].QualityTier > models[j].QualityTier
				}
				return models[i].InputCost < models[j].InputCost
			})

			fmt.Println("AVAILABLE MODELS")
			fmt.Println("----------------")
			fmt.Printf("%-30s  Tier  Cost($/1M)  Context   Caps\n", "Model ID")
			fmt.Println(strings.Repeat("-", 80))

			for _, m := range models {
				tierStr := fmt.Sprintf("T%d", m.QualityTier)
				costStr := fmt.Sprintf("$%.2f/$%.2f", m.InputCost, m.OutputCost)
				ctxStr := fmt.Sprintf("%dk", m.ContextSize/1000)
				capsStr := strings.Join(m.Capabilities[:min(3, len(m.Capabilities))], ",")
				if len(m.Capabilities) > 3 {
					capsStr += "..."
				}

				fmt.Printf("%-30s  %-4s  %-10s  %-8s  %s\n",
					m.ID, tierStr, costStr, ctxStr, capsStr)
			}

			fmt.Printf("\nTotal: %d models\n", len(models))
			return nil
		},
	})

	// urp router stats
	statsCmd := newCommand(CommandConfig{
		Use:      "stats",
		Short:    "Show model usage statistics",
		Category: audit.CategoryCognitive,
		Action:   "stats",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			learning := agent.NewModelLearningStoreWithPath(learningPath)
			budget := agent.NewBudgetTrackerWithPath(budgetPath)

			fmt.Println("MODEL USAGE STATISTICS")
			fmt.Println("----------------------")

			// Budget summary
			summary := budget.GetSummary()
			fmt.Printf("\nCurrent Session:\n")
			fmt.Printf("  Spent:     $%.4f / $%.2f (%.1f%%)\n",
				summary.CurrentSession, summary.SessionLimit, summary.SessionPercent)
			fmt.Printf("  Daily:     $%.4f / $%.2f (%.1f%%)\n",
				summary.CurrentDaily, summary.DailyLimit, summary.DailyPercent)
			fmt.Printf("  API Calls: %d\n", summary.TotalCalls)

			// Model breakdown
			modelCosts := budget.GetModelCosts()
			if len(modelCosts) > 0 {
				fmt.Printf("\nCost by Model:\n")
				for id, mc := range modelCosts {
					fmt.Printf("  %-25s: $%.4f (%d calls, ~$%.4f/call)\n",
						id, mc.TotalCost, mc.CallCount, mc.AvgCost)
				}
			}

			// Learning stats
			allStats := learning.GetAllStats()
			if len(allStats) > 0 {
				fmt.Printf("\nLearned Performance:\n")
				for id, stats := range allStats {
					fmt.Printf("  %-25s: %.0f%% success, %.2f avg score (%d samples)\n",
						id, stats.SuccessRate*100, stats.AvgScore, stats.SampleCount)
				}
			} else {
				fmt.Printf("\nNo learning data yet. Run some tasks to start learning!\n")
			}

			return nil
		},
	})

	// urp router budget
	budgetCmd := newCommand(CommandConfig{
		Use:      "budget",
		Short:    "Show detailed budget information",
		Category: audit.CategoryCognitive,
		Action:   "budget",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			budget := agent.NewBudgetTrackerWithPath(budgetPath)

			fmt.Println("BUDGET DETAILS")
			fmt.Println("--------------")

			summary := budget.GetSummary()
			fmt.Printf("\nLimits:\n")
			fmt.Printf("  Daily:    $%.2f\n", summary.DailyLimit)
			fmt.Printf("  Session:  $%.2f\n", summary.SessionLimit)
			fmt.Printf("  Per-Task: $%.2f\n", summary.MaxPerTask)

			fmt.Printf("\nCurrent Usage:\n")
			fmt.Printf("  Session:  $%.4f (%.1f%% of limit)\n",
				summary.CurrentSession, summary.SessionPercent)
			fmt.Printf("  Daily:    $%.4f (%.1f%% of limit)\n",
				summary.CurrentDaily, summary.DailyPercent)

			remaining_session := summary.SessionLimit - summary.CurrentSession
			remaining_daily := summary.DailyLimit - summary.CurrentDaily
			fmt.Printf("\nRemaining:\n")
			fmt.Printf("  Session:  $%.4f\n", remaining_session)
			fmt.Printf("  Daily:    $%.4f\n", remaining_daily)

			// Recent history
			history := budget.GetHistory(10)
			if len(history) > 0 {
				fmt.Printf("\nRecent Costs:\n")
				for i, h := range history {
					if i >= 5 {
						break
					}
					modelStr := h.ModelID
					if modelStr == "" {
						modelStr = "(unknown)"
					}
					fmt.Printf("  $%.4f  %-20s  %s\n",
						h.Cost, modelStr, h.Timestamp.Format("15:04:05"))
				}
			}

			return nil
		},
	})

	// urp router reset
	var resetLearning, resetBudget bool
	resetCmd := newCommand(CommandConfig{
		Use:      "reset",
		Short:    "Reset learning data or budget counters",
		Category: audit.CategoryCognitive,
		Action:   "reset",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			if !resetLearning && !resetBudget {
				fmt.Println("Specify --learning or --budget to reset")
				return nil
			}

			if resetLearning {
				learning := agent.NewModelLearningStoreWithPath(learningPath)
				learning.Clear()
				fmt.Println("✓ Learning data reset")
			}

			if resetBudget {
				budget := agent.NewBudgetTrackerWithPath(budgetPath)
				budget.ResetSession()
				fmt.Println("✓ Session budget reset")
			}

			return nil
		},
	})
	resetCmd.Flags().BoolVar(&resetLearning, "learning", false, "Reset learning data")
	resetCmd.Flags().BoolVar(&resetBudget, "budget", false, "Reset session budget")

	// urp router classify <text>
	classifyCmd := newCommand(CommandConfig{
		Use:      "classify <prompt>",
		Short:    "Classify a prompt and show routing decision",
		Args:     cobra.ExactArgs(1),
		Category: audit.CategoryCognitive,
		Action:   "classify",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			classifier := agent.DefaultTaskClassifier
			router := agent.DefaultModelRouter

			classification := classifier.Classify(args[0], nil)

			fmt.Println("TASK CLASSIFICATION")
			fmt.Println("-------------------")
			fmt.Printf("Task Type:   %s\n", classification.TaskType)
			fmt.Printf("Complexity:  %.2f\n", classification.Complexity)
			fmt.Printf("Environment: %s\n", classification.Environment)
			fmt.Printf("Capabilities: %v\n", classification.RequiredCaps)
			fmt.Printf("Est. Tokens: %d\n", classification.EstTokens)
			fmt.Printf("Has Images:  %v\n", classification.HasImages)
			fmt.Printf("Confidence:  %.2f\n", classification.Confidence)

			// Get routing decision
			selection := router.SelectModel(cmd.Context(), classification)

			fmt.Println("\nROUTING DECISION")
			fmt.Println("----------------")
			fmt.Printf("Selected Model: %s\n", selection.ModelID)
			fmt.Printf("Confidence:     %.2f\n", selection.Confidence)
			fmt.Printf("Reason:         %s\n", selection.Reason)
			if selection.RuleName != "" {
				fmt.Printf("Rule:           %s\n", selection.RuleName)
			}
			if selection.EstCost > 0 {
				fmt.Printf("Est. Cost:      $%.4f\n", selection.EstCost)
			}

			return nil
		},
	})

	cmd.AddCommand(
		statusCmd,
		rulesCmd,
		modelsCmd,
		statsCmd,
		budgetCmd,
		resetCmd,
		classifyCmd,
	)

	return cmd
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
