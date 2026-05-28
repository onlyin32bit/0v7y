package engine

import (
	"os"
	"path/filepath"
	"strings"
)

// Manifest represents a local RAG document
type Manifest struct {
	Name    string
	Content string
}

// RAGEngine handles the local docs cache retrieval
type RAGEngine struct {
	DocsCacheDir string
}

// NewRAGEngine initializes the local RAG
func NewRAGEngine(cacheDir string) *RAGEngine {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".ovty", "docs_cache")
	}
	return &RAGEngine{
		DocsCacheDir: cacheDir,
	}
}

// Retrieve matching manifests based on simple keyword heuristics
// In a production app, this would use an inverted index or bm25, but 
// the prompt implies a "zero-vector" approach.
func (r *RAGEngine) Retrieve(prompt string) ([]Manifest, error) {
	var manifests []Manifest

	// Ensure dir exists
	if _, err := os.Stat(r.DocsCacheDir); os.IsNotExist(err) {
		return manifests, nil // No cache yet
	}

	files, err := os.ReadDir(r.DocsCacheDir)
	if err != nil {
		return nil, err
	}

	promptLower := strings.ToLower(prompt)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		// Basic keyword matching from filename
		name := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
		if strings.Contains(promptLower, strings.ToLower(name)) {
			content, err := os.ReadFile(filepath.Join(r.DocsCacheDir, f.Name()))
			if err == nil {
				manifests = append(manifests, Manifest{
					Name:    f.Name(),
					Content: string(content),
				})
			}
		}
	}
	return manifests, nil
}
