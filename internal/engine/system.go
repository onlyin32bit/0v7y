package engine

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// SystemProfile holds the dynamic environment context
type SystemProfile struct {
	OSRelease     string
	InitSystem    string // e.g., "systemd" or "openrc"
	NetworkInfo   string
	TargetConfig  string
	TargetRAG     string
	DetectedTools []string
	MissingTools  []string
}

var knownTools = []string{"nginx", "caddy", "wireguard", "wg", "docker", "gitea", "ufw", "iptables"}

// ProfileSystem gathers OS, init system, and network topology concurrently
func ProfileSystem(prompt string, ragCacheDir string) (*SystemProfile, error) {
	prof := &SystemProfile{}

	tools := ExtractTools(prompt)
	prof.DetectedTools = tools
	
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			prof.MissingTools = append(prof.MissingTools, tool)
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// 1. Configs & RAG
	wg.Add(1)
	go func() {
		defer wg.Done()
		var configData strings.Builder
		var ragData strings.Builder
		for _, tool := range tools {
			switch tool {
			case "caddy":
				if data, err := os.ReadFile("/etc/caddy/Caddyfile"); err == nil {
					configData.WriteString(fmt.Sprintf("\n--- /etc/caddy/Caddyfile ---\n%s\n", string(data)))
				}
			case "nginx":
				if data, err := os.ReadFile("/etc/nginx/nginx.conf"); err == nil {
					configData.WriteString(fmt.Sprintf("\n--- /etc/nginx/nginx.conf ---\n%s\n", string(data)))
				}
			}

			// RAG
			if ragCacheDir != "" {
				ragPath := fmt.Sprintf("%s/%s.md", ragCacheDir, tool)
				if data, err := os.ReadFile(ragPath); err == nil {
					ragData.WriteString(fmt.Sprintf("\n--- RAG Manifest for %s ---\n%s\n", tool, string(data)))
				}
			}
		}
		mu.Lock()
		prof.TargetConfig += configData.String()
		prof.TargetRAG += ragData.String()
		mu.Unlock()
	}()

	// 2. OS Profile
	wg.Add(1)
	go func() {
		defer wg.Done()
		osRel := ""
		initSys := "unknown"
		
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			osRel = string(data)
		}

		if _, err := os.Stat("/run/systemd/system"); err == nil {
			initSys = "systemd"
		} else if _, err := exec.LookPath("rc-service"); err == nil {
			initSys = "openrc"
		}
		
		mu.Lock()
		prof.OSRelease = osRel
		prof.InitSystem = initSys
		mu.Unlock()
	}()

	// 3. Network Topology
	wg.Add(1)
	go func() {
		defer wg.Done()
		var netInfo strings.Builder
		if out, err := exec.Command("ip", "link").Output(); err == nil {
			netInfo.WriteString(string(out))
		}
		if out, err := exec.Command("ss", "-tulpn").Output(); err == nil {
			netInfo.WriteString("\n")
			netInfo.WriteString(string(out))
		}
		
		mu.Lock()
		prof.NetworkInfo += netInfo.String()
		mu.Unlock()
	}()

	wg.Wait()
	return prof, nil
}

// ExtractTools finds potential system tools mentioned in the prompt
func ExtractTools(prompt string) []string {
	prompt = strings.ToLower(prompt)
	var found []string
	
	// Tokenize prompt to find whole words
	re := regexp.MustCompile(`\b\w+\b`)
	words := re.FindAllString(prompt, -1)
	
	wordMap := make(map[string]bool)
	for _, w := range words {
		wordMap[w] = true
	}
	
	for _, t := range knownTools {
		if wordMap[t] {
			found = append(found, t)
		}
	}
	return found
}
