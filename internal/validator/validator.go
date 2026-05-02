package validator

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type Issue struct {
	Severity   string
	LineNumber int
	Column     int
	Phrase     string
	Message    string
	Suggestion string
}

type Result struct {
	FileName    string
	Passed      bool
	HasWarnings bool
	ErrorCount  int
	WarnCount   int
	InfoCount   int
	Statements  int
	Status      string
	Issues      []Issue
	RawOutput   string
}

type pyIssue struct {
	Severity   string `json:"severity"`
	LineNumber int    `json:"line_number"`
	Column     *int   `json:"column"`
	Phrase     string `json:"phrase"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type pyReport struct {
	FilePath        string    `json:"file_path"`
	TotalStatements int       `json:"total_statements"`
	Issues          []pyIssue `json:"issues"`
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

	cmd := exec.Command(pythonPath, script, "--json", tmp.Name())
	out, runErr := cmd.CombinedOutput()

	res := Result{FileName: file.Name}

	var pr pyReport
	if err := json.Unmarshal(out, &pr); err != nil {
		res.Status = "FAILED"
		res.RawOutput = fmt.Sprintf("failed to parse script output: %v\n%s", err, string(out))
		if runErr != nil {
			res.RawOutput = fmt.Sprintf("script error: %v\n%s", runErr, string(out))
		}
		return res, nil
	}

	res.Statements = pr.TotalStatements
	for _, pi := range pr.Issues {
		col := 0
		if pi.Column != nil {
			col = *pi.Column
		}
		res.Issues = append(res.Issues, Issue{
			Severity:   pi.Severity,
			LineNumber: pi.LineNumber,
			Column:     col,
			Phrase:     pi.Phrase,
			Message:    pi.Message,
			Suggestion: pi.Suggestion,
		})
		switch pi.Severity {
		case "ERROR":
			res.ErrorCount++
		case "WARNING":
			res.WarnCount++
		case "INFO":
			res.InfoCount++
		}
	}

	if res.ErrorCount > 0 {
		res.Status = "FAILED"
	} else if res.WarnCount > 0 {
		res.Status = "PASSED with warnings"
	} else {
		res.Status = "PASSED"
	}
	res.Passed = res.ErrorCount == 0
	res.HasWarnings = res.WarnCount > 0
	res.RawOutput = buildRawOutput(res)

	return res, nil
}

func buildRawOutput(res Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Statements found: %d\n", res.Statements)
	fmt.Fprintf(&b, "Found: %d error(s)  |  %d warning(s)  |  %d info(s)\n", res.ErrorCount, res.WarnCount, res.InfoCount)
	if len(res.Issues) == 0 {
		b.WriteString("\n✅ No syntax issues found!\n")
	} else {
		for i, iss := range res.Issues {
			fmt.Fprintf(&b, "\n#%d [%s] Line %d", i+1, iss.Severity, iss.LineNumber)
			if iss.Column > 0 {
				fmt.Fprintf(&b, ", Col %d", iss.Column)
			}
			fmt.Fprintf(&b, "\n  Problem : %s\n  Found   : %s\n", iss.Message, iss.Phrase)
			if iss.Suggestion != "" {
				fmt.Fprintf(&b, "  Fix     : %s\n", iss.Suggestion)
			}
		}
	}
	fmt.Fprintf(&b, "\nRESULT: %s\n", res.Status)
	return b.String()
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

func FormatFileComment(platformName string, r Result) string {
	if strings.EqualFold(platformName, "jira") {
		return formatJiraFile(r)
	}
	return formatGitHubFile(r)
}

func formatGitHubFile(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## 🦕 Validatasaurus — `%s`\n\n", r.FileName)
	fmt.Fprintf(&b, "%s **%s** · %d statement(s) · %d error(s) · %d warning(s)\n\n",
		statusEmoji(r), r.Status, r.Statements, r.ErrorCount, r.WarnCount)
	b.WriteString("```\n")
	b.WriteString(detailBody(r))
	b.WriteString("\n```\n")
	return b.String()
}

func formatJiraFile(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "h2. 🦕 Validatasaurus — %s\n\n", r.FileName)
	fmt.Fprintf(&b, "%s *%s* — %d statement(s), %d error(s), %d warning(s)\n\n",
		statusEmoji(r), r.Status, r.Statements, r.ErrorCount, r.WarnCount)
	b.WriteString("{noformat}\n")
	b.WriteString(detailBody(r))
	b.WriteString("\n{noformat}\n")
	return b.String()
}

func detailBody(r Result) string {
	header := fmt.Sprintf("Statements: %d  Errors: %d  Warnings: %d  Infos: %d\nStatus: %s",
		r.Statements, r.ErrorCount, r.WarnCount, r.InfoCount, r.Status)
	if len(r.Issues) == 0 {
		return header + "\n\n✅ No issues found"
	}
	return header + "\n\n" + strings.TrimRight(FormatIssuesTable(r.Issues), "\n")
}
