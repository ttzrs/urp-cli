package ingestor

import (
	"context"
	"fmt"
	"time"
)

// PDFProcessor simulates the "Visual/Textual Ingestor" (e.g., ColPali/Gemini Flash).
type PDFProcessor struct {
	// In the future, this will hold the API client
}

func NewPDFProcessor() *PDFProcessor {
	return &PDFProcessor{}
}

func (p *PDFProcessor) ProcessDocument(ctx context.Context, filePath string) ([]KnowledgeChunk, error) {
	fmt.Printf("[Ingestor] Processing PDF: %s\n", filePath)
	
	// Simulate processing time
	time.Sleep(500 * time.Millisecond)

	// SIMULATION: Hardcoded extraction based on the architecture plan
	// "Extrae causalidad teórica (Φ): El manual dice que LI1 activa el motor"
	
	chunks := []KnowledgeChunk{
		{
			Source:      fmt.Sprintf("%s - Page 40", filePath),
			Proposition: "IF LI1=TRUE THEN Motor=RUN",
			Entities:    []string{"LI1", "Motor"},
			Confidence:  0.6, // Low confidence because it's just theory, not validated
		},
		{
			Source:      fmt.Sprintf("%s - Page 42", filePath),
			Proposition: "IF Jumper_J2=OPEN THEN SafeMode=ACTIVE",
			Entities:    []string{"Jumper_J2", "SafeMode"},
			Confidence:  0.8,
		},
	}

	return chunks, nil
}
