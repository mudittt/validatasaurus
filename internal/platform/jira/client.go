package jira

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

func (c *Client) Name() string { return "Jira" }

var issueKeyRE = regexp.MustCompile(`/browse/([A-Z][A-Z0-9]+-\d+)`)

func extractIssueKey(ticketURL string) (string, error) {
	u, err := url.Parse(ticketURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	m := issueKeyRE.FindStringSubmatch(u.Path)
	if len(m) != 2 {
		return "", fmt.Errorf("could not find issue key in path %q", u.Path)
	}
	return m[1], nil
}

func (c *Client) baseURL() string {
	return strings.TrimRight(c.cfg.JiraBaseURL, "/")
}

func (c *Client) authHeader() string {
	raw := c.cfg.JiraEmail + ":" + c.cfg.JiraAPIToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

type attachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type issueResp struct {
	Fields struct {
		Attachment []attachment `json:"attachment"`
	} `json:"fields"`
}

func (c *Client) FetchSQLFiles(ticketURL string) ([]platform.SQLFile, error) {
	if !c.cfg.JiraReady() {
		return nil, fmt.Errorf("jira credentials not configured")
	}
	key, err := extractIssueKey(ticketURL)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=attachment", c.baseURL(), key)
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira issue fetch failed: %s — %s", resp.Status, truncate(string(body), 200))
	}
	var issue issueResp
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}

	var files []platform.SQLFile
	for _, a := range issue.Fields.Attachment {
		if !strings.HasSuffix(strings.ToLower(a.Filename), ".sql") {
			continue
		}
		content, err := c.downloadAttachment(a.Content)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", a.Filename, err)
		}
		files = append(files, platform.SQLFile{Name: a.Filename, Content: content})
	}
	return files, nil
}

func (c *Client) downloadAttachment(downloadURL string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, downloadURL, nil)
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %s — %s", resp.Status, truncate(string(body), 200))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) PostComment(ticketURL, body string) error {
	if !c.cfg.JiraReady() {
		return fmt.Errorf("jira credentials not configured")
	}
	key, err := extractIssueKey(ticketURL)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", c.baseURL(), key)
	payload, _ := json.Marshal(map[string]string{"body": body})

	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jira comment failed: %s — %s", resp.Status, truncate(string(b), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
