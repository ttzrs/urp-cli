package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"prueba-orquestador/internal/health/usecase"
	"testing"
)

func TestHealthHandler_HandleHealth(t *testing.T) {
	// Setup
	useCase := usecase.NewHealthUseCase()
	handler := NewHealthHandler(useCase)
	
	// Create request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	
	// Execute
	handler.HandleHealth(w, req)
	
	// Assert status code
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	// Assert content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected content type 'application/json', got '%s'", contentType)
	}
	
	// Assert response body
	var response usecase.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response.Status)
	}
}
