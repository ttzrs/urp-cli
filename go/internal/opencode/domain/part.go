package domain

import (
	"encoding/json"
	"fmt"
)

// PartType constants for JSON serialization
const (
	PartTypeText     = "text"
	PartTypeReasoning = "reasoning"
	PartTypeToolCall = "tool_call"
	PartTypeFile     = "file"
	PartTypeImage    = "image"
)

// PartWrapper handles JSON serialization of Part interface
type PartWrapper struct {
	Type string `json:"type"`
	Part Part   `json:"-"`
}

// MarshalJSON serializes a Part with its type
func (w PartWrapper) MarshalJSON() ([]byte, error) {
	type wrapper struct {
		Type string `json:"type"`
	}

	// Get the underlying data
	var data []byte
	var err error

	switch p := w.Part.(type) {
	case TextPart:
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			TextPart
		}{PartTypeText, p})
	case ReasoningPart:
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			ReasoningPart
		}{PartTypeReasoning, p})
	case ToolCallPart:
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			ToolCallPart
		}{PartTypeToolCall, p})
	case FilePart:
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			FilePart
		}{PartTypeFile, p})
	case ImagePart:
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			ImagePart
		}{PartTypeImage, p})
	default:
		return nil, fmt.Errorf("unknown part type: %T", w.Part)
	}

	return data, err
}

// UnmarshalPart deserializes JSON into the appropriate Part type
func UnmarshalPart(data []byte) (Part, error) {
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return nil, err
	}

	switch typeCheck.Type {
	case PartTypeText, "":
		var p TextPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case PartTypeReasoning:
		var p ReasoningPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case PartTypeToolCall:
		var p ToolCallPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case PartTypeFile:
		var p FilePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case PartTypeImage:
		var p ImagePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown part type: %s", typeCheck.Type)
	}
}

// UnmarshalParts deserializes a JSON array of parts
func UnmarshalParts(data []byte) ([]Part, error) {
	var rawParts []json.RawMessage
	if err := json.Unmarshal(data, &rawParts); err != nil {
		return nil, err
	}

	parts := make([]Part, 0, len(rawParts))
	for _, raw := range rawParts {
		part, err := UnmarshalPart(raw)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, nil
}

// MarshalParts serializes parts with type information
func MarshalParts(parts []Part) ([]byte, error) {
	wrappers := make([]PartWrapper, len(parts))
	for i, p := range parts {
		wrappers[i] = PartWrapper{Part: p}
	}
	return json.Marshal(wrappers)
}
