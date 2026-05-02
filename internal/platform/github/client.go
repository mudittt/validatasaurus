package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/platform"
)

type Client struct {
	cfg  *config.Config
	http *http.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Name() string { return "GitHub" }

var (
	pathRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/issues/(\d+)`)
	sqlRE  = regexp.MustCompile(`\[([^\]]*\.sql)\]\((https?://[^)]+)\)`)
)

type ticketRef struct {
	owner  string
	repo   string
	number string
}

func parseTicket(ticketURL string) (ticketRef, error) {
	u, err := url.Parse(ticketURL)
	if err != nil {
		return ticketRef{}, fmt.Errorf("parse url: %w", err)
	}
	m := pathRE.FindStringSubmatch(u.Path)
	if len(m) != 4 {
		return ticketRef{}, fmt.Errorf("not a github issue url: %q", ticketURL)
	}
	return ticketRef{owner: m[1], repo: m[2], number: m[3]}, nil
}

func (c *Client) doRequest(method, endpoint string, body io.Reader) (*http.Response, error) {
	req, _ := http.NewRequest(method, endpoint, body)
	if c.cfg.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.GitHubToken)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

type issueBody struct {
	Body string `json:"body"`
}

type commentBody struct {
	Body string `json:"body"`
}

func (c *Client) FetchSQLFiles(ticketURL string) ([]platform.SQLFile, error) {
	if !c.cfg.GitHubReady() {
		return nil, fmt.Errorf("github token not configured")
	}
	t, err := parseTicket(ticketURL)
	if err != nil {
		return nil, err
	}

	bodies, err := c.collectBodies(t)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var files []platform.SQLFile
	for _, body := range bodies {
		for _, m := range sqlRE.FindAllStringSubmatch(body, -1) {
			name, link := m[1], m[2]
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			content, err := c.downloadFile(link)
			if err != nil {
				return nil, fmt.Errorf("download %s: %w", name, err)
			}
			files = append(files, platform.SQLFile{Name: name, Content: content})
		}
	}
	return files, nil
}

func (c *Client) collectBodies(t ticketRef) ([]string, error) {
	var bodies []string

	issueURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s", t.owner, t.repo, t.number)
	resp, err := c.doRequest(http.MethodGet, issueURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github issue fetch failed: %s — %s", resp.Status, truncate(string(b), 200))
	}
	var ib issueBody
	if err := json.NewDecoder(resp.Body).Decode(&ib); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}
	bodies = append(bodies, ib.Body)

	commentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", t.owner, t.repo, t.number)
	cresp, err := c.doRequest(http.MethodGet, commentsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch comments: %w", err)
	}
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cresp.Body)
		return nil, fmt.Errorf("github comments fetch failed: %s — %s", cresp.Status, truncate(string(b), 200))
	}
	var comments []commentBody
	if err := json.NewDecoder(cresp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("decode comments: %w", err)
	}
	for _, c := range comments {
		bodies = append(bodies, c.Body)
	}
	return bodies, nil
}

func (c *Client) downloadFile(link string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, link, nil)
	if c.cfg.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.GitHubToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %s — %s", resp.Status, truncate(string(b), 200))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) PostComment(ticketURL, body string) error {
	if !c.cfg.GitHubReady() {
		return fmt.Errorf("github token not configured")
	}
	t, err := parseTicket(ticketURL)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", t.owner, t.repo, t.number)
	payload, _ := json.Marshal(map[string]string{"body": body})
	resp, err := c.doRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github comment failed: %s — %s", resp.Status, truncate(string(b), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
