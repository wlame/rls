package tui

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// endpointState holds live per-endpoint metrics tracked by the TUI.
type endpointState struct {
	cfg         config.EndpointConfig
	enqueuedAt  []time.Time // one entry per currently queued request
	served      int64
	rejected    int64
	waitSamples []int64 // last 200 RequestServed wait times (ms)
	lastWaitMs  int64   // most recent serve wait time
}

// Model is the Bubble Tea model for the interactive TUI.
type Model struct {
	endpoints  []endpointState
	selected   int
	paused     bool
	width      int
	height     int
	events     <-chan endpoint.Event
	serverAddr string
	lastStatus string
	warnAfter  time.Duration
	critAfter  time.Duration
}

// NewModel creates a Model pre-populated from the server config.
func NewModel(cfg *config.Config, events <-chan endpoint.Event, thresholds DotThresholds) Model {
	states := make([]endpointState, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		states[i] = endpointState{cfg: ep}
	}
	addr := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	return Model{
		endpoints:  states,
		events:     events,
		serverAddr: addr,
		width:      80,
		height:     24,
		warnAfter:  thresholds.Warn,
		critAfter:  thresholds.Crit,
	}
}

// Init starts the event listener and the 100ms refresh tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.events),
		tickEvery(),
	)
}

func tickEvery() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Update handles all Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		if m.paused {
			return m, tickEvery()
		}
		return m, tickEvery()

	case serverEventMsg:
		if !m.paused {
			var cmd tea.Cmd
			m, cmd = m.handleServerEvent(msg.ev)
			return m, tea.Batch(cmd, waitForEvent(m.events))
		}
		return m, waitForEvent(m.events)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}

	case "down", "j":
		if m.selected < len(m.endpoints)-1 {
			m.selected++
		}

	case "r":
		if len(m.endpoints) > 0 {
			m.endpoints[m.selected].served = 0
			m.endpoints[m.selected].rejected = 0
			m.endpoints[m.selected].waitSamples = nil
			m.endpoints[m.selected].lastWaitMs = 0
			m.lastStatus = fmt.Sprintf("stats reset for %s", m.endpoints[m.selected].cfg.Path)
		}

	case "p":
		m.paused = !m.paused

	case " ":
		if len(m.endpoints) > 0 {
			path := m.endpoints[m.selected].cfg.Path
			return m, injectCmd(m.serverAddr, path)
		}
	}
	return m, nil
}

func (m Model) handleServerEvent(ev endpoint.Event) (Model, tea.Cmd) {
	idx := m.indexForPath(ev.Path)
	if idx < 0 {
		return m, nil
	}
	st := &m.endpoints[idx]

	switch ev.Kind {
	case endpoint.EventQueued:
		st.enqueuedAt = append(st.enqueuedAt, time.Now())

	case endpoint.EventServed:
		if len(st.enqueuedAt) > 0 {
			st.enqueuedAt = st.enqueuedAt[1:]
		}
		st.served++
		st.lastWaitMs = ev.WaitedMs
		st.waitSamples = appendSample(st.waitSamples, ev.WaitedMs)
		m.lastStatus = fmt.Sprintf("served %-12s  waited=%dms  queue=%d",
			ev.Path, ev.WaitedMs, ev.QueueDepth)
		if idx == m.selected {
			return m, belCmd()
		}

	case endpoint.EventRejected:
		st.rejected++
	}

	return m, nil
}

func (m Model) indexForPath(path string) int {
	for i, st := range m.endpoints {
		if st.cfg.Path == path {
			return i
		}
	}
	return -1
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Reserve 1 line for title, 1 for status bar; rest for endpoint rows.
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	// Column widths (subtract 2 for the two dividers).
	leftW := m.width * 28 / 100
	rightW := m.width * 27 / 100
	midW := m.width - leftW - rightW - 2
	if midW < 10 {
		midW = 10
	}

	// --- Title bar ---
	title := titleStyle.Render(fmt.Sprintf(" rls  %s", m.serverAddr))
	if m.paused {
		title += pausedStyle
	}
	titleLine := lipgloss.NewStyle().Width(m.width).Render(title)

	// --- Body rows ---
	n := len(m.endpoints)
	leftLines := make([]string, n)
	midLines := make([]string, n)

	for i, st := range m.endpoints {
		leftLines[i] = m.renderLeftRow(i, st, leftW)
		midLines[i] = m.renderMidRow(st, midW)
	}

	rightLines := m.renderRightColumn(rightW, bodyHeight, n)

	// Pad all columns to bodyHeight lines.
	padTo(leftLines, bodyHeight, leftW)
	padTo(midLines, bodyHeight, midW)

	// Join columns row by row.
	var rows []string
	for i := 0; i < bodyHeight; i++ {
		var l, mid, r string
		if i < len(leftLines) {
			l = leftLines[i]
		} else {
			l = strings.Repeat(" ", leftW)
		}
		if i < len(midLines) {
			mid = midLines[i]
		} else {
			mid = strings.Repeat(" ", midW)
		}
		if i < len(rightLines) {
			r = rightLines[i]
		} else {
			r = strings.Repeat(" ", rightW)
		}
		rows = append(rows, l+divider+mid+divider+r)
	}

	// --- Status bar ---
	help := " q quit  r reset  p pause  ↑↓/jk select  space inject"
	if m.lastStatus != "" {
		help += "  │  " + m.lastStatus
	}
	statusLine := helpStyle.Width(m.width).Render(help)

	return titleLine + "\n" + strings.Join(rows, "\n") + "\n" + statusLine
}

func (m Model) renderLeftRow(idx int, st endpointState, width int) string {
	cfg := st.cfg
	unit := cfg.Unit
	sched := strings.ToUpper(cfg.Scheduler)
	if sched == "PRIORITY" {
		sched = "PRIOR"
	}
	label := fmt.Sprintf(" %s  %s %.0f%s", cfg.Path, sched, cfg.Rate, unit)

	var cursor string
	if idx == m.selected {
		cursor = cursorStyle.Render("▶")
		label = selectedRowStyle.Width(width - 1).Render(label)
	} else {
		cursor = " "
		label = normalRowStyle.Width(width - 1).Render(label)
	}
	return cursor + label
}

func (m Model) renderMidRow(st endpointState, width int) string {
	maxQ := st.cfg.MaxQueueSize
	queued := len(st.enqueuedAt)

	// Right-aligned counter: " [N/M]"
	counter := counterStyle.Render(fmt.Sprintf("[%d/%d]", queued, maxQ))
	counterLen := utf8.RuneCountInString(fmt.Sprintf("[%d/%d]", queued, maxQ))

	// Dots fill the remaining space.
	dotsWidth := width - counterLen - 1 // 1 for space before counter
	if dotsWidth < 0 {
		dotsWidth = 0
	}

	var dots strings.Builder
	now := time.Now()
	for i, t := range st.enqueuedAt {
		if i >= dotsWidth {
			break
		}
		age := now.Sub(t)
		switch {
		case age < m.warnAfter:
			dots.WriteString(dotGreen)
		case age < m.critAfter:
			dots.WriteString(dotYellow)
		default:
			dots.WriteString(dotRed)
		}
	}

	// Pad dots area with spaces (visual only, no ANSI in padding).
	dotCount := min(queued, dotsWidth)
	padding := strings.Repeat(" ", dotsWidth-dotCount)

	return dots.String() + padding + " " + counter
}

func (m Model) renderRightColumn(width, height, endpointRows int) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}

	if len(m.endpoints) == 0 || m.selected >= len(m.endpoints) {
		return lines
	}

	st := m.endpoints[m.selected]
	p50, p99 := computePercentiles(st.waitSamples)

	stats := []struct{ label, value string }{
		{"served:  ", fmt.Sprintf("%d", st.served)},
		{"rejected:", fmt.Sprintf("%d", st.rejected)},
		{"p50:     ", fmt.Sprintf("%dms", p50)},
		{"p99:     ", fmt.Sprintf("%dms", p99)},
		{"last:    ", fmt.Sprintf("%dms", st.lastWaitMs)},
	}

	// Place stats starting at the selected endpoint row (aligned with left column).
	startRow := m.selected
	if startRow >= height {
		startRow = 0
	}

	for i, s := range stats {
		row := startRow + i
		if row >= height {
			break
		}
		label := statLabelStyle.Render(s.label)
		value := statValueStyle.Render(s.value)
		line := " " + label + " " + value
		// Truncate or pad to width.
		visLen := utf8.RuneCountInString(s.label) + utf8.RuneCountInString(s.value) + 3
		if visLen < width {
			line += strings.Repeat(" ", width-visLen)
		}
		lines[row] = line
	}

	return lines
}

// computePercentiles returns the p50 and p99 of samples (in ms).
// Returns 0,0 for empty or single-element slices.
func computePercentiles(samples []int64) (p50, p99 int64) {
	n := len(samples)
	if n == 0 {
		return 0, 0
	}
	cp := make([]int64, n)
	copy(cp, samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	p50 = cp[(n-1)*50/100]
	p99 = cp[(n-1)*99/100]
	return p50, p99
}

// appendSample appends v to samples, capping at 200 entries.
func appendSample(samples []int64, v int64) []int64 {
	samples = append(samples, v)
	if len(samples) > 200 {
		samples = samples[len(samples)-200:]
	}
	return samples
}

// padTo ensures lines has exactly n entries, each padded to width visible chars.
func padTo(lines []string, n, width int) {
	for len(lines) < n {
		lines = append(lines, strings.Repeat(" ", width))
	}
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// belCmd returns a Cmd that writes the BEL character to stdout.
func belCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stdout, "\a")
		return nil
	}
}

// injectCmd fires a non-blocking HTTP GET to path on addr.
func injectCmd(addr, path string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(addr + path + "?_tui=1") //nolint:noctx
		if err == nil {
			resp.Body.Close()
		}
		return nil
	}
}
