package llm

import "github.com/onlyin32bit/ovty/internal/engine"

// Provider defines the interface for different LLM providers (Adapter Pattern)
type Provider interface {
	Name() string
	Generate(prompt string) (*engine.LLMResponse, error)
	ListModels() ([]string, error)
}

func NewProvider(name, apiKey, model string) Provider {
	switch name {
	case "OpenAI":
		return &OpenAIAdapter{APIKey: apiKey, Model: model}
	case "Gemini":
		return &GeminiAdapter{APIKey: apiKey, Model: model}
	case "OpenRouter":
		return &OpenRouterAdapter{APIKey: apiKey, Model: model}
	case "Ollama":
		fallthrough
	default:
		return &OllamaAdapter{Model: model}
	}
}
