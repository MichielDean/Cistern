package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Semantic color roles for the arch renderer.
// Use these names instead of inline hex values to keep the palette legible and easy to retheme.
var (
	// archRoleBackground is a black-background cell for blank pixels in a tiled pillar.
	// The black background prevents color bleed between adjacent pillars.
	archRoleBackground = lipgloss.NewStyle().Background(lipgloss.Color("0"))

	// archRoleEdge is the shadow/edge color applied to '░' pixels.
	archRoleEdge = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Background(lipgloss.Color("0"))

	// archRoleIdle is the stone color for '▒' fill pixels when the step is not active.
	archRoleIdle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Background(lipgloss.Color("0"))

	// archRoleActive is the highlight color for '▒' fill pixels when the step is active.
	archRoleActive = lipgloss.NewStyle().Foreground(lipgloss.Color("#4bb96e")).Background(lipgloss.Color("0"))

	// archRoleDrought is the dim color used for all pixels in the drought (all-idle) arch.
	// No background is set because the drought arch is a single centred pillar — no bleed risk.
	archRoleDrought = lipgloss.NewStyle().Foreground(lipgloss.Color("#46465a"))

	// archRoleChannelWall is the color for channel wall characters (▀ top, █ sides).
	archRoleChannelWall = lipgloss.NewStyle().Foreground(lipgloss.Color("#46465a")).Background(lipgloss.Color("0"))

	// archRoleWaterBright / archRoleWaterMid / archRoleWaterDim define the three-level
	// brightness palette for the animated water channel and waterfall.
	archRoleWaterBright = lipgloss.NewStyle().Foreground(lipgloss.Color("#a8eeff")).Background(lipgloss.Color("0"))
	archRoleWaterMid    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3ec8e8")).Background(lipgloss.Color("0"))
	archRoleWaterDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a7a96")).Background(lipgloss.Color("0"))
)

// Pixel values used in archPixelMap. Each cell holds exactly one of these runes.
const (
	pxBlank rune = ' ' // transparent / background
	pxEdge  rune = '░' // arch shadow / edge shadow
	pxFill  rune = '▒' // arch stone fill
)

// archPillarW and archPillarH are the fixed dimensions of a single pillar tile.
// These match the 36x12 mipmap dimensions so the drought arch aligns with the
// active-arch pixel-art slot width.
const (
	archPillarW = 20
	archPillarH = 7
)

// The hand-drawn archPixelMap and its render functions have been removed.
// All arch rendering now uses the pre-rendered chafa mipmap (arch_20x7.ansi).
