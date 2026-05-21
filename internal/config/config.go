package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	Name   string `json:"name"`
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model,omitempty"`
}

type Config struct {
	ActiveProvider ProviderConfig `json:"active_provider"`
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ovty_config.json"
	}
	return filepath.Join(home, ".ovty", "config.json")
}

func LoadConfig() (*Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config
			return &Config{
				ActiveProvider: ProviderConfig{
					Name: "Ollama",
				},
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
