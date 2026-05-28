package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Mutation represents an internal file change action
type Mutation struct {
	FilePath string
	Action   string
	Content  string
}

// ExecuteMutation applies the mutation to a file (preserving permissions/ownership)
func ExecuteMutation(m Mutation) error {
	// Read existing file stat if it exists
	var mode os.FileMode = 0644
	var uid, gid int = 0, 0
	
	info, err := os.Stat(m.FilePath)
	if err == nil {
		mode = info.Mode()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid = int(stat.Uid)
			gid = int(stat.Gid)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Create temp file for atomic write
	tmpFile := m.FilePath + ".ovty.tmp"
	
	switch m.Action {
	case "append":
		data, err := os.ReadFile(m.FilePath)
		var newContent []byte
		if err == nil {
			newContent = append(data, []byte("\n"+m.Content)...)
		} else {
			newContent = []byte(m.Content)
		}
		if err := os.WriteFile(tmpFile, newContent, mode); err != nil {
			return err
		}
	case "overwrite":
		if err := os.WriteFile(tmpFile, []byte(m.Content), mode); err != nil {
			return err
		}
	case "delete":
		return RunPrivileged("rm", "-f", m.FilePath)
	default:
		return fmt.Errorf("unknown action: %s", m.Action)
	}

	// Attempt to chown back to original (might fail if not root)
	_ = os.Chown(tmpFile, uid, gid)

	// Atomically move the tmp file over the original file
	// If it fails due to permissions, fallback to sudo
	err = os.Rename(tmpFile, m.FilePath)
	if err != nil && os.IsPermission(err) {
		// Try using sudo to mv the file
		err = RunPrivileged("mv", tmpFile, m.FilePath)
		if err != nil {
			// Clean up temp file
			_ = os.Remove(tmpFile)
			return err
		}
	} else if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	return nil
}

// RunPrivileged attempts to run a command, falling back to sudo if not root
func RunPrivileged(name string, arg ...string) error {
	if os.Geteuid() != 0 {
		sudoArgs := append([]string{name}, arg...)
		cmd := exec.Command("sudo", sudoArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("sudo %s failed: %s", name, string(out))
		}
		return nil
	}
	cmd := exec.Command(name, arg...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s", name, string(out))
	}
	return nil
}

// Validate executes the validation command defined in the JSON
func Validate(cmdArgs []string) (string, error) {
	if len(cmdArgs) == 0 {
		return "", nil
	}
	
	var cmd *exec.Cmd
	if os.Geteuid() != 0 {
		sudoArgs := append([]string{cmdArgs[0]}, cmdArgs[1:]...)
		cmd = exec.Command("sudo", sudoArgs...)
	} else {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", string(out), err)
	}
	return string(out), nil
}

// Commit executes the service restart/reload command
func Commit(cmdArgs []string) (string, error) {
	if len(cmdArgs) == 0 {
		return "", nil
	}
	
	var cmd *exec.Cmd
	if os.Geteuid() != 0 {
		sudoArgs := append([]string{cmdArgs[0]}, cmdArgs[1:]...)
		cmd = exec.Command("sudo", sudoArgs...)
	} else {
		cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", string(out), err)
	}
	return string(out), nil
}

// RestoreFile uses sudo to safely copy a file from the ovty state to the system
func RestoreFile(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		// File was deleted in rollback, so we delete it on host
		return RunPrivileged("rm", "-f", dst)
	}
	
	// Create parent dirs if needed
	parent := filepath.Dir(dst)
	_ = RunPrivileged("mkdir", "-p", parent)
	
	// Copy file back
	return RunPrivileged("cp", "-a", src, dst)
}
