package state

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing"
)

// StateManager handles embedded version control for system files
type StateManager struct {
	RepoPath string
	repo     *git.Repository
}

// NewStateManager initializes or opens the hidden git repository in ~/.ovty/state
func NewStateManager() (*StateManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	repoPath := filepath.Join(home, ".ovty", "state")

	var repo *git.Repository
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		err = os.MkdirAll(repoPath, 0700)
		if err != nil {
			return nil, err
		}
		repo, err = git.PlainInit(repoPath, false)
		if err != nil {
			return nil, err
		}
	} else {
		repo, err = git.PlainOpen(repoPath)
		if err != nil {
			return nil, err
		}
	}

	return &StateManager{
		RepoPath: repoPath,
		repo:     repo,
	}, nil
}

// Snapshot copies a system file to the state directory and commits it
func (sm *StateManager) Snapshot(sysFilePath string, prompt string) error {
	wt, err := sm.repo.Worktree()
	if err != nil {
		return err
	}

	// Make sure file exists before reading
	if _, err := os.Stat(sysFilePath); os.IsNotExist(err) {
		// If file doesn't exist, we can't snapshot it, but that's fine for new files.
		return nil
	}

	data, err := os.ReadFile(sysFilePath)
	if err != nil {
		return err
	}

	relPath := filepath.Clean(sysFilePath)
	if strings.HasPrefix(relPath, "/") {
		relPath = relPath[1:]
	}
	destPath := filepath.Join(sm.RepoPath, relPath)
	
	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return err
	}

	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return err
	}

	if _, err := wt.Add(relPath); err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("auto: pre-flight state before <%s>", prompt)
	_, err = wt.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "ovty",
			Email: "ovty@localhost",
			When:  time.Now(),
		},
	})
	return err
}

// GetHistory retrieves the commit history
func (sm *StateManager) GetHistory() ([]string, error) {
	ref, err := sm.repo.Head()
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return []string{"(No commits yet)"}, nil
		}
		return nil, err
	}

	cIter, err := sm.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	var history []string
	err = cIter.ForEach(func(c *object.Commit) error {
		history = append(history, fmt.Sprintf("%s - %s", c.Hash.String()[:7], strings.TrimSpace(c.Message)))
		return nil
	})
	if err != nil {
		return nil, err
	}
	
	if len(history) == 0 {
		return []string{"(No commits yet)"}, nil
	}
	
	return history, nil
}

// Rollback restores the previous state by checking out HEAD~1 and restoring the specified files
func (sm *StateManager) Rollback(files []string) error {
	wt, err := sm.repo.Worktree()
	if err != nil {
		return err
	}
	
	h, err := sm.repo.ResolveRevision(plumbing.Revision("HEAD^"))
	if err != nil {
		return fmt.Errorf("could not find previous commit: %w", err)
	}
	
	err = wt.Reset(&git.ResetOptions{
		Mode: git.HardReset,
		Commit: *h,
	})
	if err != nil {
		return err
	}
	
	// Physically copy files back to host system using sudo
	for _, file := range files {
		relPath := filepath.Clean(file)
		if strings.HasPrefix(relPath, "/") {
			relPath = relPath[1:]
		}
		repoFilePath := filepath.Join(sm.RepoPath, relPath)
		
		// Create a privileged command to copy it back
		// Note: since git.go shouldn't import engine directly to avoid circular imports,
		// we use exec directly here
		if _, err := os.Stat(repoFilePath); os.IsNotExist(err) {
			// Delete on host
			cmd := exec.Command("sudo", "rm", "-f", file)
			_ = cmd.Run()
		} else {
			// Copy back to host
			cmdMkdir := exec.Command("sudo", "mkdir", "-p", filepath.Dir(file))
			_ = cmdMkdir.Run()
			
			cmdCp := exec.Command("sudo", "cp", "-a", repoFilePath, file)
			_ = cmdCp.Run()
		}
	}
	
	return nil
}
