package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/onlyin32bit/ovty/internal/config"
	"github.com/onlyin32bit/ovty/internal/engine"
	"github.com/onlyin32bit/ovty/internal/knowledge"
	"github.com/onlyin32bit/ovty/internal/llm"
	"github.com/onlyin32bit/ovty/internal/state"
)

type appState int

const (
	statePrompt appState = iota
	stateAnalysis
	stateStaging
	stateExecution
	stateProviderSelect
	stateAPIKeyInput
	stateFetchingModels
	stateModelSelect
	stateHistory
	stateStatus
	stateRollback
	stateError
	stateDoc
)

type SlashCommand struct {
	Name        string
	Description string
}

var allCommands = []SlashCommand{
	{Name: "/provider", Description: "Switch LLM provider"},
	{Name: "/model", Description: "Switch model for the current provider"},
	{Name: "/history", Description: "View history of system configuration changes"},
	{Name: "/rollback", Description: "Revert the most recent system change"},
	{Name: "/status", Description: "Show system profiling information"},
	{Name: "/clear", Description: "Reset the UI state"},
	{Name: "/doc", Description: "View knowledge base documentation for a tool (e.g. /doc docker)"},
}

type model struct {
	currentState  appState
	textarea      textarea.Model
	viewport      viewport.Model
	showModal      bool
	showReasoning  bool
	docContent     string
	width, height  int
	historyBuffer string
	spinner       spinner.Model
	err           error

	cfg            *config.Config
	providers      []string
	providerCursor int

	fetchedModels []string
	modelCursor   int

	llmResponse *engine.LLMResponse
	prompt      string

	systemProfile *engine.SystemProfile
	history       []string

	filteredCommands []SlashCommand
	commandCursor    int
	showSuggestions  bool

	executionLogs string
}

type ModelListMsg struct{ Models []string }
type SystemProfileMsg struct{ Profile *engine.SystemProfile }
type HistoryMsg struct{ History []string }
type DocMsg struct{ Content string; Err error }
type RollbackMsg struct {
	Success bool
	Err     error
}
type ObservabilityMsg struct{ Result string; Err error }
type ExecutionMsg struct{ Logs string; Err error }
type ValidationErrorMsg struct{ Err error; Stderr string; FailedCommand string }

func InitialModel() model {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = &config.Config{ActiveProvider: config.ProviderConfig{Name: "Ollama"}}
	}

	ta := textarea.New()
	ta.Placeholder = "Configure your infrastructure..."
	ta.Focus()
	ta.CharLimit = 1000

	vp := viewport.New(80, 20)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	banner := `   ____       _____     
  / __ \_   _/__  /_  __
 / / / / | / / / / / / /
/ /_/ /| |/ / / / /_/ / 
\____/ |___/ /_/\__, /  
               /____/
`
	styledBanner := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render(banner) + "\nWelcome to ovty (0v7y ~ zero-viscosity). Type / to see commands.\n"

	vp.SetContent(styledBanner)

	return model{
		currentState:   statePrompt,
		textarea:       ta,
		viewport:       vp,
		historyBuffer:  styledBanner,
		spinner:        s,
		cfg:            cfg,
		providers:      []string{"Ollama", "OpenAI", "Gemini", "OpenRouter"},
		providerCursor: 0,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func updateFilteredCommands(val string) []SlashCommand {
	var filtered []SlashCommand
	for _, cmd := range allCommands {
		if strings.HasPrefix(cmd.Name, val) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		m.textarea.SetHeight(2) // Give it 2 lines to start
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4 // Leave room for input and context bar
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.showModal {
				m.showModal = false
				m.currentState = statePrompt
				m.llmResponse = nil
				return m, nil
			}
			if m.currentState == stateHistory || m.currentState == stateStatus || m.currentState == stateExecution || m.currentState == stateRollback || m.currentState == stateError {
				m.currentState = statePrompt
				m.textarea.SetValue("")
				m.executionLogs = ""
				m.err = nil
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyCtrlR:
			m.historyBuffer += "\n[Fuzzy-finder stub: Ctrl+R triggered]\n"
			m.viewport.SetContent(m.historyBuffer)
			m.viewport.GotoBottom()
			return m, nil
		case tea.KeyCtrlZ:
			m.historyBuffer += "\n[Rollback stub: Ctrl+Z triggered]\n"
			m.viewport.SetContent(m.historyBuffer)
			m.viewport.GotoBottom()
			return m, nil

		case tea.KeyTab:
			if m.currentState == statePrompt && m.showSuggestions && len(m.filteredCommands) > 0 {
				m.textarea.SetValue(m.filteredCommands[m.commandCursor].Name + " ")
				m.textarea.SetCursor(len(m.textarea.Value()))
				m.showSuggestions = false
				return m, nil
			}

		case tea.KeyUp, tea.KeyDown:
			if m.currentState == statePrompt && m.showSuggestions {
				if msg.Type == tea.KeyUp && m.commandCursor > 0 {
					m.commandCursor--
				} else if msg.Type == tea.KeyDown && m.commandCursor < len(m.filteredCommands)-1 {
					m.commandCursor++
				}
				return m, nil
			} else if m.currentState == stateProviderSelect {
				if msg.Type == tea.KeyUp && m.providerCursor > 0 {
					m.providerCursor--
				} else if msg.Type == tea.KeyDown && m.providerCursor < len(m.providers)-1 {
					m.providerCursor++
				}
			} else if m.currentState == stateModelSelect {
				if msg.Type == tea.KeyUp && m.modelCursor > 0 {
					m.modelCursor--
				} else if msg.Type == tea.KeyDown && m.modelCursor < len(m.fetchedModels)-1 {
					m.modelCursor++
				}
			}

		case tea.KeyEnter:
			if m.currentState == statePrompt {
				val := strings.TrimSpace(m.textarea.Value())

				if val == "/provider" {
					m.currentState = stateProviderSelect
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, nil
				} else if val == "/model" {
					m.currentState = stateFetchingModels
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, tea.Batch(m.spinner.Tick, m.fetchModelsCmd())
				} else if val == "/history" {
					m.currentState = stateHistory
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, m.fetchHistoryCmd()
				} else if val == "/status" {
					m.currentState = stateStatus
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, tea.Batch(m.spinner.Tick, m.fetchStatusCmd())
				} else if val == "/rollback" {
					m.currentState = stateRollback
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, tea.Batch(m.spinner.Tick, m.rollbackCmd())
				} else if strings.HasPrefix(val, "/doc ") {
					tool := strings.TrimSpace(strings.TrimPrefix(val, "/doc "))
					m.currentState = stateDoc
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, tea.Batch(m.spinner.Tick, m.fetchDocCmd(tool))
				} else if val == "/clear" {
					m.textarea.SetValue("")
					m.showSuggestions = false
					return m, nil
				}

				if val != "" && !strings.HasPrefix(val, "/") {
					m.prompt = val
					m.textarea.SetValue("")
					m.currentState = stateAnalysis
					m.showSuggestions = false
					return m, tea.Batch(
						m.spinner.Tick,
						m.generateLLMResponseCmd(val),
					)
				}
			} else if m.currentState == stateProviderSelect {
				selected := m.providers[m.providerCursor]
				m.cfg.ActiveProvider.Name = selected
				if selected != "Ollama" {
					m.currentState = stateAPIKeyInput
					m.textarea.Placeholder = fmt.Sprintf("Enter API Key for %s", selected)
					m.textarea.SetValue("")
					m.textarea.Focus()
					return m, textarea.Blink
				} else {
					m.currentState = stateFetchingModels
					return m, tea.Batch(
						m.spinner.Tick,
						m.fetchModelsCmd(),
					)
				}
			} else if m.currentState == stateAPIKeyInput {
				m.cfg.ActiveProvider.APIKey = strings.TrimSpace(m.textarea.Value())
				m.currentState = stateFetchingModels
				return m, tea.Batch(
					m.spinner.Tick,
					m.fetchModelsCmd(),
				)
			} else if m.currentState == stateModelSelect {
				if len(m.fetchedModels) > 0 {
					m.cfg.ActiveProvider.Model = m.fetchedModels[m.modelCursor]
				}
				config.SaveConfig(m.cfg)

				m.currentState = statePrompt
				m.textarea.Placeholder = "Configure a reverse proxy for Gitea... (type / for commands)"
				m.textarea.SetValue("")
			} else if m.currentState == stateExecution || m.currentState == stateHistory || m.currentState == stateStatus || m.currentState == stateRollback {
				m.currentState = statePrompt
				m.textarea.SetValue("")
				m.executionLogs = ""
				m.err = nil
				return m, nil
			}
		case tea.KeyRunes:
			if m.currentState == stateStaging {
				val := string(msg.Runes)
				if val == "?" {
					m.showReasoning = !m.showReasoning
					return m, nil
				} else if val == "y" || val == "Y" {
					m.currentState = stateExecution
					return m, tea.Batch(m.spinner.Tick, m.executeMutationsCmd())
				} else if val == "n" || val == "N" {
					m.llmResponse = nil
					m.currentState = statePrompt
					m.textarea.SetValue("")
					return m, nil
				}
				return m, nil // Ignore other keys
			}
		}

		if m.currentState == statePrompt || m.currentState == stateAPIKeyInput {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)

			val := m.textarea.Value()
			if m.currentState == statePrompt && strings.HasPrefix(val, "/") {
				m.showSuggestions = true
				m.filteredCommands = updateFilteredCommands(val)
				if m.commandCursor >= len(m.filteredCommands) {
					m.commandCursor = len(m.filteredCommands) - 1
				}
				if m.commandCursor < 0 {
					m.commandCursor = 0
				}
			} else {
				m.showSuggestions = false
			}

			return m, cmd
		}

	case ModelListMsg:
		m.fetchedModels = msg.Models
		m.modelCursor = 0
		m.currentState = stateModelSelect
		return m, nil

	case SystemProfileMsg:
		m.systemProfile = msg.Profile
		return m, nil

	case HistoryMsg:
		m.history = msg.History
		return m, nil

	case RollbackMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.currentState = stateError
		} else {
			m.executionLogs = "Rollback successful!"
		}
		return m, nil

	case ObservabilityMsg:
		if msg.Err != nil {
			m.historyBuffer += "\nError: " + msg.Err.Error() + "\n"
		} else {
			m.historyBuffer += "\n" + msg.Result + "\n"
		}
		m.viewport.SetContent(m.historyBuffer)
		m.viewport.GotoBottom()
		m.currentState = statePrompt
		return m, nil

		case DocMsg:
		m.currentState = stateDoc
		m.showModal = true
		if msg.Err != nil {
			m.docContent = "Error: " + msg.Err.Error()
		} else {
			m.docContent = msg.Content
		}
		return m, nil
		
	case ValidationErrorMsg:
		m.historyBuffer += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("Validation Failed. Executing Atomic Rollback...") + "\n"
		m.viewport.SetContent(m.historyBuffer)
		m.viewport.GotoBottom()
		
		m.currentState = stateAnalysis
		m.showModal = false
		
		fixPrompt := fmt.Sprintf("Your previous mutation failed validation.\nCommand: %s\nExact system error: %s\nCorrect the syntax.", msg.FailedCommand, msg.Stderr)
		
		return m, tea.Batch(
			m.spinner.Tick,
			m.generateLLMResponseCmd(fixPrompt),
		)

	case ExecutionMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.currentState = stateError
		} else {
			m.historyBuffer += "\n" + msg.Logs + "\n"
			m.viewport.SetContent(m.historyBuffer)
			m.viewport.GotoBottom()
			m.currentState = statePrompt
			m.showModal = false
		}
		return m, nil

	case *engine.LLMResponse:
		m.llmResponse = msg
		if msg.Status.IntentType == "OBSERVABILITY" || msg.Status.IntentType == "observability" {
			m.currentState = stateExecution
			return m, tea.Batch(m.spinner.Tick, m.executeObservabilityCmd(msg.Operations))
		}
		m.showModal = true
		m.currentState = stateStaging
		return m, nil

	case error:
		m.err = msg
		m.currentState = stateError
		return m, nil

	case spinner.TickMsg:
		if m.currentState == stateAnalysis || m.currentState == stateExecution || m.currentState == stateFetchingModels || m.currentState == stateStatus || m.currentState == stateRollback {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m model) fetchModelsCmd() tea.Cmd {
	return func() tea.Msg {
		provider := llm.NewProvider(m.cfg.ActiveProvider.Name, m.cfg.ActiveProvider.APIKey, "")
		models, err := provider.ListModels()
		if err != nil {
			return err
		}
		if len(models) == 0 {
			models = []string{"default"}
		}
		return ModelListMsg{Models: models}
	}
}

func (m model) fetchStatusCmd() tea.Cmd {
	return func() tea.Msg {
		prof, err := engine.ProfileSystem("", "")
		if err != nil {
			return err
		}
		return SystemProfileMsg{Profile: prof}
	}
}

func (m model) fetchDocCmd(tool string) tea.Cmd {
	return func() tea.Msg {
		doc, err := knowledge.FetchToolDoc(tool)
		return DocMsg{Content: doc, Err: err}
	}
}

func (m model) fetchHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		sm, err := state.NewStateManager()
		if err != nil {
			return err
		}
		hist, err := sm.GetHistory()
		if err != nil {
			return err
		}
		return HistoryMsg{History: hist}
	}
}

func (m model) rollbackCmd() tea.Cmd {
	return func() tea.Msg {
		sm, err := state.NewStateManager()
		if err != nil {
			return RollbackMsg{Err: err}
		}
		err = sm.Rollback(nil)
		return RollbackMsg{Success: err == nil, Err: err}
	}
}

func (m model) generateLLMResponseCmd(userPrompt string) tea.Cmd {
	return func() tea.Msg {
		provider := llm.NewProvider(m.cfg.ActiveProvider.Name, m.cfg.ActiveProvider.APIKey, m.cfg.ActiveProvider.Model)

		prof, err := engine.ProfileSystem(userPrompt, "")
		if err != nil {
			return err
		}

		profCtx := ""
		if prof != nil {
			profCtx = fmt.Sprintf("OS: %s\nInit: %s\nCurrent Target Config:\n%s\nRAG Rules:\n%s\n", prof.OSRelease, prof.InitSystem, prof.TargetConfig, prof.TargetRAG)
		}

		schema := `
CRITICAL DIRECTIVE: You are an autonomous system administrator daemon. You MUST reply with a raw JSON object ONLY. NO markdown formatting. NO conversational filler. NO backticks. If you fail to output strictly JSON, the system will crash.

REQUIRED SCHEMA:
{
  "status": {
    "intent_type": "OBSERVABILITY", // OR "MUTATION"
    "requires_packages": false,
    "warnings": []
  },
  "operations": [
    {
      "step_order": 1,
      "type": "SHELL_EXECUTION", // OR "FILE_MUTATION"
      "target": "DEPENDENCY", // OR "OBSERVABILITY", "VALIDATION", "COMMIT", or file path if FILE_MUTATION
      "command": ["cmd", "arg"], // Only if SHELL_EXECUTION
      "action_enum": "APPEND", // APPEND, OVERWRITE, DELETE. Only if FILE_MUTATION
      "base64_content": "", // Base64 encoded string. Only if FILE_MUTATION
      "reasoning": "Explain why this step is taken"
    }
  ]
}
IMPORTANT: For FILE_MUTATION, you MUST encode the file content in standard base64 and put it in base64_content.
`
		fullPrompt := fmt.Sprintf("System Context:\n%s\n\nUser Request: %s\n%s", profCtx, userPrompt, schema)

		var resp *engine.LLMResponse

		// Attempt up to 3 times (1 initial + 2 retries)
		for attempt := 0; attempt < 3; attempt++ {
			resp, err = provider.Generate(fullPrompt)
			if err == nil {
				return resp
			}
			// On error, append a penalty warning to the prompt for the next iteration
			fullPrompt += fmt.Sprintf("\n\nPREVIOUS ERROR: %v\nWARNING: DO NOT OUTPUT MARKDOWN OR BACKTICKS. OUTPUT RAW JSON ONLY.", err)
		}

		return fmt.Errorf("LLM Generation failed after 3 attempts. Last error: %w", err)
	}
}

func (m model) executeMutationsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.llmResponse == nil {
			return ExecutionMsg{Err: fmt.Errorf("no staging data found")}
		}

		sm, err := state.NewStateManager()
		if err != nil {
			return ExecutionMsg{Err: err}
		}

		var logs strings.Builder
		var affectedFiles []string
		var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		var successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

		// First, take snapshots for any file mutations
		logs.WriteString("Taking snapshots...\n")
		for _, op := range m.llmResponse.Operations {
			if op.Type == "FILE_MUTATION" {
				affectedFiles = append(affectedFiles, op.Target)
				if err := sm.Snapshot(op.Target, "Automated backup before mutation"); err != nil {
					logs.WriteString(fmt.Sprintf("Warning: could not snapshot %s: %v\n", op.Target, err))
				}
			}
		}

		// Execute sequentially based on Target (DEPENDENCY, FILE_MUTATION, VALIDATION, COMMIT)
		for _, op := range m.llmResponse.Operations {
			if op.Type == "SHELL_EXECUTION" && op.Target == "DEPENDENCY" {
				logs.WriteString(fmt.Sprintf("Resolving Dependency: %v...\n", op.Command))
				out, err := engine.Commit(op.Command) // use Commit function for shell execution
				if out != "" { logs.WriteString(fmt.Sprintf("  -> %s\n", out)) }
				if err != nil {
					_ = sm.Rollback(affectedFiles)
					return ExecutionMsg{Err: fmt.Errorf("dependency resolution failed: %w", err), Logs: logs.String()}
				}
			}
		}

		for _, op := range m.llmResponse.Operations {
			if op.Type == "FILE_MUTATION" {
				logs.WriteString(fmt.Sprintf("Applying [%s] to %s\n", op.ActionEnum, op.Target))
				
				mut := engine.Mutation{
					FilePath: op.Target,
					Action: strings.ToLower(op.ActionEnum),
					Content: op.DecodedContent(),
				}
				
				if err := engine.ExecuteMutation(mut); err != nil {
					_ = sm.Rollback(affectedFiles)
					return ExecutionMsg{Err: fmt.Errorf("mutation failed on %s: %w. Rolled back.", op.Target, err), Logs: logs.String()}
				}
			}
		}

		// Validation Phase
		for _, op := range m.llmResponse.Operations {
			if op.Type == "SHELL_EXECUTION" && op.Target == "VALIDATION" {
				logs.WriteString(fmt.Sprintf("Validating: %v...\n", op.Command))
				out, err := engine.Validate(op.Command)
				if out != "" { logs.WriteString(fmt.Sprintf("  -> %s\n", out)) }
				if err != nil {
					_ = sm.Rollback(affectedFiles)
					logs.WriteString(errorStyle.Render("Validation Failed. Executing Atomic Rollback..."))
					
					// Instead of normal error, return a specific message to trigger auto-correction
					return ValidationErrorMsg{Err: err, Stderr: out, FailedCommand: strings.Join(op.Command, " ")}
				}
			}
		}

		// Commit Phase
		for _, op := range m.llmResponse.Operations {
			if op.Type == "SHELL_EXECUTION" && op.Target == "COMMIT" {
				logs.WriteString(fmt.Sprintf("Committing: %v...\n", op.Command))
				out, err := engine.Commit(op.Command)
				if out != "" { logs.WriteString(fmt.Sprintf("  -> %s\n", out)) }
				if err != nil {
					logs.WriteString(errorStyle.Render(fmt.Sprintf("Commit Failed: %v", err)))
					return ExecutionMsg{Err: fmt.Errorf("commit failed: %w", err), Logs: logs.String()}
				}
			}
		}

		logs.WriteString("\n" + successStyle.Render("System state updated successfully."))
		return ExecutionMsg{Logs: logs.String(), Err: nil}
	}
}

func (m model) View() string {
	return renderLayout(&m)
}

func (m model) executeObservabilityCmd(ops []engine.Operation) tea.Cmd {
	return func() tea.Msg {
		var commands [][]string
		for _, op := range ops {
			if op.Type == "SHELL_EXECUTION" {
				commands = append(commands, op.Command)
			}
		}
		
		out, err := engine.ExecuteObservability(commands)
		if err != nil {
			return ObservabilityMsg{Err: err}
		}

		if len(out) > 2000 {
			provider := llm.NewProvider(m.cfg.ActiveProvider.Name, m.cfg.ActiveProvider.APIKey, m.cfg.ActiveProvider.Model)
			prompt := fmt.Sprintf("Summarize the following terminal output in concise markdown for a system administrator:\n\n%s", out)

			jsonPrompt := fmt.Sprintf("%s\n\nReturn strictly JSON: {\"status\": {\"intent_type\": \"OBSERVABILITY\", \"warnings\": [\"markdown summary here\"]}, \"operations\": []}", prompt)

			resp, err := provider.Generate(jsonPrompt)
			if err == nil && resp != nil {
				if len(resp.Status.Warnings) > 0 {
					out = string(out[:1900]) + "\n\n...[Output truncated, LLM summarization active]...\n\n" + resp.Status.Warnings[0]
				}
			}
		}

		return ObservabilityMsg{Result: out}
	}
}
