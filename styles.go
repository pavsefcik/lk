package main

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent  = lipgloss.Color("#0ea5e9")
	colorSelected = lipgloss.Color("#0c3a52")
	colorMuted   = lipgloss.Color("#6c6c6c")

	styleSelected   = lipgloss.NewStyle().Background(colorSelected)
	styleMuted      = lipgloss.NewStyle().Foreground(colorMuted)
	styleBold       = lipgloss.NewStyle().Bold(true)
	styleDim        = lipgloss.NewStyle().Faint(true)
	styleDimItalic  = lipgloss.NewStyle().Faint(true).Italic(true)
	styleBoldRed    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff5555"))
	styleBoldGreen  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50fa7b"))
	styleBoldYellow = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f1fa8c"))
	styleAccent     = lipgloss.NewStyle().Foreground(colorAccent)
)
