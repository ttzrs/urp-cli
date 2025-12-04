package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	upstream   = getEnv("UPSTREAM_URL", "http://100.105.212.98:8317")
	listenAddr = getEnv("LISTEN_ADDR", ":8318")
	dbPath     = getEnv("STATS_DB", "/app/sessions/proxy_stats.db")
	db         *sql.DB
	mu         sync.Mutex
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Warning: Could not open stats DB: %v", err)
		return
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			model TEXT,
			endpoint TEXT,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cached_tokens INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			status INTEGER DEFAULT 0,
			container TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON requests(timestamp);
		CREATE INDEX IF NOT EXISTS idx_model ON requests(model);
	`)
	if err != nil {
		log.Printf("Warning: Could not create table: %v", err)
	}
}

func logRequest(model, endpoint string, inputTokens, outputTokens, cachedTokens, durationMs, status int) {
	if db == nil {
		return
	}

	container := os.Getenv("HOSTNAME")
	if container == "" {
		container = "unknown"
	}

	mu.Lock()
	defer mu.Unlock()

	_, err := db.Exec(`
		INSERT INTO requests (timestamp, model, endpoint, input_tokens, output_tokens, cached_tokens, duration_ms, status, container)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, time.Now().UTC().Format(time.RFC3339), model, endpoint, inputTokens, outputTokens, cachedTokens, durationMs, status, container)

	if err != nil {
		log.Printf("Warning: Could not log request: %v", err)
	}
}

func extractModel(body []byte) string {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "unknown"
	}
	if model, ok := req["model"].(string); ok {
		return model
	}
	return "unknown"
}

func extractTokensFromResponse(body []byte) (input, output, cached int) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, 0, 0
	}

	// Anthropic format
	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		if v, ok := usage["input_tokens"].(float64); ok {
			input = int(v)
		}
		if v, ok := usage["output_tokens"].(float64); ok {
			output = int(v)
		}
		if v, ok := usage["cache_read_input_tokens"].(float64); ok {
			cached = int(v)
		}
		return
	}

	// OpenAI format
	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		if v, ok := usage["prompt_tokens"].(float64); ok {
			input = int(v)
		}
		if v, ok := usage["completion_tokens"].(float64); ok {
			output = int(v)
		}
		return
	}

	return 0, 0, 0
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	model := extractModel(bodyBytes)
	endpoint := r.URL.Path

	// Create upstream request
	upstreamURL := upstream + r.URL.RequestURI()
	proxyReq, err := http.NewRequest(r.Method, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Send request
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Upstream error: "+err.Error(), http.StatusBadGateway)
		logRequest(model, endpoint, 0, 0, 0, int(time.Since(start).Milliseconds()), 502)
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusBadGateway)
		return
	}

	// Extract tokens
	inputTokens, outputTokens, cachedTokens := extractTokensFromResponse(respBody)
	durationMs := int(time.Since(start).Milliseconds())

	// Log to SQLite
	logRequest(model, endpoint, inputTokens, outputTokens, cachedTokens, durationMs, resp.StatusCode)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write response
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "Stats not available", http.StatusServiceUnavailable)
		return
	}

	type Stats struct {
		TotalRequests int            `json:"total_requests"`
		TotalInput    int            `json:"total_input_tokens"`
		TotalOutput   int            `json:"total_output_tokens"`
		TotalCached   int            `json:"total_cached_tokens"`
		ByModel       map[string]int `json:"by_model"`
		ByContainer   map[string]int `json:"by_container"`
	}

	stats := Stats{
		ByModel:     make(map[string]int),
		ByContainer: make(map[string]int),
	}

	// Total stats
	row := db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cached_tokens), 0)
		FROM requests
	`)
	row.Scan(&stats.TotalRequests, &stats.TotalInput, &stats.TotalOutput, &stats.TotalCached)

	// By model
	rows, _ := db.Query(`SELECT model, SUM(input_tokens + output_tokens) FROM requests GROUP BY model`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var model string
			var tokens int
			rows.Scan(&model, &tokens)
			stats.ByModel[model] = tokens
		}
	}

	// By container
	rows, _ = db.Query(`SELECT container, SUM(input_tokens + output_tokens) FROM requests GROUP BY container`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var container string
			var tokens int
			rows.Scan(&container, &tokens)
			stats.ByContainer[container] = tokens
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Printf("Starting URP proxy on %s -> %s", listenAddr, upstream)

	initDB()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/stats", statsHandler)
	mux.HandleFunc("/", proxyHandler)

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
