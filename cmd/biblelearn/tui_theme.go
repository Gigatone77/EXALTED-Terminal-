package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Dark palette for EXALTED Terminal. Colors are drawn from a dark, low-glare
// scheme with an amber accent so it reads well over long study sessions.
var (
	tBg       = lipgloss.Color("#0d1117")
	tFg       = lipgloss.Color("#e6edf3")
	tAccent   = lipgloss.Color("#f5a623")
	tAccentHi = lipgloss.Color("#ffd479")
	tMuted    = lipgloss.Color("#8b949e")
	tDim      = lipgloss.Color("#484f58")
	tBorder   = lipgloss.Color("#30363d")
	tSel      = lipgloss.Color("#1f6feb")
	tWarn     = lipgloss.Color("#f85149")
	tOk       = lipgloss.Color("#3fb950")
)

var (
	styleBase    = lipgloss.NewStyle().Background(tBg).Foreground(tFg)
	styleTitle   = lipgloss.NewStyle().Foreground(tAccentHi).Bold(true).Padding(0, 1)
	styleHint    = lipgloss.NewStyle().Foreground(tMuted).Padding(0, 1)
	styleList    = lipgloss.NewStyle().Padding(0, 2)
	styleSel     = lipgloss.NewStyle().Background(tSel).Foreground(tFg).Bold(true).Padding(0, 1)
	styleBody    = lipgloss.NewStyle().Padding(0, 2).Width(112)
	styleMuted   = lipgloss.NewStyle().Foreground(tMuted)
	styleDim     = lipgloss.NewStyle().Foreground(tDim)
	styleBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(tBorder).Padding(1, 2)
	styleTabSel  = lipgloss.NewStyle().Background(tAccent).Foreground(tBg).Bold(true).Padding(0, 1)
	styleTabIdle = lipgloss.NewStyle().Foreground(tMuted).Padding(0, 1)
)
