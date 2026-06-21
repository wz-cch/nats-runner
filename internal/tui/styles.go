package tui

import "github.com/charmbracelet/lipgloss"

// Styles groups all lipgloss style definitions used across TUI screens.
var Styles = struct {
	Title    lipgloss.Style
	Selected lipgloss.Style
	Normal   lipgloss.Style
	Hint     lipgloss.Style
	Header   lipgloss.Style
	Label    lipgloss.Style
	Value    lipgloss.Style
	Source   lipgloss.Style
	OK       lipgloss.Style
	Err      lipgloss.Style
	Border   lipgloss.Style
	CLIBox   lipgloss.Style
}{
	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		PaddingBottom(1),

	Selected: lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true),

	Normal: lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")),

	Hint: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true),

	Header: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")).
		Underline(true),

	Label: lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Bold(true),

	Value: lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")),

	Source: lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")),

	OK: lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true),

	Err: lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true),

	Border: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(0, 1),

	CLIBox: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Foreground(lipgloss.Color("6")),
}

// Cursor returns the cursor prefix for list items.
func Cursor(selected bool) string {
	if selected {
		return Styles.Selected.Render("> ")
	}
	return "  "
}
