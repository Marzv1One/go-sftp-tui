package app

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	accentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#4338CA")).Bold(true)
	directoryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5EEAD4"))
)

func displayText(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return -1
		}
		return char
	}, ansi.Strip(value))
}

func panel(content string, width int, active bool) string {
	color := lipgloss.Color("#475569")
	if active {
		color = lipgloss.Color("#A78BFA")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Width(max(4, width)).Render(content)
}

func matchingChoices(choices []string, query string) []string {
	matches := []string{}
	seen := map[string]bool{}
	for _, choice := range choices {
		if !seen[choice] && strings.Contains(strings.ToLower(choice), strings.ToLower(query)) {
			matches = append(matches, choice)
			seen[choice] = true
		}
	}
	return matches
}

func choiceView(choices []string, cursor, rows, width int) string {
	if len(choices) == 0 {
		return mutedStyle.Render("  No matches")
	}
	start := max(0, cursor-rows+1)
	lines := []string{}
	for index := start; index < min(len(choices), start+rows); index++ {
		line := "  " + displayText(choices[index])
		if index == cursor {
			line = "> " + displayText(choices[index])
		}
		line = truncate(line, width)
		if index == cursor {
			line = selectedStyle.Width(width).Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("  %d/%d", cursor+1, len(choices))))
	return strings.Join(lines, "\n")
}
