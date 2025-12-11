package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/compiler"
	"github.com/joss/urp/internal/gate"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/pkg/ingestor"
)

// retrievalAdapter adapts agent.EmbeddingStore to compiler.StrategyRetriever
type retrievalAdapter struct {
	store *agent.EmbeddingStore
}

func (r *retrievalAdapter) GetStrategyHint(ctx context.Context, goal string) (string, error) {
	env := "go"
	similar, err := r.store.FindSimilarSuccessful(ctx, goal, env, 3)
	if err != nil {
		return "", err
	}
	if len(similar) == 0 {
		return "", nil
	}

	var sbCtx string
	sbCtx += "Similar successful tasks:\n"
	for _, t := range similar {
		sbCtx += fmt.Sprintf("- %s (Strategy: %s, Similarity: %.2f)\n", t.Objective, t.StrategyUsed, t.Similarity)
	}
	return sbCtx, nil
}

func compileCmd() *cobra.Command {
	var sessionID string
	var goal string
	var logs string
	var ingestFile string

	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Test the Context Compiler (Architecture V2)",
		Long:  "Compiles the context for the LLM using the Graph State and Gate (Noise Filter). Also supports ingesting documents.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			
			// 1. Setup Dependencies
			store := compiler.NewStore(db) // db is global from main.go
			gateClient := gate.NewOpenAIClient("MODEL_GATE")
			comp := compiler.NewContextCompiler(store, gateClient)
			
			// Setup Vector Store (LanceDB)
			vectorPath := config.GetPaths().Vectors
			if err := os.MkdirAll(vectorPath, 0755); err == nil {
				es, err := agent.NewEmbeddingStore(vectorPath)
				if err == nil {
					comp.SetStrategyRetriever(&retrievalAdapter{store: es})
					fmt.Println("[Vector] LanceDB connected.")
				}
			}
			
			// 2. Optional: Ingestion (Phase 1)
			if ingestFile != "" {
				fmt.Println("--- PHASE 1: INGESTION ---")
				processor := ingestor.NewPDFProcessor()
				chunks, err := processor.ProcessDocument(ctx, ingestFile)
				if err != nil {
					fatalErrorf("Ingestion failed: %v", err)
				}
				
				for _, chunk := range chunks {
					err := store.CreateTheoreticalRule(ctx, chunk.Source, chunk.Proposition, chunk.Confidence)
					if err != nil {
						fmt.Printf("Warning: Failed to store rule: %v\n", err)
					} else {
						fmt.Printf("[Graph] Stored: %s\n", chunk.Proposition)
					}
				}
				fmt.Println("--------------------------")
			}

			// 3. Setup Session/State
			if sessionID == "" {
				sessionID = fmt.Sprintf("test_sess_%d", time.Now().Unix())
			}
			if goal == "" {
				goal = "Verify system architecture and compliation pipeline"
			}
			
			fmt.Printf("Session: %s\nGoal: %s\n", sessionID, goal)

			// Init session in Graph
			if err := store.InitSession(ctx, sessionID, goal); err != nil {
				fatalErrorf("Failed to init session: %v", err)
			}
			
			// Inject mock data to ensure we see something
			if err := store.AddMockData(ctx, sessionID); err != nil {
				fmt.Printf("Warning: Failed to add mock data: %v\n", err)
			}

			// 4. Compile Context
			fmt.Println("\n--- COMPILING CONTEXT ---")
			start := time.Now()
			wd, _ := os.Getwd()
			compiled, err := comp.Compile(ctx, sessionID, goal, logs, wd)
			if err != nil {
				fatalErrorf("Compilation failed: %v", err)
			}
			duration := time.Since(start)

			fmt.Println(compiled)
			fmt.Printf("\n-------------------------\n(Compiled in %v)\n", duration)
		},
	}

	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID (optional)")
	cmd.Flags().StringVarP(&goal, "goal", "g", "", "Goal for the session")
	cmd.Flags().StringVarP(&logs, "logs", "l", "", "Raw logs to filter")
	cmd.Flags().StringVarP(&ingestFile, "ingest", "i", "", "Path to PDF file to ingest (simulated)")

	return cmd
}
