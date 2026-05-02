package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	wordwrap "github.com/mitchellh/go-wordwrap"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/detect"
	"github.com/mudittt/validatasaurus/internal/filecache"
	"github.com/mudittt/validatasaurus/internal/platform"
	"github.com/mudittt/validatasaurus/internal/tui"
	"github.com/mudittt/validatasaurus/internal/validator"
)

const (
	numW      = 2
	severityW = 8
	lineW     = 4
	colW      = 4
	msgW      = 42
	fixW      = 42
)

func main() {
	cfg := config.Load()

	args, detailed := extractDetailedFlag(os.Args[1:])

	if len(args) >= 1 {
		switch args[0] {
		case "--validate-local":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "error: --validate-local requires a file path")
				os.Exit(2)
			}
			runLocalValidate(cfg, args[1], detailed)
			return
		case "--detect":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "error: --detect requires a url")
				os.Exit(2)
			}
			runDetect(args[1])
			return
		case "--dry-run":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "error: --dry-run requires a url")
				os.Exit(2)
			}
			runDryRun(cfg, args[1], detailed)
			return
		}
	}

	model := tui.NewModel(cfg).WithDetailed(detailed)
	if len(args) >= 1 && !startsWithDash(args[0]) {
		model = model.WithInitialURL(args[0])
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}

func extractDetailedFlag(args []string) ([]string, bool) {
	detailed := false
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--detailed" {
			detailed = true
			continue
		}
		out = append(out, a)
	}
	return out, detailed
}

func startsWithDash(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func runDetect(ticketURL string) {
	kind, err := detect.DetectKind(ticketURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "detect: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Detected: %s\n", kind)
}

func runLocalValidate(cfg *config.Config, path string, detailed bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read file: %v\n", err)
		os.Exit(1)
	}
	files := []platform.SQLFile{{Name: path, Content: data}}
	results := validator.ValidateAll(cfg.PythonPath, files)
	printResults(results, detailed)
}

func runDryRun(cfg *config.Config, ticketURL string, detailed bool) {
	kind, err := detect.DetectKind(ticketURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "detect: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Detected: %s\n", kind)

	client, err := detect.ClientFor(kind, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		os.Exit(1)
	}

	files, err := filecache.FetchWithCache(ticketURL, client.FetchSQLFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("No .sql files found on this ticket.")
		return
	}
	fmt.Printf("Fetched %d SQL file(s):\n", len(files))
	for _, f := range files {
		fmt.Printf("  - %s (%d bytes)\n", f.Name, len(f.Content))
	}

	results := validator.ValidateAll(cfg.PythonPath, files)
	printResults(results, detailed)

	fmt.Printf("\n--- Comment body (%s) ---\n", client.Name())
	fmt.Println(validator.FormatComment(client.Name(), results))
}

func wrapText(s string, width uint) []string {
	wrapped := wordwrap.WrapString(s, width)
	return strings.Split(wrapped, "\n")
}

func printResults(results []validator.Result, detailed bool) {
	for _, r := range results {
		fmt.Printf("\n=== %s ===\n", r.FileName)
		fmt.Printf("Status:     %s\n", r.Status)
		fmt.Printf("Statements: %d  Errors: %d  Warnings: %d  Infos: %d\n",
			r.Statements, r.ErrorCount, r.WarnCount, r.InfoCount)

		if detailed && len(r.Issues) > 0 {
			fmt.Println()
			fmt.Printf("  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
				numW, "#",
				severityW, "Severity",
				lineW, "Line",
				colW, "Col",
				msgW, "Message",
				fixW, "Fix",
			)

			fmt.Printf("  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
				numW, "--",
				severityW, "--------",
				lineW, "----",
				colW, "----",
				msgW, strings.Repeat("-", msgW),
				fixW, strings.Repeat("-", fixW),
			)

			for i, iss := range r.Issues {
				col := "-"
				if iss.Column > 0 {
					col = fmt.Sprintf("%d", iss.Column)
				}

				msgLines := wrapText(iss.Message, msgW)
				fixLines := wrapText(iss.Suggestion, fixW)

				maxLines := len(msgLines)
				if len(fixLines) > maxLines {
					maxLines = len(fixLines)
				}

				for line := 0; line < maxLines; line++ {
					msg := ""
					fix := ""

					if line < len(msgLines) {
						msg = msgLines[line]
					}
					if line < len(fixLines) {
						fix = fixLines[line]
					}

					if line == 0 {
						fmt.Printf("  %-*d   %-*s   %-*d   %-*s   %-*s   %-*s\n",
							numW, i+1,
							severityW, iss.Severity,
							lineW, iss.LineNumber,
							colW, col,
							msgW, msg,
							fixW, fix)
					} else {
						fmt.Printf("  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
							numW, "",
							severityW, "",
							lineW, "",
							colW, "",
							msgW, msg,
							fixW, fix)
					}
				}
			}

			fmt.Println()
		}
	}
}
