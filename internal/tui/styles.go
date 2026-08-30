package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Palette — dark neon
	bg        = lipgloss.Color("#0B0E14")
	cardBg    = lipgloss.Color("#111827")
	cardBd    = lipgloss.Color("#1F2937")
	mutedCol  = lipgloss.Color("#9CA3AF")
	accentCol = lipgloss.Color("#60A5FA")
	accent2   = lipgloss.Color("#34D399")
	redCol    = lipgloss.Color("#EF4444")
	greenCol  = lipgloss.Color("#22C55E")
	yellowCol = lipgloss.Color("#EAB308")
	purpleCol = lipgloss.Color("#A78BFA")

	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0F172A")).
			Foreground(lipgloss.Color("#E5E7EB")).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cardBd)

	titleStyle = lipgloss.NewStyle().
			Foreground(accentCol).
			Bold(true).
			Background(lipgloss.Color("#0F172A")).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedCol).
			Italic(true).
			Background(lipgloss.Color("#0F172A"))

	cardStyle = lipgloss.NewStyle().
			Background(cardBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cardBd).
			Padding(1, 1)

	cardFocusStyle = lipgloss.NewStyle().
			Background(cardBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentCol).
			Padding(1, 1)

	selectedStyle = lipgloss.NewStyle().
			Background(accentCol).
			Foreground(lipgloss.Color("#0B0E14")).
			Bold(true).
			Padding(0, 1).
			Margin(0, 0)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB")).
			Padding(0, 1)

	mutedStyle = lipgloss.NewStyle().Foreground(mutedCol)

	successStyle = lipgloss.NewStyle().Foreground(greenCol).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(redCol).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(yellowCol)

	kpiLabelStyle = lipgloss.NewStyle().
			Foreground(mutedCol).
			Bold(true).
			Width(18).
			Align(lipgloss.Center)

	kpiValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8FAFC")).
			Bold(true).
			Align(lipgloss.Center).
			Width(18)

	badgeLongStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#052E16")).
			Foreground(greenCol).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(greenCol).
			Padding(0, 1).
			Bold(true)

	badgeShortStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#450A0A")).
			Foreground(redCol).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(redCol).
			Padding(0, 1).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedCol).
			Background(cardBg).
			Padding(0, 1)

	buttonStyle = lipgloss.NewStyle().
			Background(accentCol).
			Foreground(lipgloss.Color("#0B0E14")).
			Bold(true).
			Padding(0, 2).
			Margin(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentCol)

	buttonInactiveStyle = lipgloss.NewStyle().
				Background(cardBg).
				Foreground(mutedCol).
				Padding(0, 2).
				Margin(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(cardBd)

	buttonFocusStyle = lipgloss.NewStyle().
				Background(purpleCol).
				Foreground(lipgloss.Color("#0B0E14")).
				Bold(true).
				Padding(0, 2).
				Margin(0, 1).
				Border(lipgloss.DoubleBorder()).
				BorderForeground(purpleCol)

	spinnerStyle = lipgloss.NewStyle().Foreground(accent2).Bold(true)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(mutedCol).
				Background(lipgloss.Color("#0F172A")).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(cardBd)

	tableCellStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("#E5E7EB"))

	logoStyle = lipgloss.NewStyle().
			Foreground(accentCol).
			Bold(true).
			Background(lipgloss.Color("#0F172A")).
			Padding(0, 1)
)

func focusCard(focused bool) lipgloss.Style {
	if focused {
		return cardFocusStyle
	}
	return cardStyle
}

func kpiCard(label, value, delta string, positive bool) string {
	// delta line
	deltaCol := mutedCol
	if positive {
		deltaCol = greenCol
	} else {
		deltaCol = redCol
	}
	deltaStyle := lipgloss.NewStyle().Foreground(deltaCol).Align(lipgloss.Center).Width(18)
	labelR := kpiLabelStyle.Render(label)
	valueR := kpiValueStyle.Render(value)
	deltaR := deltaStyle.Render(delta)
	return lipgloss.JoinVertical(lipgloss.Center, labelR, valueR, deltaR)
}
