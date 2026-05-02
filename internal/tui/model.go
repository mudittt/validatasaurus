package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/detect"
	"github.com/mudittt/validatasaurus/internal/platform"
	"github.com/mudittt/validatasaurus/internal/validator"
)

type State int

const (
	StateURLInput State = iota
	StateFetching
	StateAuthInput
	StateValidating
	StateResults
	StatePosting
	StateDone
	StateError
)

type Model struct {
	cfg *config.Config

	state    State
	width    int
	height   int
	errMsg   string
	logs     []string
	logCap   int
	statusLn string

	spinner spinner.Model

	urlInput textinput.Model

	authInputs []textinput.Model
	authFocus  int

	ticketURL string
	platform  platform.Platform
	kind      detect.Kind

	files       []platform.SQLFile
	fileIdx     int
	results     []validator.Result
	commentBody string
	posted      bool
}

func NewModel(cfg *config.Config) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))

	ti := textinput.New()
	ti.Placeholder = "https://company.atlassian.net/browse/PROJ-123  or  https://github.com/owner/repo/issues/42"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 80

	return Model{
		cfg:      cfg,
		state:    StateURLInput,
		spinner:  sp,
		urlInput: ti,
		logCap:   20,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

func (m Model) WithInitialURL(url string) Model {
	m.urlInput.SetValue(url)
	return m
}

func (m *Model) appendLog(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > m.logCap {
		m.logs = m.logs[len(m.logs)-m.logCap:]
	}
}

func (m *Model) setStatus(s string) { m.statusLn = s }
