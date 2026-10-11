package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Sousf/spotify-tui/internal/theme"
)

// Styles are package-level so the views can reach them directly. They are
// rebuilt by applyPalette, which Update calls on start and whenever the
// theme file changes. bubbletea runs Update and View on one goroutine, so
// swapping them here is safe.
var (
	paneStyle        lipgloss.Style
	focusedPaneStyle lipgloss.Style
	titleStyle       lipgloss.Style
	dimStyle         lipgloss.Style
	accentStyle      lipgloss.Style
	cursorStyle      lipgloss.Style
	playingStyle     lipgloss.Style
	headerStyle      lipgloss.Style
	errorStyle       lipgloss.Style
	infoStyle        lipgloss.Style
	sectionStyle     lipgloss.Style
	tabStyle         lipgloss.Style
	activeTab        lipgloss.Style
	progressFill     lipgloss.Style
	progressTrack    lipgloss.Style
	helpKeyStyle     lipgloss.Style
	// visualiser bands, bottom to top
	vizLow, vizMid, vizHigh lipgloss.Style
	// piano keys at rest
	keyWhite, keyBlack lipgloss.Style
)

func init() {
	applyPalette(theme.Default())
}

func applyPalette(p theme.Palette) {
	text := lipgloss.Color(p.Text)
	overlay := lipgloss.Color(p.Overlay)
	accent := lipgloss.Color(p.Accent)

	paneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.Surface1))
	focusedPaneStyle = paneStyle.BorderForeground(accent)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(text)
	dimStyle = lipgloss.NewStyle().Foreground(overlay)
	accentStyle = lipgloss.NewStyle().Foreground(accent)
	cursorStyle = lipgloss.NewStyle().Background(lipgloss.Color(p.Surface0)).Foreground(text).Bold(true)
	playingStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	headerStyle = lipgloss.NewStyle().Foreground(overlay).Bold(true).Underline(true)
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Red))
	infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Yellow))
	sectionStyle = lipgloss.NewStyle().Foreground(overlay).Bold(true)
	tabStyle = lipgloss.NewStyle().Foreground(overlay).Padding(0, 1)
	activeTab = lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 1).Underline(true)
	progressFill = lipgloss.NewStyle().Foreground(accent)
	progressTrack = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Surface2))
	helpKeyStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

	vizLow = accentStyle
	vizMid = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent2))
	vizHigh = errorStyle
	keyWhite = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Subtext))
	keyBlack = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Surface2))
}
