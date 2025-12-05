package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/skills"
)

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skill",
		Aliases: []string{"skills", "sk"},
		Short:   "Skill management commands",
		Long: `Manage AI agent skills organized by category:

💻 dev       - Development & Automation
🛡️ security  - Security & Intelligence (OSINT)
📝 content   - Content Creation & Social
📊 data      - Data & Analytics
🧠 growth    - Personal Development & Philosophy
🏢 business  - Business & Research
⚙️ core      - System & Maintenance`,
	}

	cmd.AddCommand(
		skillListCmd(),
		skillShowCmd(),
		skillRunCmd(),
		skillLoadCmd(),
		skillSearchCmd(),
		skillStatsCmd(),
		skillCategoriesCmd(),
		skillAddCmd(),
		skillDeleteCmd(),
	)

	return cmd
}

func skillListCmd() *cobra.Command {
	var category string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.list")
			requireDB(event)

			store := skills.NewStore(db)
			list, err := store.List(context.Background(), skills.Category(category))
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if len(list) == 0 {
				fmt.Println("No skills found. Use 'urp skill load' to load skills.")
				return
			}

			// Group by category
			grouped := make(map[skills.Category][]*skills.Skill)
			for _, sk := range list {
				grouped[sk.Category] = append(grouped[sk.Category], sk)
			}

			// Print by category
			for _, cat := range []skills.Category{
				skills.CategoryDev, skills.CategorySecurity, skills.CategoryContent,
				skills.CategoryData, skills.CategoryGrowth, skills.CategoryBusiness,
				skills.CategoryCore,
			} {
				sks := grouped[cat]
				if len(sks) == 0 {
					continue
				}

				info := skills.Categories[cat]
				fmt.Printf("\n%s %s\n", info.Icon, info.Title)
				fmt.Println(strings.Repeat("─", 40))

				for _, sk := range sks {
					agent := ""
					if sk.Agent != "" {
						agent = fmt.Sprintf(" [agent:%s]", sk.Agent)
					}
					fmt.Printf("  %-20s %s%s\n", sk.Name, truncateStr(sk.Description, 40), agent)
				}
			}
			fmt.Println()
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (dev, security, content, data, growth, business, core)")

	return cmd
}

func skillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show skill details",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.show")
			requireDB(event)

			store := skills.NewStore(db)
			sk, err := store.GetByName(context.Background(), args[0])
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			info := skills.Categories[sk.Category]
			fmt.Printf("\n%s %s\n", info.Icon, sk.Name)
			fmt.Println(strings.Repeat("═", 50))
			fmt.Printf("Category:    %s (%s)\n", info.Title, sk.Category)
			fmt.Printf("Version:     %s\n", sk.Version)
			fmt.Printf("Source:      %s (%s)\n", sk.Source, sk.SourceType)
			fmt.Printf("Usage:       %d times\n", sk.UsageCount)

			if sk.Agent != "" {
				fmt.Printf("Agent:       %s\n", sk.Agent)
			}

			if len(sk.Tags) > 0 {
				fmt.Printf("Tags:        %s\n", strings.Join(sk.Tags, ", "))
			}

			if len(sk.ContextFiles) > 0 {
				fmt.Println("\nContext Files:")
				for _, cf := range sk.ContextFiles {
					fmt.Printf("  - %s\n", cf)
				}
			}

			fmt.Printf("\nDescription:\n%s\n", sk.Description)
		},
	}
}

func skillRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <name> [input]",
		Short: "Execute a skill",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.run")
			requireDB(event)

			store := skills.NewStore(db)
			executor := skills.NewExecutor(store, config.Env().SessionID)

			input := ""
			if len(args) > 1 {
				input = strings.Join(args[1:], " ")
			}

			result, err := executor.Execute(context.Background(), args[0], input)
			if err != nil {
				exitOnError(event, err)
			}

			event.OutputSize = len(result.Output)
			auditLogger.LogSuccess(event)

			fmt.Println(result.Output)

			if result.Agent != "" {
				fmt.Printf("\n[Spawn agent: %s]\n", result.Agent)
			}
		},
	}
}

func skillLoadCmd() *cobra.Command {
	var builtins bool

	cmd := &cobra.Command{
		Use:   "load [directory]",
		Short: "Load skills from directory",
		Long: `Load skills from a directory structure.

Default directory: ~/.urp-go/skills/

Directory structure:
  skills/
    dev/
      AgentServer.md
      BrowserAutomation.md
    security/
      OSINT.md
      RedTeam.md
    content/
      Blogging.md
    ...`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.load")
			requireDB(event)

			store := skills.NewStore(db)
			loader := skills.NewLoader(store)

			// Load builtins first
			if builtins {
				if err := loader.RegisterBuiltins(context.Background()); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: loading builtins: %v\n", err)
				}
				fmt.Println("Loaded builtin skills")
			}

			// Load from directory
			var dir string
			if len(args) > 0 {
				dir = args[0]
			} else {
				dir = urpSkillsPath()
			}

			if _, err := os.Stat(dir); os.IsNotExist(err) {
				fmt.Printf("Skills directory not found: %s\n", dir)
				if builtins {
					auditLogger.LogSuccess(event)
					return
				}
				os.Exit(1)
			}

			count, err := loader.LoadFromDirectory(context.Background(), dir)
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)
			fmt.Printf("Loaded %d skills from %s\n", count, dir)
		},
	}

	cmd.Flags().BoolVarP(&builtins, "builtins", "b", true, "Include builtin skills")

	return cmd
}

func skillSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <pattern>",
		Short: "Search skills by name, tag, or description",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.search")
			requireDB(event)

			store := skills.NewStore(db)
			results, err := store.Search(context.Background(), args[0])
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if len(results) == 0 {
				fmt.Println("No skills found matching:", args[0])
				return
			}

			fmt.Printf("Found %d skills:\n\n", len(results))
			for _, sk := range results {
				info := skills.Categories[sk.Category]
				fmt.Printf("%s %-20s %s\n", info.Icon, sk.Name, truncateStr(sk.Description, 50))
			}
		},
	}
}

func skillStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show skill statistics",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.stats")
			requireDB(event)

			store := skills.NewStore(db)
			stats, err := store.Stats(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			printJSON(stats)
		},
	}
}

func skillCategoriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "categories",
		Short: "List skill categories",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("\nSkill Categories:")
			fmt.Println(strings.Repeat("═", 50))

			for _, cat := range []skills.Category{
				skills.CategoryDev, skills.CategorySecurity, skills.CategoryContent,
				skills.CategoryData, skills.CategoryGrowth, skills.CategoryBusiness,
				skills.CategoryCore,
			} {
				info := skills.Categories[cat]
				fmt.Printf("\n%s %s (%s)\n", info.Icon, info.Title, cat)
				fmt.Printf("   %s\n", info.Description)
			}
			fmt.Println()
		},
	}
}

func skillAddCmd() *cobra.Command {
	var category string
	var agent string
	var tags []string

	cmd := &cobra.Command{
		Use:   "add <name> <description>",
		Short: "Add a new skill",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.add")
			requireDB(event)

			store := skills.NewStore(db)
			loader := skills.NewLoader(store)

			// Create skill directly
			sk := &skills.Skill{
				Name:        args[0],
				Category:    skills.Category(category),
				Description: strings.Join(args[1:], " "),
				Version:     "1.0",
				SourceType:  "inline",
				Agent:       agent,
				Tags:        tags,
			}

			// Use loader to handle ID generation
			if err := loader.RegisterBuiltins(context.Background()); err != nil {
				// Ignore, just ensuring store is ready
			}

			if err := store.Create(context.Background(), sk); err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)
			fmt.Printf("Created skill: %s\n", sk.Name)
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "dev", "Skill category")
	cmd.Flags().StringVarP(&agent, "agent", "a", "", "Agent to spawn")
	cmd.Flags().StringSliceVarP(&tags, "tags", "t", nil, "Tags for search")

	return cmd
}

func skillDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a skill",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCognitive, "skill.delete")
			requireDB(event)

			store := skills.NewStore(db)

			// Get skill first to get ID
			sk, err := store.GetByName(context.Background(), args[0])
			if err != nil {
				exitOnError(event, err)
			}

			if err := store.Delete(context.Background(), sk.ID); err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)
			fmt.Printf("Deleted skill: %s\n", args[0])
		},
	}
}

