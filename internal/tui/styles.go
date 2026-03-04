package tui

import "github.com/charmbracelet/lipgloss"

var (
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	focusedPane = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("63"))

	unfocusedPane = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238"))

	refLegend = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("ref") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("=on demand  ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("inline") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("=embedded  ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render("global") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("=all projects  │  ")
)
