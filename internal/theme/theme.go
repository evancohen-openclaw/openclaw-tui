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
	Text:            "#D4D4D4",
	Dim:             "#6B7280",
	Accent:          "#A78BFA", // soft lavender
	AccentSoft:      "#818CF8", // muted indigo
	Border:          "#2E3440",
	UserBg:          "#1E293B", // deep slate
	UserText:        "#E2E8F0",
	SystemText:      "#64748B",
	ToolPendingBg:   "#1A2332",
	ToolSuccessBg:   "#162321",
	ToolErrorBg:     "#2D1B1B",
	ToolTitle:       "#A78BFA",
	ToolOutput:      "#CBD5E1",
	Error:           "#F87171",
	Success:         "#6EE7B7",
	ThinkingText:    "#64748B",
	ThinkingBorder:  "#334155",
	OverlayBg:       "#0F172A",
	OverlayActiveBg: "#1E293B",
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
// Accepts an optional override from config ("dark", "light", or "").
func IsLight(override string) bool {
	src := strings.ToLower(override)
	if src == "" {
		src = strings.ToLower(os.Getenv("OPENCLAW_THEME"))
	}
	if src == "light" {
		return true
	}
	// Default to dark
	return false
}

// New creates a theme. Pass themeOverride from config (or "").
func New(themeOverride ...string) Theme {
	ov := ""
	if len(themeOverride) > 0 {
		ov = themeOverride[0]
	}
	light := IsLight(ov)
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
