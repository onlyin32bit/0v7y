package knowledge

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const RepoBaseURL = "https://raw.githubusercontent.com/onlyin32bit/ovty-knowledge/main/"

// GetCacheDir returns the absolute path to the local knowledge cache
func GetCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ovty", "knowledge")
}

// FetchToolDoc attempts to read a tool's documentation from the local cache.
// If it does not exist, it fetches it dynamically from the remote repository.
func FetchToolDoc(tool string) (string, error) {
	cacheDir := GetCacheDir()
	os.MkdirAll(cacheDir, 0755)

	filePath := filepath.Join(cacheDir, fmt.Sprintf("%s.md", tool))

	// 1. Try Local Cache
	data, err := os.ReadFile(filePath)
	if err == nil {
		return string(data), nil
	}

	// 2. Fetch from Remote
	url := fmt.Sprintf("%s%s.md", RepoBaseURL, tool)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to reach knowledge repository: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("documentation for '%s' not found in remote repository", tool)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch documentation: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 3. Save to Local Cache
	_ = os.WriteFile(filePath, body, 0644)

	return string(body), nil
}
