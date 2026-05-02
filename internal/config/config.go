package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	JiraBaseURL  string
	JiraEmail    string
	JiraAPIToken string
	GitHubToken  string
	PythonPath   string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		JiraBaseURL:  os.Getenv("JIRA_BASE_URL"),
		JiraEmail:    os.Getenv("JIRA_EMAIL"),
		JiraAPIToken: os.Getenv("JIRA_API_TOKEN"),
		GitHubToken:  os.Getenv("GITHUB_TOKEN"),
		PythonPath:   os.Getenv("PYTHON_PATH"),
	}

	if cfg.PythonPath == "" {
		cfg.PythonPath = "python3"
	}

	return cfg
}

func (c *Config) JiraReady() bool {
	return c.JiraBaseURL != "" && c.JiraEmail != "" && c.JiraAPIToken != ""
}

func (c *Config) GitHubReady() bool {
	return c.GitHubToken != ""
}
