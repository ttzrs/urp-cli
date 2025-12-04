package main

import (
	"fmt"
	"log"
	"net/http"
	"prueba-orquestador/internal/health/handler"
	"prueba-orquestador/internal/health/usecase"
)

func main() {
	// Initialize use case
	healthUseCase := usecase.NewHealthUseCase()
	
	// Initialize handler
	healthHandler := handler.NewHealthHandler(healthUseCase)
	
	// Setup routes
	http.HandleFunc("/health", healthHandler.HandleHealth)
	
	// Start server
	port := ":8080"
	fmt.Printf("Server starting on port %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
