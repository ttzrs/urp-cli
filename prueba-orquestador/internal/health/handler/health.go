package handler

import (
	"encoding/json"
	"net/http"
	"prueba-orquestador/internal/health/usecase"
)

// HealthHandler handles HTTP requests for health checks
type HealthHandler struct {
	useCase *usecase.HealthUseCase
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(useCase *usecase.HealthUseCase) *HealthHandler {
	return &HealthHandler{
		useCase: useCase,
	}
}

// HandleHealth handles the /health endpoint
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	response := h.useCase.Check()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
