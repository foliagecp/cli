package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Messages ──────────────────────────────────────────────────────────────────

// vertexLoadedMsg is kept for tests: delivers a fully-loaded vertex in one shot.
type vertexLoadedMsg struct {
	id         string
	fvi        *fullVertexInfo
	links      []displayLink
	err        error
	partialErr error
}

// vertexInfoMsg is sent after the vertex body is fetched (phase 1 of loading).
type vertexInfoMsg struct {
	id  string
	gen int
	fvi *fullVertexInfo
	err error
}

// linksLoadedMsg is sent after all link details are fetched (phase 2 of loading).
type linksLoadedMsg struct {
	id         string
	gen        int
	links      []displayLink
	partialErr error
}

type queryResultMsg struct {
	results []string
	err     error
}

// ── displayLink ───────────────────────────────────────────────────────────────

type displayLink struct {
	info  fullLinkInfo
	isOut bool
}

func (dl displayLink) target() string {
	if dl.isOut {
		return dl.info.to
	}
	return dl.info.id.from
}

func (dl displayLink) label() string {
	return dl.info.id.name
}

// ── Model ─────────────────────────────────────────────────────────────────────

type tuiModel struct {
	currentID string
	fvi       *fullVertexInfo
	links     []displayLink

	cursor  int
	linkOff int // scroll offset for link list
	bodyVP  viewport.Model
	ready   bool

	loading    bool
	linksTotal int // known after phase-1; drives "loading N links…" display
	loadGen    int // incremented on each navigation to discard stale messages
	errMsg     string

	history []string

	queryMode    bool
	queryInput   textinput.Model
	queryResult  string   // status-bar message for errors / empty
	queryResults []string // non-nil when showing results in right panel
	qCursor      int
	qOffset      int

	showAll bool // mirrors inspect -a: show link details in body panel

	width  int
	height int
}

func newTuiModel(startID string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "JPGQL expression…"
	ti.CharLimit = 512
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	return tuiModel{
		currentID:  startID,
		loading:    true,
		queryInput: ti,
	}
}

func gWalkTUI() error {
	if err := gWalkLoad(); err != nil {
		return err
	}
	startID := gWalkData.GetByPath("id").AsStringDefault("")
	if startID == "" {
		startID = "root"
		_ = gWalkTo(startID) // persist default so one-off commands agree
	}
	p := tea.NewProgram(newTuiModel(startID), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ── Commands ──────────────────────────────────────────────────────────────────

// fetchVertexCmd loads vertex body only (phase 1). UI renders immediately after.
func fetchVertexCmd(id string, gen int) tea.Cmd {
	return func() tea.Msg {
		fvi, err := getVertexFullInfo(id)
		if err != nil {
			return vertexInfoMsg{id: id, gen: gen, err: err}
		}
		return vertexInfoMsg{id: id, gen: gen, fvi: &fvi}
	}
}

// fetchLinksCmd loads all link details (phase 2). Knows total count from fvi.
func fetchLinksCmd(id string, gen int, fvi *fullVertexInfo) tea.Cmd {
	return func() tea.Msg {
		links := make([]displayLink, 0, len(fvi.outLinks)+len(fvi.inLinks))
		var failed int
		for _, lid := range fvi.outLinks {
			if fli, err := getLinkFullInfo(lid); err == nil {
				links = append(links, displayLink{info: fli, isOut: true})
			} else {
				failed++
			}
		}
		for _, lid := range fvi.inLinks {
			if fli, err := getLinkFullInfo(lid); err == nil {
				links = append(links, displayLink{info: fli, isOut: false})
			} else {
				failed++
			}
		}
		sort.Slice(links, func(i, j int) bool {
			return links[i].info.id.name < links[j].info.id.name
		})
		var partialErr error
		if failed > 0 {
			partialErr = fmt.Errorf("%d link(s) failed to load", failed)
		}
		return linksLoadedMsg{id: id, gen: gen, links: links, partialErr: partialErr}
	}
}

func runQueryCmd(fromID, query string) tea.Cmd {
	return func() tea.Msg {
		if err := initDBClient(); err != nil {
			return queryResultMsg{err: err}
		}
		result, err := dbClient.Query.JPGQLCtraQuery(fromID, query)
		if err != nil {
			return queryResultMsg{err: err}
		}
		return queryResultMsg{results: result}
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m tuiModel) Init() tea.Cmd {
	return fetchVertexCmd(m.currentID, m.loadGen)
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	colorAccent   = lipgloss.Color("99")
	colorOut      = lipgloss.Color("42")
	colorIn       = lipgloss.Color("214")
	colorDim      = lipgloss.Color("240")
	colorSelected = lipgloss.Color("57")
	colorErr      = lipgloss.Color("196")
	colorLoading  = lipgloss.Color("220")
	colorHeader   = lipgloss.Color("205")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			MarginBottom(0)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(lipgloss.Color("255"))

	styleOut     = lipgloss.NewStyle().Foreground(colorOut)
	styleIn      = lipgloss.NewStyle().Foreground(colorIn)
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleMetaKey = lipgloss.NewStyle().Foreground(colorAccent)
	styleMetaVal = lipgloss.NewStyle().Foreground(lipgloss.Color("253"))

	styleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("253")).
			Padding(0, 1)

	styleHintKey = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	styleHintSep = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleErr     = lipgloss.NewStyle().Foreground(colorErr)
	styleLoading = lipgloss.NewStyle().Foreground(colorLoading)
)

// ── Dimensions ────────────────────────────────────────────────────────────────

const narrowThreshold = 60

func (m tuiModel) isNarrow() bool { return m.width < narrowThreshold }

func (m tuiModel) bodyOuterW() int {
	if m.width < 2 {
		return 1
	}
	w := m.width * 2 / 5
	if w < 20 {
		w = 20
	}
	maxW := m.width - 20
	if maxW < 1 {
		maxW = 1
	}
	if w > maxW {
		w = maxW
	}
	return w
}

func (m tuiModel) linksOuterW() int {
	w := m.width - m.bodyOuterW()
	if w < 1 {
		w = 1
	}
	return w
}

// Total height available for panel content (inside borders).
// Layout: header(1) + panels + status(1)
func (m tuiModel) panelContentH() int {
	h := m.height - 1 - 1 - 2 // -header -status -top/bottom borders
	if h < 1 {
		h = 1
	}
	return h
}

func (m tuiModel) vpWidth() int {
	w := m.bodyOuterW() - 4 // border(2) + padding(2)
	if w < 1 {
		w = 1
	}
	return w
}

func (m tuiModel) vpHeight() int {
	h := m.panelContentH()
	if h < 1 {
		h = 1
	}
	return h
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.bodyVP = viewport.New(m.vpWidth(), m.vpHeight())
			m.bodyVP.SetContent(m.bodyContent())
			m.ready = true
		} else {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
		}
		return m, nil

	// vertexLoadedMsg: test shortcut — delivers fully-loaded state in one shot.
	case vertexLoadedMsg:
		m.loading = false
		m.linksTotal = 0
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		if msg.partialErr != nil {
			m.errMsg = msg.partialErr.Error()
		}
		m.currentID = msg.id
		m.fvi = msg.fvi
		m.links = msg.links
		m.showAll = false
		if m.cursor >= len(m.links) {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
			m.bodyVP.SetContent(m.bodyContent())
			m.bodyVP.GotoTop()
		}
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, nil

	// vertexInfoMsg: phase 1 complete — body is ready, start loading links.
	case vertexInfoMsg:
		if msg.gen != m.loadGen {
			return m, nil // stale, navigation changed
		}
		if msg.err != nil {
			m.loading = false
			m.linksTotal = 0
			m.errMsg = msg.err.Error()
			if m.ready {
				m.bodyVP.SetContent(m.bodyContent())
			}
			return m, nil
		}
		m.errMsg = ""
		m.currentID = msg.id
		m.fvi = msg.fvi
		m.links = nil
		m.showAll = false
		m.linksTotal = len(msg.fvi.outLinks) + len(msg.fvi.inLinks)
		if m.cursor >= m.linksTotal {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
			m.bodyVP.SetContent(m.bodyContent())
			m.bodyVP.GotoTop()
		}
		if m.linksTotal == 0 {
			m.loading = false
			if err := gWalkTo(msg.id); err != nil {
				m.errMsg = "persist failed: " + err.Error()
			}
			return m, nil
		}
		return m, fetchLinksCmd(msg.id, msg.gen, msg.fvi)

	// linksLoadedMsg: phase 2 complete — all links fetched.
	case linksLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil // stale
		}
		m.loading = false
		m.linksTotal = 0
		m.links = msg.links
		if msg.partialErr != nil {
			m.errMsg = msg.partialErr.Error()
		}
		if m.cursor >= len(m.links) {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.SetContent(m.bodyContent())
		}
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, nil

	case queryResultMsg:
		m.queryMode = false
		m.queryInput.Blur()
		m.queryInput.SetValue("")
		m.queryResults = nil
		if msg.err != nil {
			m.queryResult = styleErr.Render("error: " + msg.err.Error())
		} else if len(msg.results) == 0 {
			m.queryResult = styleDim.Render("(no results)")
		} else {
			m.queryResult = ""
			m.queryResults = msg.results
			m.qCursor = 0
			m.qOffset = 0
		}
		return m, nil
	}

	if m.queryMode {
		return m.updateQuery(msg)
	}
	return m.updateNav(msg)
}

func (m tuiModel) updateNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.queryResults) > 0 {
		return m.updateNavQueryResults(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "j", "down":
			if len(m.links) > 0 {
				m.cursor = (m.cursor + 1) % len(m.links)
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil
		case "k", "up":
			if len(m.links) > 0 {
				m.cursor = (m.cursor - 1 + len(m.links)) % len(m.links)
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil

		case "enter":
			if m.loading || len(m.links) == 0 {
				break
			}
			target := m.links[m.cursor].target()
			if target == "" {
				break
			}
			m.history = append(m.history, m.currentID)
			m.loading = true
			m.linksTotal = 0
			m.queryResult = ""
			m.loadGen++
			return m, fetchVertexCmd(target, m.loadGen)

		case "b", "backspace":
			if len(m.history) > 0 {
				prev := m.history[len(m.history)-1]
				m.history = m.history[:len(m.history)-1]
				m.loading = true
				m.linksTotal = 0
				m.queryResult = ""
				m.loadGen++
				return m, fetchVertexCmd(prev, m.loadGen)
			}

		case "/":
			m.queryMode = true
			m.queryResult = ""
			m.queryInput.Focus()
			return m, textinput.Blink

		case "a":
			m.showAll = !m.showAll
			if m.ready {
				m.bodyVP.SetContent(m.bodyContent())
				m.bodyVP.GotoTop()
			}

		case "r":
			m.loading = true
			m.linksTotal = 0
			m.queryResult = ""
			m.loadGen++
			return m, fetchVertexCmd(m.currentID, m.loadGen)

		case "g":
			m.bodyVP.HalfPageUp()
		case "G":
			m.bodyVP.HalfPageDown()
		}
	}

	var cmd tea.Cmd
	m.bodyVP, cmd = m.bodyVP.Update(msg)
	return m, cmd
}

func (m tuiModel) updateQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.queryMode = false
			m.queryInput.Blur()
			m.queryInput.SetValue("")
			return m, nil
		case "enter":
			q := strings.TrimSpace(m.queryInput.Value())
			m.queryInput.SetValue("")
			if q == "" {
				m.queryMode = false
				m.queryInput.Blur()
				return m, nil
			}
			m.queryMode = false
			m.queryInput.Blur()
			return m, runQueryCmd(m.currentID, q)
		}
	}
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(msg)
	return m, cmd
}

func (m tuiModel) updateNavQueryResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if len(m.queryResults) > 0 {
				m.qCursor = (m.qCursor + 1) % len(m.queryResults)
				m = m.clampQueryScroll()
			}
			return m, nil
		case "k", "up":
			if len(m.queryResults) > 0 {
				m.qCursor = (m.qCursor - 1 + len(m.queryResults)) % len(m.queryResults)
				m = m.clampQueryScroll()
			}
			return m, nil
		case "enter":
			target := m.queryResults[m.qCursor]
			m.queryResults = nil
			m.queryResult = ""
			m.history = append(m.history, m.currentID)
			m.loading = true
			m.linksTotal = 0
			m.loadGen++
			return m, fetchVertexCmd(target, m.loadGen)
		case "esc", "b", "backspace":
			m.queryResults = nil
			m.queryResult = ""
			return m, nil
		}
	}
	return m, nil
}

// clampScroll keeps cursor visible in the link list.
func (m tuiModel) clampScroll() tuiModel {
	visible := m.panelContentH() - 1 // -1 for section title
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.linkOff {
		m.linkOff = m.cursor
	}
	if m.cursor >= m.linkOff+visible {
		m.linkOff = m.cursor - visible + 1
	}
	return m
}

func (m tuiModel) clampQueryScroll() tuiModel {
	visible := m.panelContentH() - 1
	if visible < 1 {
		visible = 1
	}
	if m.qCursor < m.qOffset {
		m.qOffset = m.qCursor
	}
	if m.qCursor >= m.qOffset+visible {
		m.qOffset = m.qCursor - visible + 1
	}
	return m
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m tuiModel) bodyContent() string {
	if m.fvi == nil {
		if m.errMsg != "" {
			return styleErr.Render("⚠ " + m.errMsg)
		}
		return styleDim.Render("(loading…)")
	}

	var bodyStr string
	if m.fvi.body == nil || !m.fvi.body.IsNonEmptyObject() {
		bodyStr = styleDim.Render("(empty body)")
	} else {
		bodyStr = JSONStrPrettyString(m.fvi.body, 0, 2)
	}

	if !m.showAll || len(m.links) == 0 {
		return bodyStr
	}

	var sb strings.Builder
	sb.WriteString(bodyStr)

	outCount := m.outCount()
	for i, dl := range m.links {
		sb.WriteString("\n\n")
		if dl.isOut {
			sb.WriteString(styleOut.Render(fmt.Sprintf("→ [%d/%d] %s → %s", i+1, outCount, dl.label(), dl.target())))
		} else {
			sb.WriteString(styleIn.Render(fmt.Sprintf("← %s ← %s", dl.label(), dl.target())))
		}
		if dl.info.tp != "" {
			sb.WriteString("  " + styleMetaKey.Render("type:") + " " + styleMetaVal.Render(dl.info.tp))
		}
		if len(dl.info.tags) > 0 {
			sb.WriteString("  " + styleMetaKey.Render("tags:") + " " + styleMetaVal.Render(strings.Join(dl.info.tags, " ")))
		}
		if dl.info.body != nil && dl.info.body.IsNonEmptyObject() {
			sb.WriteString("\n  " + JSONStrPrettyString(dl.info.body, 2, 2))
		}
	}

	return sb.String()
}

// vertexKindBadge returns a styled badge for the current vertex:
//   - "[type]"          if it has a link to the "types" vertex
//   - "[<__type>]"      if it has a link to the "objects" vertex (shows its type)
//   - ""                otherwise
func (m tuiModel) vertexKindBadge() string {
	isObject, isType := false, false
	typeName := ""
	typesID := NatsHubDomain + "/types"
	objectsID := NatsHubDomain + "/objects"
	for _, dl := range m.links {
		target := dl.target()
		if target == typesID {
			isType = true
		}
		if target == objectsID {
			isObject = true
		}
		if dl.isOut && dl.info.tp == "__type" {
			typeName = target
		}
	}
	if isType {
		return " " + styleMetaKey.Render("[type]")
	}
	if isObject && typeName != "" {
		return " " + styleMetaKey.Render("["+typeName+"]")
	}
	return ""
}

func (m tuiModel) renderBodyPanel() string {
	innerW := m.bodyOuterW() - 2
	innerH := m.panelContentH()

	allMark := ""
	if m.showAll {
		allMark = styleDim.Render(" [+all]")
	}
	title := styleTitle.Render("◈ "+m.currentID) + m.vertexKindBadge() + allMark
	content := m.bodyVP.View()

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return stylePanel.
		Width(innerW).
		Height(innerH + 1). // +1 for title line
		Render(inner)
}

func (m tuiModel) renderLinksPanel() string {
	innerW := m.linksOuterW() - 2
	innerH := m.panelContentH()
	listH := innerH - 1 // -1 for title

	var title, content string

	if len(m.queryResults) > 0 {
		title = styleTitle.Render(fmt.Sprintf("Query results (%d)", len(m.queryResults)))
		content = m.renderQueryResults(innerW-2, listH)
	} else {
		title = styleTitle.Render(
			fmt.Sprintf("Links  %s %s",
				styleOut.Render(fmt.Sprintf("↓out:%d", m.outCount())),
				styleIn.Render(fmt.Sprintf("↑in:%d", m.inCount())),
			),
		)
		if m.loading {
			if m.linksTotal > 0 {
				content = styleLoading.Render(fmt.Sprintf("loading %d links…", m.linksTotal))
			} else {
				content = styleLoading.Render("loading…")
			}
		} else if len(m.links) == 0 {
			content = styleDim.Render("(no links)")
		} else {
			content = m.renderLinkList(innerW-2, listH)
		}
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return stylePanel.
		Width(innerW).
		Height(innerH + 1).
		Render(inner)
}

func (m tuiModel) renderQueryResults(w, h int) string {
	lines := make([]string, 0, h)
	for i := m.qOffset; i < len(m.queryResults) && len(lines) < h; i++ {
		id := m.queryResults[i]
		if i == m.qCursor {
			lines = append(lines, styleSelected.Width(w).Render("▶ "+id))
		} else {
			lines = append(lines, "  "+id)
		}
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderLinkList(w, h int) string {
	lines := make([]string, 0, h)

	for i := m.linkOff; i < len(m.links) && len(lines) < h; i++ {
		dl := m.links[i]

		var dir string
		if dl.isOut {
			dir = styleOut.Render("→")
		} else {
			dir = styleIn.Render("←")
		}

		name := dl.label()
		target := dl.target()
		tp := ""
		if dl.info.tp != "" {
			tp = styleDim.Render(" [" + dl.info.tp + "]")
		}

		// Truncate target if needed
		maxT := w - len(name) - 8
		if maxT < 6 {
			maxT = 6
		}
		if len(target) > maxT {
			target = target[:maxT-1] + "…"
		}

		if i == m.cursor {
			dirPlain := "→"
			if !dl.isOut {
				dirPlain = "←"
			}
			tpPlain := ""
			if dl.info.tp != "" {
				tpPlain = " [" + dl.info.tp + "]"
			}
			lines = append(lines, styleSelected.Width(w).Render(
				fmt.Sprintf("▶ %s %-16s %s%s", dirPlain, name, target, tpPlain),
			))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %-16s %s%s", dir, name, target, tp))
		}
	}

	return strings.Join(lines, "\n")
}

func (m tuiModel) outCount() int {
	if m.fvi == nil {
		return 0
	}
	return len(m.fvi.outLinks)
}

func (m tuiModel) inCount() int {
	if m.fvi == nil {
		return 0
	}
	return len(m.fvi.inLinks)
}

func (m tuiModel) renderHeader() string {
	back := ""
	if len(m.history) > 0 {
		back = styleDim.Render(" ← " + m.history[len(m.history)-1])
	}
	load := ""
	if m.loading {
		if m.linksTotal > 0 {
			load = " " + styleLoading.Render(fmt.Sprintf("loading %d links…", m.linksTotal))
		} else {
			load = " " + styleLoading.Render("loading…")
		}
	}
	errPart := ""
	if m.errMsg != "" {
		errPart = "  " + styleErr.Render("⚠ "+m.errMsg)
	}
	header := styleHeader.Render("◈ "+m.currentID) + back + load + errPart
	return lipgloss.NewStyle().Width(m.width).Render(header)
}

// hint renders a key:description pair for the status bar.
func hint(key, desc string) string {
	return styleHintKey.Render(key) + styleHintSep.Render(":"+desc)
}

func (m tuiModel) renderStatus() string {
	var s string
	switch {
	case m.queryMode:
		s = "Query: " + m.queryInput.View() + styleHintSep.Render("  Esc:cancel")
	case len(m.queryResults) > 0:
		sep := styleHintSep.Render("  ")
		s = strings.Join([]string{
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("Esc/b", "close"),
			hint("q", "quit"),
		}, sep)
	case m.queryResult != "":
		s = "↳ " + m.queryResult
	case m.errMsg != "":
		s = hint("r", "retry") + "  " + hint("q", "quit")
	default:
		sep := styleHintSep.Render("  ")
		s = strings.Join([]string{
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("b", "back"),
			hint("a", "all"),
			hint("/", "query"),
			hint("r", "refresh"),
			hint("g/G", "body ↑↓ half"),
			hint("q", "quit"),
		}, sep)
	}
	return styleStatus.Width(m.width).Render(s)
}

// viewNarrow renders a compact single-column layout for terminals narrower than narrowThreshold.
func (m tuiModel) viewNarrow() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	listH := m.height - 3 // header + status + divider
	if listH < 1 {
		listH = 1
	}

	divider := styleDim.Render(strings.Repeat("─", w))

	var links string
	if m.loading {
		links = styleLoading.Render("loading…")
	} else if len(m.links) == 0 {
		links = styleDim.Render("(no links)")
	} else {
		links = m.renderLinkList(w, listH)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		divider,
		links,
		m.renderStatus(),
	)
}

func (m tuiModel) View() string {
	if !m.ready || m.width == 0 {
		return "Starting…"
	}

	if m.isNarrow() {
		return m.viewNarrow()
	}

	panels := lipgloss.JoinHorizontal(lipgloss.Top, m.renderBodyPanel(), m.renderLinksPanel())
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), panels, m.renderStatus())
}
