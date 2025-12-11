package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/opencode/modelservice"
)

func modelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List all available LLM models by source",
		Long: `List all LLM models from all configured sources (proxy, deepseek, openai, anthropic, google).

Models are shown grouped by source with their 3-letter shortcodes.
Shortcodes can be used with --model flag in other commands.`,
	}

	// urp models (default list)
	listCmd := newSimpleCommand(CommandConfig{
		Use:      "list",
		Short:    "List models grouped by source",
		Category: audit.CategoryCognitive,
		Action:   "models_list",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			// Refresh from all sources first
			svc := modelservice.DefaultService
			if err := svc.Refresh(context.Background()); err != nil {
				// Log but continue - we might have cached data
				fmt.Printf("Warning: failed to refresh some models: %v\n", err)
			}

			bySource := svc.ListBySource()
			totalModels := 0

			// Sort sources for consistent output
			sources := make([]modelservice.Source, 0, len(bySource))
			for source := range bySource {
				sources = append(sources, source)
			}
			sort.Slice(sources, func(i, j int) bool {
				return string(sources[i]) < string(sources[j])
			})

			fmt.Println("AVAILABLE MODELS BY SOURCE")
			fmt.Println("===========================")

			for _, source := range sources {
				models := bySource[source]
				if len(models) == 0 {
					continue
				}
				totalModels += len(models)

				fmt.Printf("\n%s (%d models)\n", strings.ToUpper(string(source)), len(models))
				fmt.Println(strings.Repeat("-", 60))

				// Sort models by shortcode
				sort.Slice(models, func(i, j int) bool {
					if models[i].ShortCode != models[j].ShortCode {
						return models[i].ShortCode < models[j].ShortCode
					}
					return models[i].ID < models[j].ID
				})

				for _, model := range models {
					ctxStr := "?"
					if model.ContextSize > 0 {
						ctxStr = fmt.Sprintf("%dk", model.ContextSize/1000)
					}
					costStr := "?"
					if model.InputCost > 0 || model.OutputCost > 0 {
						costStr = fmt.Sprintf("$%.2f/$%.2f", model.InputCost, model.OutputCost)
					}
					fmt.Printf("  %-6s  %-35s  %6s  %-12s  %s\n",
						model.ShortCode,
						truncateStr(model.ID, 35),
						ctxStr,
						costStr,
						model.Name)
				}
			}

			fmt.Printf("\nTotal: %d models across %d sources\n", totalModels, len(sources))
			fmt.Println("\nUse shortcode with --model flag: e.g., --model 4o for gpt-4o")
			return nil
		},
	})

	// urp models refresh
	refreshCmd := newSimpleCommand(CommandConfig{
		Use:      "refresh",
		Short:    "Force refresh models from all sources",
		Category: audit.CategoryCognitive,
		Action:   "models_refresh",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			svc := modelservice.DefaultService
			if err := svc.Refresh(context.Background()); err != nil {
				return fmt.Errorf("failed to refresh models: %w", err)
			}

			bySource := svc.ListBySource()
			total := 0
			for _, models := range bySource {
				total += len(models)
			}

			fmt.Printf("✓ Refreshed %d models from %d sources\n", total, len(bySource))
			return nil
		},
	})

	// urp models resolve <shortcode>
	resolveCmd := newSimpleCommand(CommandConfig{
		Use:      "resolve <shortcode>",
		Short:    "Resolve a shortcode to model details",
		Args:     cobra.ExactArgs(1),
		Category: audit.CategoryCognitive,
		Action:   "models_resolve",
		RunFunc: func(cmd *cobra.Command, args []string) error {
			svc := modelservice.DefaultService
			// Refresh to ensure models are loaded
			if err := svc.Refresh(context.Background()); err != nil {
				// Log but continue - static models should still be available
				fmt.Printf("Warning: failed to refresh models: %v\n", err)
			}
			model, ok := svc.ResolveShortCode(args[0])
			if !ok {
				return fmt.Errorf("shortcode not found: %s", args[0])
			}

			fmt.Printf("SHORTCODE: %s\n", args[0])
			fmt.Printf("Model ID:  %s\n", model.ID)
			fmt.Printf("Name:      %s\n", model.Name)
			fmt.Printf("Source:    %s\n", model.Source)
			if model.ContextSize > 0 {
				fmt.Printf("Context:   %dk tokens\n", model.ContextSize/1000)
			}
			if model.InputCost > 0 || model.OutputCost > 0 {
				fmt.Printf("Cost:      $%.4f per 1M input / $%.4f per 1M output\n",
					model.InputCost, model.OutputCost)
			}
			fmt.Printf("Provider:  %s\n", string(model.Source))
			return nil
		},
	})

	cmd.AddCommand(listCmd, refreshCmd, resolveCmd)
	return cmd
}
