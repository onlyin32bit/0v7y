package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var (
	diffAdditionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	diffDeletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Width(50)
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)

// renderStagingView constructs the split pane view
func renderStagingView(m *model) string {
	if m.llmResponse == nil {
		return "Waiting for LLM response..."
	}

	var header strings.Builder
	if len(m.llmResponse.Status.Warnings) > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("220")).Bold(true).Padding(0, 1)
		header.WriteString(warnStyle.Render("⚠️ HEADS UP: Agent Warnings") + "\n")
		for _, w := range m.llmResponse.Status.Warnings {
			header.WriteString(" - " + w + "\n")
		}
		header.WriteString("\n")
	}

	if m.showReasoning {
		var auditPane strings.Builder
		auditPane.WriteString(titleStyle.Render("Audit Trail (Reasoning)") + "\n\n")
		
		for _, op := range m.llmResponse.Operations {
			if op.Reasoning != "" {
				auditPane.WriteString(fmt.Sprintf("[%d] %s: %s\n> %s\n\n", op.StepOrder, op.Type, op.Target, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(op.Reasoning)))
			}
		}

		splitPane := lipgloss.JoinHorizontal(lipgloss.Top,
			paneStyle.Width(100).Render(auditPane.String()),
		)
		footer := lipgloss.NewStyle().MarginTop(1).Bold(true).Render("[Y] Commit Changes | [N] Abort & Discard | [?] Hide Reasoning")
		return header.String() + splitPane + "\n" + footer
	}

	dmp := diffmatchpatch.New()
	var leftPane strings.Builder
	
	leftPane.WriteString(titleStyle.Render("Diff View") + "\n\n")

	for _, op := range m.llmResponse.Operations {
		if op.Type == "FILE_MUTATION" {
			leftPane.WriteString(fmt.Sprintf("File: %s [%s]\n", op.Target, op.ActionEnum))
			
			var oldContent string
			if data, err := os.ReadFile(op.Target); err == nil {
				oldContent = string(data)
			}

			newContent := oldContent
			action := strings.ToLower(op.ActionEnum)
			if action == "append" {
				newContent += "\n" + op.DecodedContent()
			} else if action == "overwrite" {
				newContent = op.DecodedContent()
			} else if action == "delete" {
				newContent = ""
			}

			if oldContent == "" {
				leftPane.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render("(New file created)") + "\n")
				leftPane.WriteString(newContent)
			} else {
				diffs := dmp.DiffMain(oldContent, newContent, false)
				for _, d := range diffs {
					text := d.Text
					if d.Type == diffmatchpatch.DiffInsert {
						leftPane.WriteString(diffAdditionStyle.Render(text))
					} else if d.Type == diffmatchpatch.DiffDelete {
						leftPane.WriteString(diffDeletionStyle.Render(text))
					} else {
						leftPane.WriteString(text)
					}
				}
			}
			leftPane.WriteString("\n\n")
		}
	}

	var rightPane strings.Builder
	rightPane.WriteString(titleStyle.Render("Operations View") + "\n\n")
	
	for _, op := range m.llmResponse.Operations {
		if op.Type == "SHELL_EXECUTION" {
			rightPane.WriteString(fmt.Sprintf("[%s Command]:\n> %s\n\n", op.Target, strings.Join(op.Command, " ")))
		}
	}

	splitPane := lipgloss.JoinHorizontal(lipgloss.Top,
		paneStyle.Render(leftPane.String()),
		paneStyle.Render(rightPane.String()),
	)

	footer := lipgloss.NewStyle().MarginTop(1).Bold(true).Render("[Y] Commit Changes | [N] Abort & Discard | [?] Inspect Reasoning")
	return header.String() + splitPane + "\n" + footer
}
