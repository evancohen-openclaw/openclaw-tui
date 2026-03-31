package theme

import (
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme holds all styled renderers for the TUI.
type Theme struct {
	Light bool

	// Text styles
	Text       lipgloss.Style
	Dim        lipgloss.Style
	Accent     lipgloss.Style
	AccentSoft lipgloss.Style
	Success    lipgloss.Style
	Error      lipgloss.Style
	System     lipgloss.Style
	Bold       lipgloss.Style

	// Chat styles
	UserMsg      lipgloss.Style
	AssistantMsg lipgloss.Style
	ToolPending  lipgloss.Style
	ToolSuccess  lipgloss.Style
	ToolError    lipgloss.Style
	ToolTitle    lipgloss.Style
	Thinking     lipgloss.Style

	// Overlay
	OverlayBorder    lipgloss.Style
	OverlayTitle     lipgloss.Style
	OverlayItem      lipgloss.Style
	OverlayItemActive lipgloss.Style

	// Autocomplete
	AutocompleteItem       lipgloss.Style
	AutocompleteItemActive lipgloss.Style

	// Layout
	Header lipgloss.Style
	Footer lipgloss.Style
	Status lipgloss.Style
	Border lipgloss.Style
}

// Palette holds the raw color values.
type Palette struct {
	Text          string
	Dim           string
	Accent        string
	AccentSoft    string
	Border        string
	UserBg        string
	UserText      string
	SystemText    string
	ToolPendingBg string
	ToolSuccessBg string
	ToolErrorBg   string
	ToolTitle     string
	ToolOutput    string
	Error         string
	Success       string
	ThinkingText  string
	ThinkingBorder string
	OverlayBg     string
	OverlayActiveBg string
}

var DarkPalette = Palette{
	Text:            "#E8E3D5",
	Dim:             "#7B7F87",
	Accent:          "#F6C453",
	AccentSoft:      "#F2A65A",
	Border:          "#3C414B",
	UserBg:          "#2B2F36",
	UserText:        "#F3EEE0",
	SystemText:      "#9BA3B2",
	ToolPendingBg:   "#1F2A2F",
	ToolSuccessBg:   "#1E2D23",
	ToolErrorBg:     "#2F1F1F",
	ToolTitle:       "#F6C453",
	ToolOutput:      "#E1DACB",
	Error:           "#F97066",
	Success:         "#7DD3A5",
	ThinkingText:    "#7B7F87",
	ThinkingBorder:  "#3C414B",
	OverlayBg:       "#1E2024",
	OverlayActiveBg: "#2B2F36",
}

var LightPalette = Palette{
	Text:            "#1E1E1E",
	Dim:             "#5B6472",
	Accent:          "#B45309",
	AccentSoft:      "#C2410C",
	Border:          "#5B6472",
	UserBg:          "#F3F0E8",
	UserText:        "#1E1E1E",
	SystemText:      "#4B5563",
	ToolPendingBg:   "#EFF6FF",
	ToolSuccessBg:   "#ECFDF5",
	ToolErrorBg:     "#FEF2F2",
	ToolTitle:       "#B45309",
	ToolOutput:      "#374151",
	Error:           "#DC2626",
	Success:         "#047857",
	ThinkingText:    "#5B6472",
	ThinkingBorder:  "#D1D5DB",
	OverlayBg:       "#FFFFFF",
	OverlayActiveBg: "#F3F0E8",
}

// IsLight detects if the terminal has a light background.
func IsLight() bool {
	env := strings.ToLower(os.Getenv("OPENCLAW_THEME"))
	if env == "light" {
		return true
	}
	if env == "dark" {
		return false
	}
	// Default to dark
	return false
}

// New creates a theme based on terminal detection.
func New() Theme {
	light := IsLight()
	p := DarkPalette
	if light {
		p = LightPalette
	}

	return Theme{
		Light: light,

		Text:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)),
		Dim:        lipgloss.NewStyle().Foreground(lipgloss.Color(p.Dim)),
		Accent:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)),
		AccentSoft: lipgloss.NewStyle().Foreground(lipgloss.Color(p.AccentSoft)),
		Success:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.Success)),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.Error)),
		System:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.SystemText)),
		Bold:       lipgloss.NewStyle().Bold(true),

		UserMsg: lipgloss.NewStyle().
			Background(lipgloss.Color(p.UserBg)).
			Foreground(lipgloss.Color(p.UserText)).
			Padding(0, 1),

		AssistantMsg: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Text)).
			Padding(0, 1),

		ToolPending: lipgloss.NewStyle().
			Background(lipgloss.Color(p.ToolPendingBg)).
			Foreground(lipgloss.Color(p.ToolTitle)).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.Border)),

		ToolSuccess: lipgloss.NewStyle().
			Background(lipgloss.Color(p.ToolSuccessBg)).
			Foreground(lipgloss.Color(p.ToolOutput)).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.Border)),

		ToolError: lipgloss.NewStyle().
			Background(lipgloss.Color(p.ToolErrorBg)).
			Foreground(lipgloss.Color(p.Error)).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.Border)),

		ToolTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ToolTitle)).
			Bold(true),

		Thinking: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ThinkingText)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.ThinkingBorder)).
			Padding(0, 1),

		OverlayBorder: lipgloss.NewStyle().
			Background(lipgloss.Color(p.OverlayBg)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.Border)).
			Padding(1, 2),

		OverlayTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Bold(true),

		OverlayItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Text)).
			Padding(0, 1),

		OverlayItemActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Background(lipgloss.Color(p.OverlayActiveBg)).
			Bold(true).
			Padding(0, 1),

		AutocompleteItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Dim)).
			Padding(0, 1),

		AutocompleteItemActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Background(lipgloss.Color(p.OverlayActiveBg)).
			Bold(true).
			Padding(0, 1),

		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Bold(true).
			Padding(0, 1),

		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Dim)).
			Padding(0, 1),

		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Dim)).
			Padding(0, 1),

		Border: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Border)),
	}
}
