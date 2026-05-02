package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	colorGreen  = "#50FA7B"
	colorPurple = "#BD93F9"
	colorRed    = "#FF5555"
	colorOrange = "#FFB86C"
	colorCyan   = "#8BE9FD"
	colorGray   = "#6272A4"
	colorDark   = "#44475A"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorGreen))
	subtitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple))
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGray))
	dividerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDark))

	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorCyan))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGray)).Italic(true)
	headerBox = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorDark)).
			Padding(0, 1)

	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorPurple))
	tableCellStyle   = lipgloss.NewStyle().Padding(0, 1)
)

func divider(width int) string {
	if width < 1 {
		width = 60
	}
	out := ""
	for i := 0; i < width; i++ {
		out += "─"
	}
	return dividerStyle.Render(out)
}

func colourLogLine(s string) string {
	switch {
	case strings.HasPrefix(s, "❌"):
		return errorStyle.Render(s)
	case strings.HasPrefix(s, "⚠️"):
		return warnStyle.Render(s)
	case strings.HasPrefix(s, "✅"):
		return successStyle.Render(s)
	}
	return s
}
