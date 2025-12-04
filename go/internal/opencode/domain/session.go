package domain

import (
	"time"
)

// Session represents a conversation session with an AI agent
type Session struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectID"`
	Directory string    `json:"directory"`
	ParentID  string    `json:"parentID,omitempty"`
	Title     string    `json:"title"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Summary   *Summary  `json:"summary,omitempty"`
}

type Summary struct {
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	Files     []string `json:"files"`
}
