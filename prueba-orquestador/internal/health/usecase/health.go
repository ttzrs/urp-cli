package usecase

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
}

// HealthUseCase handles health check business logic
type HealthUseCase struct{}

// NewHealthUseCase creates a new health use case
func NewHealthUseCase() *HealthUseCase {
	return &HealthUseCase{}
}

// Check performs a health check
func (h *HealthUseCase) Check() HealthResponse {
	return HealthResponse{
		Status: "ok",
	}
}
