package tui

import (
	"fmt"
	"strings"

	"github.com/mudittt/validatasaurus/internal/validator"
)

const dinoArt = `
        __
       / _)
  .-^^^-/ /
__/       /
<__.|_|-|_|
`

func (m Model) View() string {
	switch m.state {
	case StateURLInput:
		return m.viewURLInput()
	case StateAuthInput:
		return m.viewAuthInput()
	case StateFetching, StateValidating, StatePosting:
		return m.viewProgress()
	case StateResults:
		return m.viewResults()
	case StateDone:
		return m.viewDone()
	case StateError:
		return m.viewError()
	}
	return ""
}

func (m Model) header() string {
	return titleStyle.Render("🦕 validatasaurus")
}

func (m Model) viewURLInput() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(dinoArt))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Ticket URL:"))
	b.WriteString("\n")
	b.WriteString(m.urlInput.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[Enter] validate    [Esc] quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewAuthInput() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("  ")
	b.WriteString(subtitleStyle.Render("›  Auth — " + m.kind.String()))
	b.WriteString("\n\n")
	b.WriteString(warnStyle.Render("Credentials not found in environment."))
	b.WriteString("\n\n")

	switch m.kind.String() {
	case "Jira":
		labels := []string{"Base URL", "Email", "API Token"}
		for i, in := range m.authInputs {
			b.WriteString(labelStyle.Render(labels[i] + ":"))
			b.WriteString("\n")
			b.WriteString(in.View())
			b.WriteString("\n\n")
		}
	case "GitHub":
		b.WriteString(labelStyle.Render("Personal Access Token:"))
		b.WriteString("\n")
		b.WriteString(m.authInputs[0].View())
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("[Tab/↓] next  [Shift-Tab/↑] prev  [Enter] submit  [Esc] quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewProgress() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(m.statusLn)
	b.WriteString("\n\n")
	b.WriteString(divider(60))
	b.WriteString("\n")

	start := 0
	if len(m.logs) > 10 {
		start = len(m.logs) - 10
	}
	for _, line := range m.logs[start:] {
		b.WriteString(colourLogLine(line))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewResults() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(summarySentence(m.results))
	b.WriteString("\n\n")
	b.WriteString(renderResultsTable(m.results))
	b.WriteString("\n")
	if m.detailed {
		b.WriteString(renderResultsDetail(m.results))
		b.WriteString("\n")
	}
	b.WriteString(labelStyle.Render("Post this report as a comment? "))
	b.WriteString(helpStyle.Render("[y] yes  [n] no  [d] " + detailToggleLabel(m.detailed) + "  [q] quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewDone() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(successStyle.Render("✅ Done"))
	b.WriteString("\n\n")
	if m.posted {
		b.WriteString(successStyle.Render("Comment posted to " + m.platform.Name() + "."))
	} else {
		b.WriteString(mutedStyle.Render("Comment was not posted."))
	}
	b.WriteString("\n\n")
	b.WriteString(renderResultsTable(m.results))
	b.WriteString("\n")
	if m.detailed {
		b.WriteString(renderResultsDetail(m.results))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Render("[d] " + detailToggleLabel(m.detailed) + "  [Enter] or [q] to exit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewError() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render("✖ Error"))
	b.WriteString("\n\n")
	for _, line := range strings.Split(m.errMsg, "\n") {
		b.WriteString(mutedStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[Enter] or [q] to exit"))
	b.WriteString("\n")
	return b.String()
}

func summarySentence(results []validator.Result) string {
	if len(results) == 0 {
		return mutedStyle.Render("No files validated.")
	}
	failed, warns, passed := 0, 0, 0
	for _, r := range results {
		switch {
		case !r.Passed:
			failed++
		case r.HasWarnings:
			warns++
		default:
			passed++
		}
	}
	switch {
	case failed > 0:
		return errorStyle.Render(fmt.Sprintf("%d file(s) failed", failed)) +
			" · " + warnStyle.Render(fmt.Sprintf("%d with warnings", warns)) +
			" · " + successStyle.Render(fmt.Sprintf("%d passed", passed))
	case warns > 0:
		return warnStyle.Render(fmt.Sprintf("%d with warnings", warns)) +
			" · " + successStyle.Render(fmt.Sprintf("%d passed", passed))
	default:
		return successStyle.Render(fmt.Sprintf("All %d file(s) passed ✨", passed))
	}
}

func renderResultsTable(results []validator.Result) string {
	if len(results) == 0 {
		return mutedStyle.Render("(no results)")
	}
	var b strings.Builder
	b.WriteString(tableHeaderStyle.Render(
		fmt.Sprintf("  %-30s  %-22s  %6s  %6s  %6s", "File", "Status", "Stmts", "Errors", "Warns")))
	b.WriteString("\n")
	b.WriteString(divider(80))
	b.WriteString("\n")
	for _, r := range results {
		row := fmt.Sprintf("  %-30s  %-22s  %6d  %6d  %6d",
			truncFile(r.FileName, 30), r.Status, r.Statements, r.ErrorCount, r.WarnCount)
		switch {
		case !r.Passed:
			b.WriteString(errorStyle.Render(row))
		case r.HasWarnings:
			b.WriteString(warnStyle.Render(row))
		default:
			b.WriteString(successStyle.Render(row))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncFile(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func renderResultsDetail(results []validator.Result) string {
	var b strings.Builder
	for _, r := range results {
		b.WriteString(labelStyle.Render("── " + r.FileName + " ──"))
		b.WriteString("\n")
		if len(r.Issues) == 0 {
			b.WriteString(mutedStyle.Render("✅ No issues"))
		} else {
			b.WriteString(mutedStyle.Render(strings.TrimRight(validator.FormatIssuesTable(r.Issues), "\n")))
		}
		b.WriteString("\n\n")
	}
	return b.String()
}

func detailToggleLabel(detailed bool) string {
	if detailed {
		return "hide details"
	}
	return "show details"
}
