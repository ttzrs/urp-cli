package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/alerts"
)

// alertCmd provides commands for sending and managing system alerts
func alertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "System alert commands",
		Long:  "Send and manage alerts that Claude receives via hooks",
	}

	cmd.AddCommand(
		alertSendCmd(),
		alertListCmd(),
		alertResolveCmd(),
		alertDirCmd(),
	)
	return cmd
}

func alertSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <level> <component> <title> <message>",
		Short: "Send a system alert",
		Long: `Send an alert that will be injected into Claude's context.

Levels: info, warning, error, critical

Examples:
  urp alert send error worker "Worker Crashed" "Worker-1 exited with code 137"
  urp alert send critical container "OOM Kill" "Container ran out of memory"`,
		Args: cobra.ExactArgs(4),
		Run: func(cmd *cobra.Command, args []string) {
			level := alerts.Level(args[0])
			component := args[1]
			title := args[2]
			message := args[3]

			ctx := make(map[string]interface{})
			if ctxFlag, _ := cmd.Flags().GetString("context"); ctxFlag != "" {
				json.Unmarshal([]byte(ctxFlag), &ctx)
			}

			var alert *alerts.Alert
			switch level {
			case alerts.LevelInfo:
				alert = alerts.Info(component, title, message)
			case alerts.LevelWarning:
				alert = alerts.Warning(component, title, message)
			case alerts.LevelError:
				alert = alerts.Error(component, title, message, ctx)
			case alerts.LevelCritical:
				alert = alerts.Critical(component, title, message, ctx)
			default:
				fatalErrorf("Invalid level: %s (use info, warning, error, critical)", level)
			}

			fmt.Printf("Alert sent: %s\n", alert.ID)
			fmt.Printf("  Level: %s\n", alert.Level)
			fmt.Printf("  Component: %s\n", alert.Component)
			fmt.Printf("  Title: %s\n", alert.Title)
		},
	}
	cmd.Flags().String("context", "", "JSON context data")

	return cmd
}

func alertListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active alerts",
		Run: func(cmd *cobra.Command, args []string) {
			active := alerts.Global().GetActive()
			if len(active) == 0 {
				fmt.Println("No active alerts")
				return
			}

			fmt.Printf("%d active alert(s):\n\n", len(active))
			for _, a := range active {
				icon := "i"
				switch a.Level {
				case alerts.LevelWarning:
					icon = "!"
				case alerts.LevelError:
					icon = "X"
				case alerts.LevelCritical:
					icon = "!!"
				}
				fmt.Printf("[%s] %s: %s\n", icon, a.Component, a.Title)
				fmt.Printf("    %s\n", a.Message)
				fmt.Printf("    ID: %s\n\n", a.ID)
			}
		},
	}
}

func alertResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <alert-id>",
		Short: "Resolve an alert",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alerts.Resolve(args[0])
			fmt.Printf("Resolved: %s\n", args[0])
		},
	}
}

func alertDirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dir",
		Short: "Show alert directory path",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(alerts.GetAlertDir())
		},
	}
}
