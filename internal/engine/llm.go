package engine

import (
	"encoding/base64"
)

type Status struct {
	IntentType       string   `json:"intent_type"` // "OBSERVABILITY" or "MUTATION"
	RequiresPackages bool     `json:"requires_packages"`
	Warnings         []string `json:"warnings"`
}

type Operation struct {
	StepOrder     int      `json:"step_order"`
	Type          string   `json:"type"` // "SHELL_EXECUTION" or "FILE_MUTATION"
	Target        string   `json:"target"`
	Command       []string `json:"command"`
	ActionEnum    string   `json:"action_enum"` // "APPEND", "OVERWRITE", "DELETE"
	Base64Content string   `json:"base64_content"`
	Reasoning     string   `json:"reasoning"`
}

func (o *Operation) DecodedContent() string {
	if o.Base64Content == "" {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(o.Base64Content)
	if err != nil {
		return ""
	}
	return string(data)
}

type LLMResponse struct {
	Status     Status      `json:"status"`
	Operations []Operation `json:"operations"`
}
