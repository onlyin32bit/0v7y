package engine

import (
	"fmt"
	"os/exec"
	"strings"
)

var blacklistedCommands = []string{"rm", "mv", "wget", "curl", ">", ">>", "chmod", "chown", "reboot", "shutdown"}

// ValidateObservabilityCommands checks the read_commands against a blacklist
func ValidateObservabilityCommands(cmds [][]string) error {
	for _, cmdParts := range cmds {
		for _, part := range cmdParts {
			for _, blacklist := range blacklistedCommands {
				if part == blacklist {
					return fmt.Errorf("security violation: observability command contains blacklisted token '%s'", blacklist)
				}
			}
		}
	}
	return nil
}

// ExecuteObservability runs read-only commands using sh -c to support pipes
func ExecuteObservability(cmds [][]string) (string, error) {
	if err := ValidateObservabilityCommands(cmds); err != nil {
		return "", err
	}

	var results strings.Builder
	for _, cmdParts := range cmds {
		if len(cmdParts) == 0 {
			continue
		}
		
		fullCommand := strings.Join(cmdParts, " ")
		cmd := exec.Command("sh", "-c", fullCommand)
		
		out, err := cmd.CombinedOutput()
		if err != nil {
			results.WriteString(fmt.Sprintf("Error executing '%s': %v\n", fullCommand, string(out)))
		} else {
			results.WriteString(fmt.Sprintf("Output of '%s':\n%s\n", fullCommand, string(out)))
		}
	}

	return results.String(), nil
}
