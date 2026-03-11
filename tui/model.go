package tui

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wlame/rls/attach"
	"github.com/wlame/rls/config"
	"github.com/wlame/rls/endpoint"
)

// endpointState holds live per-endpoint metrics tracked by the TUI.
type endpointState struct {
	cfg          config.EndpointConfig
	enqueuedAt   []time.Time // one entry per currently queued request
	served       int64
	rejected     int64
	waitSamples  []int64 // last 200 RequestServed wait times (ms)
	lastWaitMs   int64   // most recent serve wait time
	totalWaitMs  int64   // cumulative wait across all served requests
	dynamic      bool
}

// treeEntry is the computed tree label for rendering.
type treeEntry struct {
	label   string
	depth   int
	dynamic bool
}

// Model is the Bubble Tea model for the interactive TUI.
type Model struct {
	endpoints   []endpointState
	selected    int
	paused      bool
	width       int
	height      int
	events      <-chan endpoint.Event
	logCh       <-chan string
	logLines    []string
	serverAddr  string
	lastStatus  string
	warnAfter   time.Duration
	critAfter   time.Duration
	showInfo    bool
	attachedPID int
	registry    *endpoint.Registry
	treeLabels  []treeEntry
}

// NewModel creates a Model pre-populated from the server config.
// logCh may be nil (log panel stays empty).
// snapshots (optional) seeds the queue dots from attach state.
func NewModel(cfg *config.Config, events <-chan endpoint.Event, thresholds DotThresholds, logCh <-chan string, attachedPID int, snapshots []attach.EndpointSnapshot) Model {
	states := make([]endpointState, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		states[i] = endpointState{cfg: ep}
	}

	// Seed queue dots from attach snapshots.
	now := time.Now()
	for _, snap := range snapshots {
		for i := range states {
			if states[i].cfg.Path == snap.Path && snap.QueueDepth > 0 {
				states[i].enqueuedAt = make([]time.Time, snap.QueueDepth)
				for j := range states[i].enqueuedAt {
					states[i].enqueuedAt[j] = now
				}
				break
			}
		}
	}

	treeLabels := buildTreeLabels(states)

	addr := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	return Model{
		endpoints:   states,
		events:      events,
		logCh:       logCh,
		serverAddr:  addr,
		width:       80,
		height:      24,
		warnAfter:   thresholds.Warn,
		critAfter:   thresholds.Crit,
		showInfo:    true,
		attachedPID: attachedPID,
		treeLabels:  treeLabels,
	}
}

// Init starts the event listener, log listener, and the 100ms refresh tick.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.events),
		waitForLog(m.logCh),
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
		m.syncEndpoints()
		return m, tickEvery()

	case serverEventMsg:
		if !m.paused {
			var cmd tea.Cmd
			m, cmd = m.handleServerEvent(msg.ev)
			return m, tea.Batch(cmd, waitForEvent(m.events))
		}
		return m, waitForEvent(m.events)

	case disconnectedMsg:
		m.lastStatus = "disconnected"
		return m, tea.Quit

	case logLineMsg:
		m.logLines = append(m.logLines, msg.line)
		if len(m.logLines) > 200 {
			m.logLines = m.logLines[len(m.logLines)-200:]
		}
		return m, waitForLog(m.logCh)
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
		if len(m.endpoints) > 0 && m.selected < len(m.endpoints) {
			st := &m.endpoints[m.selected]
			st.served = 0
			st.rejected = 0
			st.waitSamples = nil
			st.lastWaitMs = 0
			st.totalWaitMs = 0
			m.lastStatus = fmt.Sprintf("stats reset for %s", st.cfg.Path)
		}

	case "p":
		m.paused = !m.paused

	case "i":
		m.showInfo = !m.showInfo

	case " ":
		if len(m.endpoints) > 0 && m.selected < len(m.endpoints) {
			path := m.endpoints[m.selected].cfg.Path
			return m, injectCmd(m.serverAddr, path)
		}
	}
	return m, nil
}

func (m Model) handleServerEvent(ev endpoint.Event) (Model, tea.Cmd) {
	idx := m.indexForPath(ev.Path)
	if idx < 0 {
		// Unknown path — create a dynamic endpoint inheriting from nearest parent.
		m = m.addDynamicEndpoint(ev.Path)
		idx = m.indexForPath(ev.Path)
		if idx < 0 {
			return m, nil
		}
	}
	st := &m.endpoints[idx]

	switch ev.Kind {
	case endpoint.EventQueued:
		switch st.cfg.Scheduler {
		case "lifo":
			// Newest at left — leftmost is served next.
			st.enqueuedAt = append([]time.Time{time.Now()}, st.enqueuedAt...)
		case "random":
			// Insert at a random position to reflect unpredictable serve order.
			pos := rand.Intn(len(st.enqueuedAt) + 1)
			st.enqueuedAt = append(st.enqueuedAt, time.Time{})
			copy(st.enqueuedAt[pos+1:], st.enqueuedAt[pos:])
			st.enqueuedAt[pos] = time.Now()
		default:
			// fifo / priority: newest at right, oldest (leftmost) served first.
			st.enqueuedAt = append(st.enqueuedAt, time.Now())
		}

	case endpoint.EventServed:
		if len(st.enqueuedAt) > 0 {
			if st.cfg.Scheduler == "random" {
				i := rand.Intn(len(st.enqueuedAt))
				st.enqueuedAt = append(st.enqueuedAt[:i], st.enqueuedAt[i+1:]...)
			} else {
				st.enqueuedAt = st.enqueuedAt[1:]
			}
		}
		st.served++
		st.lastWaitMs = ev.WaitedMs
		st.totalWaitMs += ev.WaitedMs
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

// addDynamicEndpoint creates a new endpoint state for an unknown path by
// inheriting config from the nearest existing parent. This allows the TUI
// to discover dynamic endpoints from the event stream (attach mode).
func (m Model) addDynamicEndpoint(eventPath string) Model {
	// Find nearest parent by walking up the path.
	var parent *endpointState
	tmp := eventPath
	for {
		dir := strings.TrimRight(tmp, "/")
		idx := strings.LastIndex(dir, "/")
		if idx < 0 {
			break
		}
		candidate := dir[:idx]
		if candidate == "" {
			candidate = "/"
		}
		if candidate != eventPath {
			if pi := m.indexForPath(candidate); pi >= 0 {
				parent = &m.endpoints[pi]
				break
			}
		}
		if candidate == "/" {
			break
		}
		tmp = candidate
	}

	var newCfg config.EndpointConfig
	if parent != nil {
		newCfg = config.InheritFrom(config.EndpointConfig{Path: eventPath, Dynamic: true}, parent.cfg)
	} else {
		// No parent found — use minimal defaults.
		newCfg = config.EndpointConfig{
			Path:         eventPath,
			Dynamic:      true,
			Rate:         1,
			Unit:         "rps",
			Scheduler:    "fifo",
			Algorithm:    "strict",
			MaxQueueSize: 1000,
			Overflow:     "reject",
		}
	}

	newState := endpointState{cfg: newCfg, dynamic: true}

	// Insert in sorted order by path.
	inserted := false
	newEndpoints := make([]endpointState, 0, len(m.endpoints)+1)
	for _, st := range m.endpoints {
		if !inserted && eventPath < st.cfg.Path {
			newEndpoints = append(newEndpoints, newState)
			inserted = true
		}
		newEndpoints = append(newEndpoints, st)
	}
	if !inserted {
		newEndpoints = append(newEndpoints, newState)
	}

	m.endpoints = newEndpoints
	m.treeLabels = buildTreeLabels(m.endpoints)

	// Adjust selected index if insertion shifted it.
	if m.selected >= len(m.endpoints) {
		m.selected = len(m.endpoints) - 1
	}

	return m
}

// syncEndpoints updates the endpoint list from the registry snapshot,
// inserting new dynamic endpoints while preserving existing stats.
func (m *Model) syncEndpoints() {
	if m.registry == nil {
		return
	}
	snap := m.registry.Snapshot()

	// Build lookup from current endpoints.
	existing := make(map[string]int, len(m.endpoints))
	for i, st := range m.endpoints {
		existing[st.cfg.Path] = i
	}

	// Build new list in snapshot order, preserving existing stats.
	newStates := make([]endpointState, 0, len(snap))
	for _, info := range snap {
		if idx, ok := existing[info.Config.Path]; ok {
			// Preserve existing state, update cfg in case it changed.
			st := m.endpoints[idx]
			st.cfg = info.Config
			st.dynamic = info.Config.Dynamic
			newStates = append(newStates, st)
		} else {
			newStates = append(newStates, endpointState{
				cfg:     info.Config,
				dynamic: info.Config.Dynamic,
			})
		}
	}

	m.endpoints = newStates
	m.treeLabels = buildTreeLabels(m.endpoints)

	// Clamp selection.
	if m.selected >= len(m.endpoints) {
		m.selected = len(m.endpoints) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// buildTreeLabels computes tree labels for rendering endpoints as a tree.
// Endpoints must be sorted by path. Max depth is 3; deeper paths flatten with "/".
func buildTreeLabels(endpoints []endpointState) []treeEntry {
	entries := make([]treeEntry, len(endpoints))
	// Build set of registered paths for parent lookup.
	registered := make(map[string]bool, len(endpoints))
	for _, st := range endpoints {
		registered[st.cfg.Path] = true
	}

	for i, st := range endpoints {
		p := st.cfg.Path
		entries[i].dynamic = st.dynamic

		// Walk up to find the nearest registered parent.
		depth := 0
		parent := ""
		tmp := p
		for {
			dir := strings.TrimRight(tmp, "/")
			idx := strings.LastIndex(dir, "/")
			if idx < 0 {
				break
			}
			candidate := dir[:idx]
			if candidate == "" {
				candidate = "/"
			}
			if candidate != p && registered[candidate] {
				parent = candidate
				break
			}
			if candidate == "/" {
				break
			}
			tmp = candidate
		}

		if parent == "" || parent == p {
			// No tree parent found — render as full path at depth 0.
			entries[i].label = p
			entries[i].depth = 0
			continue
		}

		// Compute depth by counting ancestors.
		depth = 0
		ancestor := parent
		for ancestor != "" {
			depth++
			// Find ancestor's parent.
			found := false
			tmp2 := ancestor
			for {
				dir := strings.TrimRight(tmp2, "/")
				idx := strings.LastIndex(dir, "/")
				if idx < 0 {
					break
				}
				cand := dir[:idx]
				if cand == "" {
					cand = "/"
				}
				if cand != ancestor && registered[cand] {
					ancestor = cand
					found = true
					break
				}
				if cand == "/" {
					break
				}
				tmp2 = cand
			}
			if !found {
				break
			}
		}

		// Cap depth at 3, flatten deeper paths.
		label := strings.TrimPrefix(p, parent)
		label = strings.TrimPrefix(label, "/")
		if depth > 3 {
			depth = 3
		}

		entries[i].label = label
		entries[i].depth = depth
	}
	return entries
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	// Reserve 1 line for title, 1 for status bar; split remainder between
	// the 3-column endpoint area (top ~2/3) and the log panel (bottom ~1/3).
	totalAvail := m.height - 2
	if totalAvail < 2 {
		totalAvail = 2
	}
	logHeight := totalAvail / 3
	if logHeight < 2 {
		logHeight = 2
	}
	bodyHeight := totalAvail - logHeight - 1 // -1 for the separator line
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	// Column widths.
	leftW := m.width * 28 / 100
	rightW := 0
	if m.showInfo {
		rightW = max(22, m.width*18/100)
	}
	divCount := 1 // always one divider between left and mid
	if m.showInfo {
		divCount = 2
	}
	midW := m.width - leftW - rightW - divCount
	if midW < 10 {
		midW = 10
	}

	// --- Title bar ---
	title := titleStyle.Render(fmt.Sprintf(" rls  %s", m.serverAddr))
	if m.attachedPID != 0 {
		title += attachedStyle.Render(fmt.Sprintf("  [attached: PID %d]", m.attachedPID))
	}
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

	var rightLines []string
	if m.showInfo {
		rightLines = m.renderRightColumn(rightW, bodyHeight)
	}

	// Join columns row by row.
	var rows []string
	for i := 0; i < bodyHeight; i++ {
		var l, mid string
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
		row := l + divider + mid
		if m.showInfo {
			var r string
			if i < len(rightLines) {
				r = rightLines[i]
			} else {
				r = strings.Repeat(" ", rightW)
			}
			row += divider + r
		}
		rows = append(rows, row)
	}

	// --- Log panel ---
	sep := logSepStyle.Render(strings.Repeat("─", m.width))

	start := len(m.logLines) - logHeight
	if start < 0 {
		start = 0
	}
	tail := m.logLines[start:]
	logRows := make([]string, logHeight)
	for i := range logRows {
		logRows[i] = strings.Repeat(" ", m.width)
	}
	for i, line := range tail {
		if i >= logHeight {
			break
		}
		if len(line) > m.width {
			line = line[:m.width]
		}
		logRows[i] = logLineStyle.Render(line) + strings.Repeat(" ", max(0, m.width-len(line)))
	}

	// --- Status bar ---
	help := " q quit  r reset  p pause  i info  ↑↓/jk select  space inject"
	if m.lastStatus != "" {
		help += "  │  " + m.lastStatus
	}
	statusLine := helpStyle.Width(m.width).Render(help)

	return titleLine + "\n" +
		strings.Join(rows, "\n") + "\n" +
		sep + "\n" +
		strings.Join(logRows, "\n") + "\n" +
		statusLine
}

func (m Model) renderLeftRow(idx int, st endpointState, width int) string {
	cfg := st.cfg
	unit := cfg.Unit
	sched := strings.ToUpper(cfg.Scheduler)
	if sched == "PRIORITY" {
		sched = "PRIOR"
	}

	// Use tree label if available, otherwise full path.
	pathLabel := cfg.Path
	indent := ""
	if idx < len(m.treeLabels) {
		te := m.treeLabels[idx]
		pathLabel = te.label
		if te.depth > 0 {
			indent = strings.Repeat("  ", te.depth-1) + treeConnectorStyle.Render("└ ")
		}
	}

	label := fmt.Sprintf("%s  %s %.0f%s", pathLabel, sched, cfg.Rate, unit)

	var cursor string
	var rowStyle lipgloss.Style
	if idx == m.selected {
		cursor = cursorStyle.Render("▶")
		rowStyle = selectedRowStyle
	} else {
		cursor = " "
		if st.dynamic {
			rowStyle = dynamicRowStyle
		} else {
			rowStyle = configuredRowStyle
		}
	}

	rendered := " " + indent + rowStyle.Render(label)
	return cursor + lipgloss.NewStyle().Width(width - 1).Render(rendered)
}

func (m Model) renderMidRow(st endpointState, width int) string {
	maxQ := st.cfg.MaxQueueSize
	queued := len(st.enqueuedAt)

	// Right-aligned counter: " [N/M]"
	counterText := fmt.Sprintf("[%d/%d]", queued, maxQ)
	counter := counterStyle.Render(counterText)
	counterLen := utf8.RuneCountInString(counterText)

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

func (m Model) renderRightColumn(width, height int) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}

	if len(m.endpoints) == 0 || m.selected >= len(m.endpoints) {
		return lines
	}

	st := m.endpoints[m.selected]
	p50, p90, p99 := computePercentiles(st.waitSamples)
	sumSec := fmt.Sprintf("%.1fs", float64(st.totalWaitMs)/1000)

	stats := []struct{ label, value string }{
		{"served:  ", fmt.Sprintf("%d", st.served)},
		{"rejected:", fmt.Sprintf("%d", st.rejected)},
		{"p50:     ", fmt.Sprintf("%dms", p50)},
		{"p90:     ", fmt.Sprintf("%dms", p90)},
		{"p99:     ", fmt.Sprintf("%dms", p99)},
		{"last:    ", fmt.Sprintf("%dms", st.lastWaitMs)},
		{"sum:     ", sumSec},
	}

	// Stats pinned to top of column.
	for i, s := range stats {
		if i >= height {
			break
		}
		label := statLabelStyle.Render(s.label)
		value := statValueStyle.Render(s.value)
		line := " " + label + " " + value
		visLen := utf8.RuneCountInString(s.label) + utf8.RuneCountInString(s.value) + 3
		if visLen < width {
			line += strings.Repeat(" ", width-visLen)
		}
		lines[i] = line
	}

	return lines
}

// computePercentiles returns the p50, p90, and p99 of samples (in ms).
// Returns 0,0,0 for empty slices.
func computePercentiles(samples []int64) (p50, p90, p99 int64) {
	n := len(samples)
	if n == 0 {
		return 0, 0, 0
	}
	cp := make([]int64, n)
	copy(cp, samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	p50 = cp[(n-1)*50/100]
	p90 = cp[(n-1)*90/100]
	p99 = cp[(n-1)*99/100]
	return p50, p90, p99
}

// appendSample appends v to samples, capping at 200 entries.
func appendSample(samples []int64, v int64) []int64 {
	samples = append(samples, v)
	if len(samples) > 200 {
		samples = samples[len(samples)-200:]
	}
	return samples
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
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
