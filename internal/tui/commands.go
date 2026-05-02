package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/detect"
	"github.com/mudittt/validatasaurus/internal/filecache"
	"github.com/mudittt/validatasaurus/internal/platform"
	"github.com/mudittt/validatasaurus/internal/validator"
)

type (
	msgPlatformDetected struct {
		kind   detect.Kind
		client platform.Platform
	}
	msgAuthRequired struct{ kind detect.Kind }
	msgReadyToFetch struct{}

	msgFilesFetched struct{ files []platform.SQLFile }
	msgValidated    struct {
		idx    int
		result validator.Result
	}
	msgAllValidated struct{}

	msgCommentPosted struct{}

	msgErr struct{ err error }
)

func detectPlatformCmd(cfg *config.Config, ticketURL string) tea.Cmd {
	return func() tea.Msg {
		kind, err := detect.DetectKind(ticketURL)
		if err != nil {
			return msgErr{err}
		}
		client, err := detect.ClientFor(kind, cfg)
		if err != nil {
			return msgErr{err}
		}
		return msgPlatformDetected{kind: kind, client: client}
	}
}

func checkAuthCmd(cfg *config.Config, kind detect.Kind) tea.Cmd {
	return func() tea.Msg {
		switch kind {
		case detect.KindJira:
			if !cfg.JiraReady() {
				return msgAuthRequired{kind}
			}
		case detect.KindGitHub:
			if !cfg.GitHubReady() {
				return msgAuthRequired{kind}
			}
		}
		return msgReadyToFetch{}
	}
}

func fetchFilesCmd(client platform.Platform, ticketURL string) tea.Cmd {
	return func() tea.Msg {
		files, err := filecache.FetchWithCache(ticketURL, client.FetchSQLFiles)
		if err != nil {
			return msgErr{err}
		}
		return msgFilesFetched{files}
	}
}

func validateFileCmd(pythonPath string, idx int, file platform.SQLFile) tea.Cmd {
	return func() tea.Msg {
		r, err := validator.Validate(pythonPath, file)
		if err != nil {
			return msgErr{err}
		}
		return msgValidated{idx: idx, result: r}
	}
}

func postCommentCmd(client platform.Platform, ticketURL, body string) tea.Cmd {
	return func() tea.Msg {
		if err := client.PostComment(ticketURL, body); err != nil {
			return msgErr{err}
		}
		return msgCommentPosted{}
	}
}
