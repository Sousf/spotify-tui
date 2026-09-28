package ui

import "github.com/charmbracelet/lipgloss"

var (
	green  = lipgloss.Color("#1DB954")
	dim    = lipgloss.Color("#6b6b6b")
	subtle = lipgloss.Color("#3a3a3a")
	white  = lipgloss.Color("#f0f0f0")
	yellow = lipgloss.Color("#e5c07b")
	red    = lipgloss.Color("#e06c75")

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle)
	focusedPaneStyle = paneStyle.BorderForeground(green)

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(white)
	dimStyle      = lipgloss.NewStyle().Foreground(dim)
	accentStyle   = lipgloss.NewStyle().Foreground(green)
	cursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#2a2a2a")).Foreground(white).Bold(true)
	playingStyle  = lipgloss.NewStyle().Foreground(green).Bold(true)
	headerStyle   = lipgloss.NewStyle().Foreground(dim).Bold(true).Underline(true)
	errorStyle    = lipgloss.NewStyle().Foreground(red)
	infoStyle     = lipgloss.NewStyle().Foreground(yellow)
	sectionStyle  = lipgloss.NewStyle().Foreground(dim).Bold(true)
	tabStyle      = lipgloss.NewStyle().Foreground(dim).Padding(0, 1)
	activeTab     = lipgloss.NewStyle().Foreground(green).Bold(true).Padding(0, 1).Underline(true)
	progressFill  = lipgloss.NewStyle().Foreground(green)
	progressTrack = lipgloss.NewStyle().Foreground(subtle)
	helpKeyStyle  = lipgloss.NewStyle().Foreground(green).Bold(true)
	helpBox       = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(green).
			Padding(1, 2)
)
