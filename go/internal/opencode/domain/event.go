package domain

// StreamEvent represents events during message streaming
type StreamEvent struct {
	Type           StreamEventType    `json:"type"`
	Content        string             `json:"content,omitempty"`
	Part           Part               `json:"part,omitempty"`
	Error          error              `json:"error,omitempty"`
	Done           bool               `json:"done,omitempty"`
	Usage          *Usage             `json:"usage,omitempty"`
	PermissionReq  *PermissionRequest `json:"permissionReq,omitempty"`
	PermissionResp chan bool          `json:"-"` // response channel for permission
	GateInfo       *GateCallInfo      `json:"gateInfo,omitempty"`
}

// GateCallInfo contains information about a Gate LLM call.
type GateCallInfo struct {
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Filtered     bool   `json:"filtered"` // true if logs contained signal
	InputChars   int    `json:"input_chars"`
	OutputChars  int    `json:"output_chars"`
}

type StreamEventType string

const (
	StreamEventText          StreamEventType = "text"
	StreamEventThinking      StreamEventType = "thinking"
	StreamEventToolCall      StreamEventType = "tool_call"
	StreamEventToolDone      StreamEventType = "tool_done"
	StreamEventDone          StreamEventType = "done"
	StreamEventError         StreamEventType = "error"
	StreamEventUsage         StreamEventType = "usage"
	StreamEventPermissionAsk StreamEventType = "permission_ask"
	StreamEventGateCall      StreamEventType = "gate_call" // Gate LLM call for context compilation
)

// PermissionRequest for asking user permission
type PermissionRequest struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason"`
}
