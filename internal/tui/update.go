package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mudittt/validatasaurus/internal/detect"
	"github.com/mudittt/validatasaurus/internal/validator"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Spinner ticks must always be processed.
	if tickMsg, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(tickMsg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case msgErr:
		m.errMsg = msg.err.Error()
		m.state = StateError
		return m, nil

	case msgPlatformDetected:
		m.kind = msg.kind
		m.platform = msg.client
		m.appendLog("✅ Detected platform: " + msg.kind.String())
		m.setStatus("Checking auth…")
		return m, checkAuthCmd(m.cfg, msg.kind)

	case msgAuthRequired:
		m.appendLog("⚠️ Auth missing — please enter credentials.")
		m.setupAuthInputs(msg.kind)
		m.state = StateAuthInput
		return m, textinput.Blink

	case msgReadyToFetch:
		m.appendLog("✅ Auth OK")
		m.setStatus("Fetching SQL files…")
		m.state = StateFetching
		return m, fetchFilesCmd(m.platform, m.ticketURL)

	case msgFilesFetched:
		m.files = msg.files
		if len(m.files) == 0 {
			m.errMsg = "No .sql files attached to this ticket."
			m.state = StateError
			return m, nil
		}
		m.appendLog("✅ Fetched " + intStr(len(m.files)) + " SQL file(s)")
		m.results = make([]validator.Result, 0, len(m.files))
		m.fileIdx = 0
		m.state = StateValidating
		m.setStatus("Validating " + m.files[0].Name + "…")
		return m, validateFileCmd(m.cfg.PythonPath, 0, m.files[0])

	case msgValidated:
		m.results = append(m.results, msg.result)
		switch {
		case !msg.result.Passed:
			m.appendLog("❌ " + msg.result.FileName + " — " + msg.result.Status)
		case msg.result.HasWarnings:
			m.appendLog("⚠️ " + msg.result.FileName + " — " + msg.result.Status)
		default:
			m.appendLog("✅ " + msg.result.FileName + " — " + msg.result.Status)
		}
		next := m.fileIdx + 1
		if next < len(m.files) {
			m.fileIdx = next
			m.setStatus("Validating " + m.files[next].Name + "…")
			return m, validateFileCmd(m.cfg.PythonPath, next, m.files[next])
		}
		m.state = StateResults
		return m, nil

	case msgCommentPosted:
		m.posted = true
		m.state = StateDone
		return m, nil
	}

	// Default: route through whichever input has focus.
	switch m.state {
	case StateURLInput:
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(msg)
		return m, cmd
	case StateAuthInput:
		var cmd tea.Cmd
		m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quits
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.state {
	case StateURLInput:
		switch msg.Type {
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			url := strings.TrimSpace(m.urlInput.Value())
			if url == "" {
				return m, nil
			}
			m.ticketURL = url
			m.appendLog("→ " + url)
			m.setStatus("Detecting platform…")
			m.state = StateFetching
			return m, detectPlatformCmd(m.cfg, url)
		}
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(msg)
		return m, cmd

	case StateAuthInput:
		switch msg.Type {
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyTab, tea.KeyDown:
			m.cycleAuthFocus(1)
			return m, nil
		case tea.KeyShiftTab, tea.KeyUp:
			m.cycleAuthFocus(-1)
			return m, nil
		case tea.KeyEnter:
			if m.authFocus < len(m.authInputs)-1 {
				m.cycleAuthFocus(1)
				return m, nil
			}
			m.applyAuth()
			m.appendLog("✅ Credentials captured")
			m.setStatus("Fetching SQL files…")
			m.state = StateFetching
			return m, fetchFilesCmd(m.platform, m.ticketURL)
		}
		var cmd tea.Cmd
		m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
		return m, cmd

	case StateResults:
		switch strings.ToLower(msg.String()) {
		case "y":
			body := validator.FormatComment(m.platform.Name(), m.results)
			m.commentBody = body
			m.state = StatePosting
			m.setStatus("Posting report to " + m.platform.Name() + "…")
			return m, postCommentCmd(m.platform, m.ticketURL, body)
		case "n":
			m.state = StateDone
			return m, nil
		case "d":
			m.detailed = !m.detailed
			return m, nil
		case "q":
			return m, tea.Quit
		}
		return m, nil

	case StateDone, StateError:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
			return m, tea.Quit
		}
		switch strings.ToLower(msg.String()) {
		case "q":
			return m, tea.Quit
		case "d":
			if m.state == StateDone {
				m.detailed = !m.detailed
			}
			return m, nil
		}
		return m, nil

	case StateFetching, StateValidating, StatePosting:
		if msg.Type == tea.KeyEsc {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) setupAuthInputs(kind detect.Kind) {
	m.authInputs = nil
	switch kind {
	case detect.KindJira:
		base := textinput.New()
		base.Placeholder = "https://company.atlassian.net"
		base.SetValue(m.cfg.JiraBaseURL)
		base.CharLimit = 256
		base.Width = 60

		email := textinput.New()
		email.Placeholder = "you@example.com"
		email.SetValue(m.cfg.JiraEmail)
		email.CharLimit = 256
		email.Width = 60

		token := textinput.New()
		token.Placeholder = "API token"
		token.EchoMode = textinput.EchoPassword
		token.EchoCharacter = '•'
		token.SetValue(m.cfg.JiraAPIToken)
		token.CharLimit = 256
		token.Width = 60

		m.authInputs = []textinput.Model{base, email, token}

	case detect.KindGitHub:
		token := textinput.New()
		token.Placeholder = "ghp_… (Personal Access Token)"
		token.EchoMode = textinput.EchoPassword
		token.EchoCharacter = '•'
		token.SetValue(m.cfg.GitHubToken)
		token.CharLimit = 256
		token.Width = 60
		m.authInputs = []textinput.Model{token}
	}
	m.authFocus = 0
	if len(m.authInputs) > 0 {
		m.authInputs[0].Focus()
	}
}

func (m *Model) cycleAuthFocus(delta int) {
	n := len(m.authInputs)
	if n == 0 {
		return
	}
	m.authInputs[m.authFocus].Blur()
	m.authFocus = (m.authFocus + delta + n) % n
	m.authInputs[m.authFocus].Focus()
}

func (m *Model) applyAuth() {
	switch m.kind {
	case detect.KindJira:
		m.cfg.JiraBaseURL = strings.TrimSpace(m.authInputs[0].Value())
		m.cfg.JiraEmail = strings.TrimSpace(m.authInputs[1].Value())
		m.cfg.JiraAPIToken = strings.TrimSpace(m.authInputs[2].Value())
	case detect.KindGitHub:
		m.cfg.GitHubToken = strings.TrimSpace(m.authInputs[0].Value())
	}
}

func intStr(n int) string {
	// small itoa to avoid an import
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
