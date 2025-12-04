package domain

import (
	"time"
)

// Message represents a single message in a session
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionID"`
	Role      Role      `json:"role"`
	Parts     []Part    `json:"parts"`
	Timestamp time.Time `json:"timestamp"`
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Part represents content within a message
type Part interface {
	PartType() string
}

type TextPart struct {
	Text string `json:"text"`
}

func (p TextPart) PartType() string { return "text" }

type ReasoningPart struct {
	Text string `json:"text"`
}

func (p ReasoningPart) PartType() string { return "reasoning" }

type ToolCallPart struct {
	ToolID   string         `json:"toolID"`
	Name     string         `json:"name"`
	Args     map[string]any `json:"args"`
	Result   string         `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
	Duration time.Duration  `json:"duration,omitempty"`
}

func (p ToolCallPart) PartType() string { return "tool_call" }

type FilePart struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language,omitempty"`
}

func (p FilePart) PartType() string { return "file" }

type ImagePart struct {
	Data      []byte `json:"-"`              // Binary data
	Base64    string `json:"base64"`         // Base64 encoded
	MediaType string `json:"mediaType"`      // image/png, image/jpeg, etc
	Path      string `json:"path,omitempty"`
}

func (p ImagePart) PartType() string { return "image" }
