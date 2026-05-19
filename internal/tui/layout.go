package tui

import (
	"strings"

	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	contextBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	dimmedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderLayout(m *model) string {
	// 1. History Buffer
	historyView := lipgloss.NewStyle().Height(m.viewport.Height).Render(m.viewport.View())
	
		var suggestionsBox string
	if m.showSuggestions && len(m.filteredCommands) > 0 {
		var sb strings.Builder
		sb.WriteString("--- Commands ---\n")
		
		start := m.commandCursor - 2
		if start < 0 {
			start = 0
		}
		end := start + 5
		if end > len(m.filteredCommands) {
			end = len(m.filteredCommands)
			start = end - 5
			if start < 0 {
				start = 0
			}
		}

		if start > 0 {
			sb.WriteString("  ↑ ...\n")
		}
		for i := start; i < end; i++ {
			cmd := m.filteredCommands[i]
			cursor := "  "
			if i == m.commandCursor {
				cursor = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%s\t- %s\n", cursor, cmd.Name, cmd.Description))
		}
		if end < len(m.filteredCommands) {
			sb.WriteString("  ↓ ...\n")
		}
		sb.WriteString("(tab to autocomplete, up/down to navigate)")
		
		suggestionsBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Background(lipgloss.Color("236")).
			Render(sb.String())
			
		// Float it at the bottom left of historyView
		historyView = lipgloss.Place(m.width, m.viewport.Height, lipgloss.Left, lipgloss.Bottom, suggestionsBox, lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("238")))
	}
	
	if m.showModal || m.currentState == stateProviderSelect || m.currentState == stateModelSelect || m.currentState == stateHistory || m.currentState == stateStatus || m.currentState == stateError || m.currentState == stateDoc {
		historyView = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(historyView)
	}

	// 2. Context Bar
	activeModel := m.cfg.ActiveProvider.Model
	if activeModel == "" {
		activeModel = "default"
	}
	
	contextStr := fmt.Sprintf("ovty - %s | %s", m.cfg.ActiveProvider.Name, activeModel)
	if m.systemProfile != nil {
		contextStr = fmt.Sprintf("ovty - %s | %s | %s | %s", m.systemProfile.OSRelease, m.systemProfile.InitSystem, m.cfg.ActiveProvider.Name, activeModel)
	}
	
	contextBar := contextBarStyle.Render(contextStr)
	
		// 3. Input
	var inputView string
	if m.currentState == stateAnalysis {
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(fmt.Sprintf("%s Analyzing system state and intent...", m.spinner.View()))
	} else if m.currentState == stateFetchingModels {
		inputView = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(fmt.Sprintf("%s Fetching available models...", m.spinner.View()))
	} else if m.currentState == stateProviderSelect {
		inputView = "Select Model Provider: (Use arrow keys and enter in a moment...)" // Stub for now
	} else {
		inputView = m.textarea.View()
	}
	
	if m.showModal || m.currentState == stateProviderSelect || m.currentState == stateModelSelect || m.currentState == stateHistory || m.currentState == stateStatus || m.currentState == stateError || m.currentState == stateDoc {
		// Render Modal overlay
		var modal string
		if m.currentState == stateStaging {
			modal = renderStagingView(m)
		} else if m.currentState == stateExecution {
			modal = "Executing...\n" + m.executionLogs
		} else if m.currentState == stateProviderSelect {
			var b strings.Builder
			b.WriteString(titleStyle.Render("Select Model Provider:") + "\n\n")
			for i, p := range m.providers {
				cursor := "  "
				if m.providerCursor == i {
					cursor = "> "
				}
				b.WriteString(fmt.Sprintf("%s%s\n", cursor, p))
			}
			b.WriteString("\n" + dimmedStyle.Render("(up/down to navigate, enter to select)"))
			modal = b.String()
		} else if m.currentState == stateModelSelect {
			var b strings.Builder
			b.WriteString(titleStyle.Render("Select Model:") + "\n\n")
			
			start := m.modelCursor - 5
			if start < 0 {
				start = 0
			}
			end := start + 10
			if end > len(m.fetchedModels) {
				end = len(m.fetchedModels)
				start = end - 10
				if start < 0 {
					start = 0
				}
			}
			if start > 0 {
				b.WriteString("  ↑ ...\n")
			}
			for i := start; i < end; i++ {
				p := m.fetchedModels[i]
				cursor := "  "
				if m.modelCursor == i {
					cursor = "> "
				}
				b.WriteString(fmt.Sprintf("%s%s\n", cursor, p))
			}
			if end < len(m.fetchedModels) {
				b.WriteString("  ↓ ...\n")
			}
			b.WriteString("\n" + dimmedStyle.Render("(up/down to navigate, enter to select)"))
			modal = b.String()
		} else if m.currentState == stateHistory {
			modal = titleStyle.Render("History:") + "\n\n"
			
			start := 0
			if len(m.history) > 10 {
				start = len(m.history) - 10
				modal += "  ↑ ... (earlier history hidden)\n"
			}
			
			for i := start; i < len(m.history); i++ {
				modal += fmt.Sprintf("[%d] %s\n", i+1, m.history[i])
			}
			if len(m.history) == 0 {
				modal += "No history available.\n"
			}
			modal += "\n" + dimmedStyle.Render("(esc to close)")
		} else if m.currentState == stateStatus {
			modal = titleStyle.Render("System Status:") + "\n\n"
			if m.systemProfile != nil {
				modal += fmt.Sprintf("OS: %s\nInit: %s\nProvider: %s\nModel: %s\n",
					m.systemProfile.OSRelease, m.systemProfile.InitSystem, m.cfg.ActiveProvider.Name, m.cfg.ActiveProvider.Model)
			} else {
				modal += "Loading..."
			}
			modal += "\n" + dimmedStyle.Render("(esc to close)")
		} else if m.currentState == stateError || m.currentState == stateDoc {
			modal = titleStyle.Render("Error:") + "\n\n"
			if m.err != nil {
				modal += m.err.Error()
			}
			modal += "\n\n" + dimmedStyle.Render("(esc to close)")
		} else {
			modal = "Loading..."
		}
		
		modalBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).BorderForeground(lipgloss.Color("205")).Render(modal)
		
		// We can't do true floating over viewport easily without Place(), so we just replace the viewport for now
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.Place(m.width, m.height-m.textarea.Height()-2, lipgloss.Center, lipgloss.Center, modalBox),
			contextBar,
			inputView,
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		historyView,
		contextBar,
		inputView,
	)
}
