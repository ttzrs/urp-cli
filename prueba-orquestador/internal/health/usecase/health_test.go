package usecase

import "testing"

func TestHealthUseCase_Check(t *testing.T) {
	useCase := NewHealthUseCase()
	
	response := useCase.Check()
	
	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response.Status)
	}
}
