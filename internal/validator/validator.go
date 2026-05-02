package validator

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/mudittt/validatasaurus/internal/platform"
)

//go:embed sql_checker.py
var pythonScript []byte

const (
	scriptDir  = "/tmp/validatasaurus"
	scriptName = "sql_checker.py"
)

var (
	extractOnce sync.Once
	scriptPath  string
	extractErr  error
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

var (
	stmtCountRE = regexp.MustCompile(`Statements found\s*:\s*(\d+)`)
	countsRE    = regexp.MustCompile(`Found:\s*(\d+)\s*error\(s\).*?(\d+)\s*warning\(s\).*?(\d+)\s*info\(s\)`)
	resultRE    = regexp.MustCompile(`RESULT:\s*(.+)`)
)

type Result struct {
	FileName    string
	Passed      bool
	HasWarnings bool
	ErrorCount  int
	WarnCount   int
	InfoCount   int
	Statements  int
	Status      string
	RawOutput   string
}

func extractScript() (string, error) {
	extractOnce.Do(func() {
		if err := os.MkdirAll(scriptDir, 0o755); err != nil {
			extractErr = err
			return
		}
		path := filepath.Join(scriptDir, scriptName)
		if err := os.WriteFile(path, pythonScript, 0o644); err != nil {
			extractErr = err
			return
		}
		scriptPath = path
	})
	return scriptPath, extractErr
}

func Validate(pythonPath string, file platform.SQLFile) (Result, error) {
	script, err := extractScript()
	if err != nil {
		return Result{}, fmt.Errorf("extract python script: %w", err)
	}

	tmp, err := os.CreateTemp(scriptDir, "input-*.sql")
	if err != nil {
		return Result{}, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(file.Content); err != nil {
		tmp.Close()
		return Result{}, fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command(pythonPath, script, tmp.Name())
	out, runErr := cmd.CombinedOutput()
	clean := ansiRE.ReplaceAllString(string(out), "")

	res := Result{
		FileName:  file.Name,
		RawOutput: clean,
	}

	if m := stmtCountRE.FindStringSubmatch(clean); len(m) == 2 {
		res.Statements, _ = strconv.Atoi(m[1])
	}
	if m := countsRE.FindStringSubmatch(clean); len(m) == 4 {
		res.ErrorCount, _ = strconv.Atoi(m[1])
		res.WarnCount, _ = strconv.Atoi(m[2])
		res.InfoCount, _ = strconv.Atoi(m[3])
	}
	if m := resultRE.FindStringSubmatch(clean); len(m) == 2 {
		res.Status = strings.TrimSpace(m[1])
	}

	if res.Status == "" {
		// No issues at all → script prints success line, not RESULT:
		if strings.Contains(clean, "No syntax issues found") {
			res.Status = "PASSED"
		} else if runErr != nil {
			res.Status = "FAILED"
		} else {
			res.Status = "PASSED"
		}
	}

	res.Passed = !strings.EqualFold(res.Status, "FAILED")
	res.HasWarnings = strings.Contains(strings.ToLower(res.Status), "warning")

	// Sanity: exit code 1 from script ⇒ FAILED regardless
	if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		res.Passed = false
		if res.Status == "" || !strings.EqualFold(res.Status, "FAILED") {
			res.Status = "FAILED"
		}
	}

	return res, nil
}

func ValidateAll(pythonPath string, files []platform.SQLFile) []Result {
	results := make([]Result, 0, len(files))
	for _, f := range files {
		r, err := Validate(pythonPath, f)
		if err != nil {
			r = Result{
				FileName:  f.Name,
				Passed:    false,
				Status:    "FAILED",
				RawOutput: fmt.Sprintf("validator error: %v", err),
			}
		}
		results = append(results, r)
	}
	return results
}

func statusEmoji(r Result) string {
	switch {
	case !r.Passed:
		return "❌"
	case r.HasWarnings:
		return "⚠️"
	default:
		return "✅"
	}
}

func FormatComment(platformName string, results []Result) string {
	if strings.EqualFold(platformName, "jira") {
		return formatJira(results)
	}
	return formatGitHub(results)
}

func formatGitHub(results []Result) string {
	var b strings.Builder
	b.WriteString("## 🦕 Validatasaurus — SQL Validation Report\n\n")
	b.WriteString("| File | Status | Statements | Errors | Warnings |\n")
	b.WriteString("|------|--------|-----------:|-------:|---------:|\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("| `%s` | %s %s | %d | %d | %d |\n",
			r.FileName, statusEmoji(r), r.Status, r.Statements, r.ErrorCount, r.WarnCount))
	}
	b.WriteString("\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("<details><summary>📄 %s</summary>\n\n", r.FileName))
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(r.RawOutput))
		b.WriteString("\n```\n\n</details>\n\n")
	}
	return b.String()
}

func formatJira(results []Result) string {
	var b strings.Builder
	b.WriteString("h2. 🦕 Validatasaurus — SQL Validation Report\n\n")
	b.WriteString("|| File || Status || Statements || Errors || Warnings ||\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("| %s | %s %s | %d | %d | %d |\n",
			r.FileName, statusEmoji(r), r.Status, r.Statements, r.ErrorCount, r.WarnCount))
	}
	b.WriteString("\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("{panel:title=%s}\n{noformat}\n", r.FileName))
		b.WriteString(strings.TrimSpace(r.RawOutput))
		b.WriteString("\n{noformat}\n{panel}\n\n")
	}
	return b.String()
}
