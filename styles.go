package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

var (
	blackBgOn bool

	colorAccent  = lipgloss.Color("#ff5fd2") // magenta
	colorAccent2 = lipgloss.Color("#5fe1ff") // cyan
	colorMuted   = lipgloss.Color("#6c6c6c")
	colorDim     = lipgloss.Color("#8a8a8a")
	colorBorder  = lipgloss.Color("#3a3a3a")
	colorBlack   = lipgloss.Color("#000000")

	styleMuted      lipgloss.Style
	styleBold       lipgloss.Style
	styleDim        lipgloss.Style
	styleDimItalic  lipgloss.Style
	styleBoldRed    lipgloss.Style
	styleBoldGreen  lipgloss.Style
	styleBoldYellow lipgloss.Style
	styleAccent     lipgloss.Style
	styleAccent2    lipgloss.Style
	styleLogo       lipgloss.Style
	styleBar        lipgloss.Style
	styleBarDim     lipgloss.Style
	styleBlackBg    lipgloss.Style
)

const barChar = "▎"

// withBg conditionally bakes the black background into a style, so every
// rendered cell carries bg=black instead of relying on an outer wrap (which
// only fills padding cells, leaving terminal default to bleed through).
func withBg(s lipgloss.Style) lipgloss.Style {
	if blackBgOn {
		return s.Background(colorBlack)
	}
	return s
}

func rebuildTheme() {
	styleMuted = withBg(lipgloss.NewStyle().Foreground(colorMuted))
	styleBold = withBg(lipgloss.NewStyle().Bold(true))
	styleDim = withBg(lipgloss.NewStyle().Foreground(colorDim))
	styleDimItalic = withBg(lipgloss.NewStyle().Foreground(colorDim).Italic(true))
	styleBoldRed = withBg(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5555")))
	styleBoldGreen = withBg(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50fa7b")))
	styleBoldYellow = withBg(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f1fa8c")))
	styleAccent = withBg(lipgloss.NewStyle().Foreground(colorAccent))
	styleAccent2 = withBg(lipgloss.NewStyle().Foreground(colorAccent2))
	styleLogo = withBg(lipgloss.NewStyle().Foreground(colorAccent).Bold(true))
	styleBar = withBg(lipgloss.NewStyle().Foreground(colorAccent).Bold(true))
	styleBarDim = withBg(lipgloss.NewStyle().Foreground(colorBorder))
	styleBlackBg = lipgloss.NewStyle().Background(colorBlack)
}

func init() { rebuildTheme() }

// styleInput applies the current theme background to a bubbles textinput
// so its prompt, text, placeholder, and cursor cells all carry bg=black
// when the toggle is on (otherwise leaves them at terminal default).
func styleInput(ti *textinput.Model) {
	ti.PromptStyle = withBg(lipgloss.NewStyle())
	ti.TextStyle = withBg(lipgloss.NewStyle())
	ti.PlaceholderStyle = withBg(lipgloss.NewStyle().Foreground(colorMuted))
	ti.Cursor.Style = withBg(lipgloss.NewStyle())
	ti.Cursor.TextStyle = withBg(lipgloss.NewStyle())
}

func renderBrand(subtitle string) string {
	out := "  " + styleLogo.Render("▌▌") + " " + styleLogo.Render("lk")
	if subtitle != "" {
		out += styleDim.Render(" · ") + styleDim.Render(subtitle)
	}
	return out
}
