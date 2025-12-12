package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cere/internal/compiler"
	"cere/internal/graph"
	"cere/internal/llm"
	"cere/pkg/ingestor"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("URP: Universal Robotic Programmer - Master Node Starting...")

	// 0. Load Env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found")
	}

	ctx := context.Background()

	// 1. Initialize Graph Client
	time.Sleep(1 * time.Second)
	
	client, err := graph.NewClient("bolt://localhost:7687", "urp", "urp_secret")
	if err != nil {
		log.Fatalf("Failed to connect to Memgraph: %v", err)
	}
	defer client.Close(ctx)
	fmt.Println("[Connected] Memgraph is ready.")

	// =========================================================================
	// PHASE 1: THE THEORIST (Ingestion)
	// =========================================================================
	fmt.Println("\n--- PHASE 1: INGESTION (The Theorist) ---")
	
	pdfProcessor := ingestor.NewPDFProcessor()
	manualPath := "Manual_VFD_Series_X.pdf"

	// A. Process Document (Simulated ColPali/Gemini)
	chunks, err := pdfProcessor.ProcessDocument(ctx, manualPath)
	if err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	// B. Store Knowledge in Graph
	for _, chunk := range chunks {
		err := client.CreateTheoreticalRule(ctx, chunk.Source, chunk.Proposition, chunk.Confidence)
		if err != nil {
			log.Printf("Failed to store rule: %v", err)
			continue
		}
		fmt.Printf("[Graph] Stored Theoretical Rule: %s (Confidence: %.2f)\n", chunk.Proposition, chunk.Confidence)
	}


	// =========================================================================
	// PHASE 2: EXECUTION & CONTEXT (The Empiricist)
	// =========================================================================
	fmt.Println("\n--- PHASE 2: RUNTIME CONTEXT ---")

	// 2. Initialize Compiler with Real Gate (Proxy/Qwen)
	gateModel := os.Getenv("MODEL_GATE")
	fmt.Printf("[Gate] Initializing Client (Model: %s)...\n", gateModel)
	gate := llm.NewOpenAIClient("MODEL_GATE")
	
	comp := compiler.NewContextCompiler(client, gate)

	// 3. Simulate Execution Cycle
	sessionID := "sess_001"
	goal := "Initialize project infrastructure and verify graph connection"

	// A. Create Session
	if err := client.InitSession(ctx, sessionID, goal); err != nil {
		log.Fatalf("Failed to init session: %v", err)
	}

	// B. Inject Mock Data (Simulate 'Ingest/Execute' phase)
	if err := client.AddMockData(ctx, sessionID); err != nil {
		log.Fatalf("Failed to add mock data: %v", err)
	}

	// C. Simulate Noisy Logs
	rawLogs := `
INFO: Starting process...
DEBUG: Loaded config...
INFO: User logged in.
CRITICAL Error: Database connection timeout in module X.
DEBUG: Retrying...
INFO: Done.
	`

	// D. Compile Context (The Core Feature)
	compiledContext, err := comp.Compile(ctx, sessionID, goal, rawLogs)
	if err != nil {
		log.Fatalf("Failed to compile context: %v", err)
	}

	fmt.Println("\n--- COMPILED CONTEXT VIEW (Cerebras Gated & Graph) ---")
	fmt.Println(compiledContext)
	fmt.Println("------------------------------------------------------")
}