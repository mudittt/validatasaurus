package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mudittt/validatasaurus/internal/config"
	"github.com/mudittt/validatasaurus/internal/detect"
	"github.com/mudittt/validatasaurus/internal/platform"
	"github.com/mudittt/validatasaurus/internal/tui"
	"github.com/mudittt/validatasaurus/internal/validator"
)

func main() {
	cfg := config.Load()

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--validate-local":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "error: --validate-local requires a file path")
				os.Exit(2)
			}
			runLocalValidate(cfg, os.Args[2])
			return
		case "--detect":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "error: --detect requires a url")
				os.Exit(2)
			}
			runDetect(os.Args[2])
			return
		case "--dry-run":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "error: --dry-run requires a url")
				os.Exit(2)
			}
			runDryRun(cfg, os.Args[2])
			return
		}
	}

	model := tui.NewModel(cfg)
	if len(os.Args) >= 2 && !startsWithDash(os.Args[1]) {
		model = model.WithInitialURL(os.Args[1])
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
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

func runLocalValidate(cfg *config.Config, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read file: %v\n", err)
		os.Exit(1)
	}
	files := []platform.SQLFile{{Name: path, Content: data}}
	results := validator.ValidateAll(cfg.PythonPath, files)
	printResults(results)
}

func runDryRun(cfg *config.Config, ticketURL string) {
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

	files, err := client.FetchSQLFiles(ticketURL)
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
	printResults(results)

	fmt.Printf("\n--- Comment body (%s) ---\n", client.Name())
	fmt.Println(validator.FormatComment(client.Name(), results))
}

func printResults(results []validator.Result) {
	for _, r := range results {
		fmt.Printf("\n=== %s ===\n", r.FileName)
		fmt.Printf("Status:     %s\n", r.Status)
		fmt.Printf("Statements: %d  Errors: %d  Warnings: %d  Infos: %d\n",
			r.Statements, r.ErrorCount, r.WarnCount, r.InfoCount)
	}
}
