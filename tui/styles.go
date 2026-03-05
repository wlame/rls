package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Column dividers
	divider = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("│")

	// Endpoint list (left column)
	selectedRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	normalRowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cursorStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))

	// Dot colours by age
	dotGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")  // < 500ms
	dotYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("●")  // 500ms – 2s
	dotRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("●")  // > 2s

	// Queue counter (right-aligned in middle column)
	counterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Stats column
	statLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Title bar
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))

	// Status / help bar
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Paused indicator
	pausedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Render(" [PAUSED]")

	// Attached indicator
	attachedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))

	// Log panel
	logSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	logLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Tree rendering: configured vs dynamic endpoints
	configuredRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dynamicRowStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	treeConnectorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)
