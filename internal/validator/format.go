package validator

import (
	"fmt"
	"strings"
)

const (
	tblNumW      = 2
	tblSeverityW = 8
	tblLineW     = 4
	tblColW      = 4
	tblPhraseW   = 15
	tblMsgW      = 32
	tblFixW      = 32
)

func FormatIssuesTable(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}

	var b strings.Builder

	fmt.Fprintf(&b, "  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
		tblNumW, "#",
		tblSeverityW, "Severity",
		tblLineW, "Line",
		tblColW, "Col",
		tblPhraseW, "Phrase",
		tblMsgW, "Message",
		tblFixW, "Fix",
	)

	fmt.Fprintf(&b, "  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
		tblNumW, "--",
		tblSeverityW, "--------",
		tblLineW, "----",
		tblColW, "----",
		tblPhraseW, strings.Repeat("-", tblPhraseW),
		tblMsgW, strings.Repeat("-", tblMsgW),
		tblFixW, strings.Repeat("-", tblFixW),
	)

	for i, iss := range issues {
		col := "-"
		if iss.Column > 0 {
			col = fmt.Sprintf("%d", iss.Column)
		}

		phraseLines := wrapText(iss.Phrase, tblPhraseW)
		msgLines := wrapText(iss.Message, tblMsgW)
		fixLines := wrapText(iss.Suggestion, tblFixW)

		maxLines := len(phraseLines)
		if len(msgLines) > maxLines {
			maxLines = len(msgLines)
		}
		if len(fixLines) > maxLines {
			maxLines = len(fixLines)
		}

		for line := 0; line < maxLines; line++ {
			phrase := ""
			msg := ""
			fix := ""

			if line < len(phraseLines) {
				phrase = phraseLines[line]
			}
			if line < len(msgLines) {
				msg = msgLines[line]
			}
			if line < len(fixLines) {
				fix = fixLines[line]
			}

			if line == 0 {
				fmt.Fprintf(&b, "  %-*d   %-*s   %-*d   %-*s   %-*s   %-*s   %-*s\n",
					tblNumW, i+1,
					tblSeverityW, iss.Severity,
					tblLineW, iss.LineNumber,
					tblColW, col,
					tblPhraseW, phrase,
					tblMsgW, msg,
					tblFixW, fix)
			} else {
				fmt.Fprintf(&b, "  %-*s   %-*s   %-*s   %-*s   %-*s   %-*s   %-*s\n",
					tblNumW, "",
					tblSeverityW, "",
					tblLineW, "",
					tblColW, "",
					tblPhraseW, phrase,
					tblMsgW, msg,
					tblFixW, fix)
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	s = strings.ReplaceAll(s, "↵", "\n")
	var out []string
	for _, rawLine := range strings.Split(s, "\n") {
		line := rawLine
		for len(line) > width {
			splitAt := width
			if idx := strings.LastIndex(line[:width], " "); idx > 0 {
				splitAt = idx
			}
			out = append(out, strings.TrimSpace(line[:splitAt]))
			line = strings.TrimSpace(line[splitAt:])
		}
		out = append(out, line)
	}
	return out
}
