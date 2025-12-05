package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/container"
	urpmetrics "github.com/joss/urp/internal/metrics"
	"github.com/joss/urp/internal/runtime"
	"github.com/joss/urp/internal/vector"
)

func sysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sys",
		Short: "System/runtime commands",
		Long:  "Container observation: vitals, topology, health (Φ energy primitives)",
	}

	// urp sys vitals
	vitalsCmd := &cobra.Command{
		Use:   "vitals",
		Short: "Show container CPU/RAM metrics",
		Long:  "Display energy metrics for running containers (Φ primitive)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategorySystem, "vitals")

			obs := runtime.NewObserver(db)

			if obs.Runtime() == "" {
				auditLogger.LogWarning(event, "no container runtime detected")
				fmt.Println("No container runtime detected (docker/podman)")
				return
			}

			states, err := obs.Vitals(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if len(states) == 0 {
				fmt.Println("No running containers")
				return
			}

			fmt.Println("VITALS (Φ energy)")
			fmt.Println()
			for _, s := range states {
				memPct := fmt.Sprintf("%.1f%%", s.MemoryPct)
				cpuPct := fmt.Sprintf("%.1f%%", s.CPUPercent)
				fmt.Printf("  %-20s  CPU: %6s  MEM: %6s (%s / %s)\n",
					truncateStr(s.Name, 20),
					cpuPct,
					memPct,
					formatBytes(s.MemoryBytes),
					formatBytes(s.MemoryLimit))
			}
		},
	}

	// urp sys topology
	topologyCmd := &cobra.Command{
		Use:   "topology",
		Short: "Show container network map",
		Long:  "Display container network topology (⊆ inclusion)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategorySystem, "topology")

			obs := runtime.NewObserver(db)

			topo, err := obs.Topology(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if topo.Error != "" {
				fmt.Printf("Warning: %s\n\n", topo.Error)
			}

			if len(topo.Containers) == 0 {
				fmt.Println("No containers found")
				return
			}

			fmt.Println("TOPOLOGY (⊆ network map)")
			fmt.Println()

			// Group by network
			byNetwork := make(map[string][]string)
			for _, c := range topo.Containers {
				for _, net := range c.Networks {
					byNetwork[net] = append(byNetwork[net], c.Name)
				}
			}

			for net, containers := range byNetwork {
				fmt.Printf("  [%s]\n", net)
				for _, name := range containers {
					fmt.Printf("    └── %s\n", name)
				}
			}
		},
	}

	// urp sys health
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check container health issues",
		Long:  "Detect container problems (⊥ orthogonal conflicts)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategorySystem, "health")

			obs := runtime.NewObserver(db)

			issues, err := obs.Health(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			auditLogger.LogSuccess(event)

			if len(issues) == 0 {
				fmt.Println("HEALTH: All containers healthy")
				return
			}

			fmt.Println("HEALTH (⊥ issues detected)")
			fmt.Println()
			for _, issue := range issues {
				icon := "⚠"
				if issue.Severity == "ERROR" || issue.Severity == "FATAL" {
					icon = "✗"
				}
				fmt.Printf("  %s [%s] %s: %s\n",
					icon, issue.Type, issue.Container, issue.Detail)
			}
		},
	}

	// urp sys runtime
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "Show detected container runtime",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategorySystem, "runtime")

			obs := runtime.NewObserver(db)
			rt := obs.Runtime()

			auditLogger.LogSuccess(event)

			if rt == "" {
				fmt.Println("No container runtime detected")
				return
			}
			fmt.Printf("Runtime: %s\n", rt)
		},
	}

	// urp sys gpu - check GPU availability
	gpuCmd := &cobra.Command{
		Use:   "gpu",
		Short: "Check GPU availability for NeMo",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())
			status := mgr.CheckGPU()

			if status.Available {
				fmt.Printf("GPU: available (%d device(s))\n", status.DeviceCount)
				fmt.Println("NeMo will use GPU acceleration")
			} else {
				fmt.Printf("GPU: not available\n")
				fmt.Printf("Reason: %s\n", status.Reason)
				fmt.Println("NeMo will run in CPU-only mode (slower)")
			}
		},
	}

	// urp sys metrics - show or serve metrics
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show current metrics or start metrics server",
		Long: `Display current URP metrics in Prometheus format.

Use --serve to start a metrics server on port 9090 (or custom port).
The /metrics endpoint can be scraped by Prometheus.`,
		Run: func(cmd *cobra.Command, args []string) {
			serve, _ := cmd.Flags().GetBool("serve")
			port, _ := cmd.Flags().GetInt("port")

			m := urpmetrics.Global()

			if serve {
				srv := urpmetrics.NewServer(port)
				fmt.Printf("Starting metrics server on :%d\n", port)
				fmt.Println("Endpoints: /metrics, /health")
				fmt.Println("Press Ctrl+C to stop")
				if err := srv.Start(); err != nil {
					fatalErrorf("start server: %v", err)
				}
				// Block forever
				select {}
			}

			// Just print current metrics
			fmt.Printf("URP Metrics (current session):\n\n")
			fmt.Printf("Container Operations:\n")
			fmt.Printf("  Worker spawns:     %d (errors: %d)\n", m.WorkerSpawns.Load(), m.WorkerSpawnErrors.Load())
			fmt.Printf("  NeMo launches:     %d (errors: %d)\n", m.NeMoLaunches.Load(), m.NeMoLaunchErrors.Load())
			fmt.Printf("\nHealth Checks:\n")
			fmt.Printf("  Total checks:      %d (failures: %d)\n", m.HealthChecks.Load(), m.HealthCheckFailures.Load())
			fmt.Printf("\nGraph Operations:\n")
			fmt.Printf("  Writes:            %d (errors: %d)\n", m.GraphWrites.Load(), m.GraphWriteErrors.Load())
			fmt.Printf("\nTiming (last operation):\n")
			fmt.Printf("  Last spawn:        %dms\n", m.LastSpawnDurationMs.Load())
			fmt.Printf("  Last NeMo launch:  %dms\n", m.LastNeMoDurationMs.Load())
		},
	}
	metricsCmd.Flags().Bool("serve", false, "Start metrics HTTP server")
	metricsCmd.Flags().Int("port", 9090, "Port for metrics server")

	cmd.AddCommand(vitalsCmd, topologyCmd, healthCmd, runtimeCmd, gpuCmd, metricsCmd)
	return cmd
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func vecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vec",
		Short: "Vector store commands",
		Long:  "Manage vector embeddings for semantic search",
	}

	// urp vec stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show vector store statistics",
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			count, err := store.Count(context.Background())
			if err != nil {
				fatalError(err)
			}

			embedder := vector.GetDefaultEmbedder()

			fmt.Println("VECTOR STORE")
			fmt.Println()
			fmt.Printf("  Entries:    %d\n", count)
			fmt.Printf("  Dimensions: %d\n", embedder.Dimensions())
			fmt.Printf("  Embedder:   local (hash-based)\n")
		},
	}

	// urp vec search <query>
	var limit int
	var kind string
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search vectors by text",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			embedder := vector.GetDefaultEmbedder()

			// Generate embedding for query
			queryVec, err := embedder.Embed(context.Background(), args[0])
			if err != nil {
				fatalErrorf("embedding query: %v", err)
			}

			// Search
			results, err := store.Search(context.Background(), queryVec, limit, kind)
			if err != nil {
				fatalError(err)
			}

			if len(results) == 0 {
				fmt.Println("No matching vectors found")
				return
			}

			fmt.Printf("VECTOR SEARCH: %d results\n", len(results))
			fmt.Println()
			for i, r := range results {
				fmt.Printf("%d. [%.0f%%] [%s] %s\n",
					i+1, r.Score*100, r.Entry.Kind, truncateStr(r.Entry.Text, 60))
				if r.Entry.Metadata != nil {
					if cmd := r.Entry.Metadata["command"]; cmd != "" {
						fmt.Printf("   Command: %s\n", truncateStr(cmd, 50))
					}
					if proj := r.Entry.Metadata["project"]; proj != "" {
						fmt.Printf("   Project: %s\n", proj)
					}
				}
			}
		},
	}
	searchCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")
	searchCmd.Flags().StringVarP(&kind, "kind", "k", "", "Filter by kind (error|code|solution)")

	// urp vec add <text>
	var addKind string
	addCmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Add text to vector store",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			embedder := vector.GetDefaultEmbedder()

			// Generate embedding
			vec, err := embedder.Embed(context.Background(), args[0])
			if err != nil {
				fatalErrorf("embedding: %v", err)
			}

			entry := vector.VectorEntry{
				Text:   args[0],
				Vector: vec,
				Kind:   addKind,
			}

			if err := store.Add(context.Background(), entry); err != nil {
				fatalError(err)
			}

			fmt.Printf("Added to vector store [%s]: %s\n", addKind, truncateStr(args[0], 50))
		},
	}
	addCmd.Flags().StringVarP(&addKind, "kind", "k", "knowledge", "Entry kind (error|code|solution|knowledge)")

	cmd.AddCommand(statsCmd, searchCmd, addCmd)
	return cmd
}
