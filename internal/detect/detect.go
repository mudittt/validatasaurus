package detect

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/platform"
	gh "github.com/mudittt/validatasaurus/internal/platform/github"
	"github.com/mudittt/validatasaurus/internal/platform/jira"
)

type Kind int

const (
	KindUnknown Kind = iota
	KindJira
	KindGitHub
)

func (k Kind) String() string {
	switch k {
	case KindJira:
		return "Jira"
	case KindGitHub:
		return "GitHub"
	}
	return "Unknown"
}

func DetectKind(ticketURL string) (Kind, error) {
	u, err := url.Parse(strings.TrimSpace(ticketURL))
	if err != nil {
		return KindUnknown, fmt.Errorf("parse url: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	path := u.Path

	switch {
	case strings.Contains(host, "atlassian.net") || strings.Contains(path, "/browse/"):
		return KindJira, nil
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return KindGitHub, nil
	}
	return KindUnknown, fmt.Errorf("unrecognised ticket url: %s", ticketURL)
}

func ClientFor(kind Kind, cfg *config.Config) (platform.Platform, error) {
	switch kind {
	case KindJira:
		return jira.New(cfg), nil
	case KindGitHub:
		return gh.New(cfg), nil
	}
	return nil, fmt.Errorf("no client for kind %s", kind)
}
