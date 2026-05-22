package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/onlyin32bit/ovty/internal/engine"
)

// Base implementation to parse response
func parseResponse(responseStr string) (*engine.LLMResponse, error) {
	// Simple cleanup: sometimes LLMs return markdown block around JSON
	responseStr = strings.TrimSpace(responseStr)
	if strings.HasPrefix(responseStr, "```json") {
		responseStr = strings.TrimPrefix(responseStr, "```json")
		responseStr = strings.TrimSuffix(responseStr, "```")
	} else if strings.HasPrefix(responseStr, "```") {
		responseStr = strings.TrimPrefix(responseStr, "```")
		responseStr = strings.TrimSuffix(responseStr, "```")
	}

	var llmResp engine.LLMResponse
	if err := json.Unmarshal([]byte(responseStr), &llmResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON contract: %w", err)
	}
	return &llmResp, nil
}

// ---------------- Ollama Adapter ----------------

type OllamaAdapter struct {
	Endpoint string
	Model    string
}

func (a *OllamaAdapter) Name() string { return "Ollama" }

func (a *OllamaAdapter) Generate(prompt string) (*engine.LLMResponse, error) {
	endpoint := a.Endpoint
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434/api/generate"
	}
	model := a.Model
	if model == "" {
		model = "llama3"
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"format": "json",
		"stream": false,
	})

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	responseString, _ := result["response"].(string)
	return parseResponse(responseString)
}

func (a *OllamaAdapter) ListModels() ([]string, error) {
	endpoint := "http://127.0.0.1:11434/api/tags"
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Models {
		models = append(models, m.Name)
	}
	return models, nil
}

// ---------------- OpenAI Adapter ----------------

type OpenAIAdapter struct {
	APIKey string
	Model  string
}

func (a *OpenAIAdapter) Name() string { return "OpenAI" }

func (a *OpenAIAdapter) Generate(prompt string) (*engine.LLMResponse, error) {
	return generateOpenAICompatible("https://api.openai.com/v1/chat/completions", a.APIKey, a.Model, prompt)
}

func (a *OpenAIAdapter) ListModels() ([]string, error) {
	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+a.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Data {
		if strings.HasPrefix(m.ID, "gpt") { // Filter to GPT models roughly
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		models = []string{"gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo"}
	}
	return models, nil
}

// ---------------- OpenRouter Adapter ----------------

type OpenRouterAdapter struct {
	APIKey string
	Model  string
}

func (a *OpenRouterAdapter) Name() string { return "OpenRouter" }

func (a *OpenRouterAdapter) Generate(prompt string) (*engine.LLMResponse, error) {
	return generateOpenAICompatible("https://openrouter.ai/api/v1/chat/completions", a.APIKey, a.Model, prompt)
}

func (a *OpenRouterAdapter) ListModels() ([]string, error) {
	resp, err := http.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// ---------------- Gemini Adapter ----------------

type GeminiAdapter struct {
	APIKey string
	Model  string
}

func (a *GeminiAdapter) Name() string { return "Gemini" }

func (a *GeminiAdapter) Generate(prompt string) (*engine.LLMResponse, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", a.Model, a.APIKey)
	
	reqBody, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt + "\n\nRespond strictly in JSON matching the requested contract."},
				},
			},
		},
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
    
	bodyBytes, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, err
	}
    
    candidates, ok := result["candidates"].([]interface{})
    if !ok || len(candidates) == 0 {
        return nil, fmt.Errorf("no candidates in Gemini response: %s", string(bodyBytes))
    }
    cand := candidates[0].(map[string]interface{})
    content := cand["content"].(map[string]interface{})
    parts := content["parts"].([]interface{})
    text := parts[0].(map[string]interface{})["text"].(string)

	return parseResponse(text)
}

func (a *GeminiAdapter) ListModels() ([]string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", a.APIKey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Models {
		// Clean up "models/" prefix
		name := strings.TrimPrefix(m.Name, "models/")
		models = append(models, name)
	}
	return models, nil
}

// Helper for OpenAI-compatible endpoints
func generateOpenAICompatible(endpoint, apiKey, model, prompt string) (*engine.LLMResponse, error) {
	if model == "" {
		model = "gpt-4o"
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful system administrator. Always output strictly in the requested JSON format."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	})

	req, _ := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	bodyBytes, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, err
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response: %s", string(bodyBytes))
	}

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	text := message["content"].(string)

	return parseResponse(text)
}
