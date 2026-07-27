package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Export constants ──────────────────────────────────────────────────────────

type exportFmt struct {
	label string
	value string
}

var (
	exportFmts = []exportFmt{
		{"graphml", "graphml"},
		{"dot", "dot"},
		{"json2xml", "graphml_json2xml"},
	}
	exportDepthPresets = []string{"", "1", "2", "3", "5", "10"} // "" means -1 (all)
)

// ── Messages ──────────────────────────────────────────────────────────────────

type vertexLoadedMsg struct {
	id         string
	gen        int
	fvi        *fullVertexInfo
	links      []displayLink
	err        error
	partialErr error
}

type cachedVertex struct {
	fvi   *fullVertexInfo
	links []displayLink
}

type vertexInfoMsg struct {
	id  string
	gen int
	fvi *fullVertexInfo
	err error
}

type linksLoadedMsg struct {
	id         string
	gen        int
	links      []displayLink
	partialErr error
}

type queryResultMsg struct {
	gen     int
	results []string
	err     error
}

type depth2TypesMsg struct {
	gen      int
	outTypes []string
	inTypes  []string
}

type exportResultMsg struct {
	gen  int
	file string
	err  error
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

// ── Grouped view ──────────────────────────────────────────────────────────────

type flatItemKind int

const (
	flatTypeGroup flatItemKind = iota // type-group header — Tab collapses
	flatLink                          // individual link row — Enter navigates
)

type flatItem struct {
	kind     flatItemKind
	groupIdx int // index in the corresponding groups slice
	linkIdx  int // flatLink only: index within group.links
}

type linkGroup struct {
	tp        string
	links     []displayLink
	collapsed bool
}

type groupedView struct {
	outGroups []linkGroup
	inGroups  []linkGroup
	outFlat   []flatItem // navigable rows for the right (outgoing) panel
	inFlat    []flatItem // navigable rows for the left (incoming) panel
}

func buildGroupedView(links []displayLink, searchQuery string) groupedView {
	outByType := map[string][]displayLink{}
	inByType := map[string][]displayLink{}
	for _, dl := range links {
		tp := dl.info.tp
		if tp == "" {
			tp = "(no type)"
		}
		if dl.isOut {
			outByType[tp] = append(outByType[tp], dl)
		} else {
			inByType[tp] = append(inByType[tp], dl)
		}
	}

	buildGroups := func(byType map[string][]displayLink) []linkGroup {
		types := make([]string, 0, len(byType))
		for k := range byType {
			types = append(types, k)
		}
		sort.Strings(types)

		groups := make([]linkGroup, 0, len(types))
		for _, tp := range types {
			ls := byType[tp]
			sort.Slice(ls, func(i, j int) bool {
				return ls[i].info.id.name < ls[j].info.id.name
			})
			if searchQuery != "" {
				q := strings.ToLower(searchQuery)
				filtered := ls[:0:0]
				for _, dl := range ls {
					if strings.Contains(strings.ToLower(dl.label()), q) ||
						strings.Contains(strings.ToLower(dl.target()), q) {
						filtered = append(filtered, dl)
					}
				}
				ls = filtered
			}
			if len(ls) == 0 {
				continue
			}
			groups = append(groups, linkGroup{tp: tp, links: ls})
		}
		return groups
	}

	gv := groupedView{
		outGroups: buildGroups(outByType),
		inGroups:  buildGroups(inByType),
	}
	gv.outFlat = buildPanelFlat(gv.outGroups)
	gv.inFlat = buildPanelFlat(gv.inGroups)
	return gv
}

// buildPanelFlat builds the navigable flat list for a single panel's group list.
func buildPanelFlat(groups []linkGroup) []flatItem {
	flat := make([]flatItem, 0, len(groups)*4)
	for gi, g := range groups {
		flat = append(flat, flatItem{kind: flatTypeGroup, groupIdx: gi})
		if !g.collapsed {
			for li := range g.links {
				flat = append(flat, flatItem{kind: flatLink, groupIdx: gi, linkIdx: li})
			}
		}
	}
	return flat
}

func (gv *groupedView) rebuildFlat() {
	gv.outFlat = buildPanelFlat(gv.outGroups)
	gv.inFlat = buildPanelFlat(gv.inGroups)
}

// linkForItemIn returns the displayLink for a flatLink item within groups.
func linkForItemIn(groups []linkGroup, item flatItem) (displayLink, bool) {
	if item.kind != flatLink || item.groupIdx >= len(groups) {
		return displayLink{}, false
	}
	g := groups[item.groupIdx]
	if item.linkIdx >= len(g.links) {
		return displayLink{}, false
	}
	return g.links[item.linkIdx], true
}

// nextSelectable advances the cursor by dir (+1/-1), wrapping around the list.
func nextSelectable(flat []flatItem, from, dir int) int {
	n := len(flat)
	if n == 0 {
		return 0
	}
	return (from + dir + n) % n
}

// firstSelectableIdx returns 0; all items in a panel flat list are selectable.
func firstSelectableIdx(flat []flatItem) int {
	_ = flat
	return 0
}

// ── Panel focus ────────────────────────────────────────────────────────────────

type panelFocus int

const (
	panelOut panelFocus = iota // right panel — outgoing links (default)
	panelIn                    // left panel — incoming links
)

// ── Model ─────────────────────────────────────────────────────────────────────

type tuiModel struct {
	currentID string
	fvi       *fullVertexInfo
	links     []displayLink
	grouped   groupedView

	focus   panelFocus
	rCursor int // cursor in grouped.outFlat (right/outgoing panel)
	lCursor int // cursor in grouped.inFlat  (left/incoming panel)
	rOffset int // scroll offset in right panel
	lOffset int // scroll offset in left panel

	bodyVP viewport.Model
	ready  bool

	loading    bool
	linksTotal int
	loadGen    int
	errMsg     string

	history []string

	queryMode    bool
	queryInput   textinput.Model
	queryResult  string
	queryResults []string
	qCursor      int
	qOffset      int

	searchMode  bool
	searchInput textinput.Model
	searchQuery string

	exportMode     bool
	exportDepStep  bool // false = format selection, true = depth entry
	exportFmtIdx   int
	exportDepthIdx int // index in exportDepthPresets
	exportInput    textinput.Model

	cache map[string]cachedVertex

	outTypes2 []string
	inTypes2  []string

	rawBody bool

	// form holds the active CRUD form, or nil. It is checked FIRST in the
	// dispatch chain, so "a form is modal" is the semantics rather than a
	// convention. Keeping it a single nullable field leaves every existing
	// mode and every existing test untouched.
	form *formState

	// llMode forces the low-level API even on typed vertices — the escape
	// hatch for inspecting or repairing graph state that the high-level API
	// would refuse to express.
	llMode bool

	// restore carries view state across a reload; see tui_restore.go.
	restore *viewRestore

	// pendingNavAfterDelete is where to go once the in-flight delete of the
	// current vertex succeeds. Decided before the delete, while the history is
	// still intact.
	pendingNavAfterDelete string

	// anchor marks a vertex as the source for the next link; see tui_flows.go.
	anchor *anchorState

	// bodyRegister holds a yanked body, offered as a template when creating
	// or editing another entity. Navigating to a sibling, pressing y, and
	// coming back is how "copy the body of an existing object" works without
	// any extra API surface.
	bodyRegister string

	// helpOpen shows the full keymap. Checked before everything else, since
	// the status bar can only advertise a handful of the bindings.
	helpOpen bool

	width  int
	height int
}

// ── Active-panel helpers ───────────────────────────────────────────────────────

func (m tuiModel) activeFlat() []flatItem {
	if m.focus == panelIn {
		return m.grouped.inFlat
	}
	return m.grouped.outFlat
}

func (m tuiModel) activeCursorVal() int {
	if m.focus == panelIn {
		return m.lCursor
	}
	return m.rCursor
}

func (m tuiModel) activeOffsetVal() int {
	if m.focus == panelIn {
		return m.lOffset
	}
	return m.rOffset
}

func (m tuiModel) activeGroups() []linkGroup {
	if m.focus == panelIn {
		return m.grouped.inGroups
	}
	return m.grouped.outGroups
}

func (m tuiModel) setActiveCursor(v int) tuiModel {
	if m.focus == panelIn {
		m.lCursor = v
	} else {
		m.rCursor = v
	}
	return m
}

func (m tuiModel) setActiveOffset(v int) tuiModel {
	if m.focus == panelIn {
		m.lOffset = v
	} else {
		m.rOffset = v
	}
	return m
}

// refreshBody re-syncs the body viewport size and content. No-op if not ready.
func (m tuiModel) refreshBody() tuiModel {
	if !m.ready {
		return m
	}
	m.bodyVP.Width = m.vpWidth()
	m.bodyVP.Height = m.vpHeight()
	m.bodyVP.SetContent(m.bodyContent())
	return m
}

// ── Vertex classification ─────────────────────────────────────────────────────

type vertexKind int

const (
	vkPlain  vertexKind = iota // a bare graph vertex — only the low-level API applies
	vkType                     // a CMDB type: linked from the built-in `types` root
	vkObject                   // a CMDB object: linked from `objects`, with a __type out-link
)

// vertexKind classifies the current vertex from its links, and for an object
// also returns its type name.
//
// The precedence matters and is not arbitrary: a CMDB types-link is stored as a
// `__type`-typed edge — the SAME link type an object uses for its instance-of
// edge — so a `__type` out-link alone cannot tell a type vertex from an object.
// Membership in the `types` topology is the discriminator and must be checked
// first. This is the ordering vertexKindBadge has always relied on; extracting
// it here keeps the CRUD flows from re-deriving it (and getting it wrong).
func (m tuiModel) vertexKind() (vertexKind, string) {
	typesID := NatsHubDomain + "/types"
	objectsID := NatsHubDomain + "/objects"
	isObject, isType := false, false
	typeName := ""
	for _, dl := range m.links {
		target := dl.target()
		if target == typesID {
			isType = true
		}
		if target == objectsID {
			isObject = true
		}
		if dl.isOut && dl.info.tp == "__type" {
			typeName = stripDomain(target)
		}
	}
	if isType {
		return vkType, ""
	}
	// An object whose __type link is missing (a half-written vertex) stays
	// vkPlain: we genuinely cannot name its type, and claiming otherwise would
	// send HL calls that the server will reject.
	if isObject && typeName != "" {
		return vkObject, typeName
	}
	return vkPlain, ""
}

// cursorLink returns the displayLink at the active cursor (only for flatLink items).
func (m tuiModel) cursorLink() (displayLink, bool) {
	flat := m.activeFlat()
	cursor := m.activeCursorVal()
	if cursor >= len(flat) {
		return displayLink{}, false
	}
	return linkForItemIn(m.activeGroups(), flat[cursor])
}

// ── Constructor ───────────────────────────────────────────────────────────────

func newTuiModel(startID string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "JPGQL expression…"
	ti.CharLimit = 512
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	si := textinput.New()
	si.Placeholder = "search…"
	si.CharLimit = 128
	si.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	si.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	ei := textinput.New()
	ei.Placeholder = "all"
	ei.CharLimit = 6
	ei.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	ei.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	return tuiModel{
		currentID:   startID,
		loading:     true,
		queryInput:  ti,
		searchInput: si,
		exportInput: ei,
		cache:       make(map[string]cachedVertex),
		focus:       panelOut,
	}
}

func gWalkTUI() error {
	if err := gWalkLoad(); err != nil {
		return err
	}
	startID := gWalkData.GetByPath("id").AsStringDefault("")
	if startID == "" {
		startID = "root"
		_ = gWalkTo(startID)
	}
	p := tea.NewProgram(newTuiModel(startID), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ── Commands ──────────────────────────────────────────────────────────────────

func fetchVertexCmd(id string, gen int) tea.Cmd {
	return func() tea.Msg {
		if err := initDBClient(); err != nil {
			return vertexInfoMsg{id: id, gen: gen, err: err}
		}
		fvi, err := getVertexFullInfo(id)
		if err != nil {
			return vertexInfoMsg{id: id, gen: gen, err: err}
		}
		return vertexInfoMsg{id: id, gen: gen, fvi: &fvi}
	}
}

func cacheHitCmd(id string, gen int, cv cachedVertex) tea.Cmd {
	return func() tea.Msg {
		return vertexLoadedMsg{id: id, gen: gen, fvi: cv.fvi, links: cv.links}
	}
}

func fetchLinksCmd(id string, gen int, fvi *fullVertexInfo) tea.Cmd {
	return func() tea.Msg {
		// Fast path: the vertex read already carried every link's target and
		// type, so there is nothing left to fetch. This is the difference
		// between one request and one-per-link — and every mutation triggers a
		// refresh, so on a type vertex with thousands of instances the fan-out
		// would dominate. Link bodies and tags are not needed here (the list
		// view never shows them) and are read lazily when an editor opens.
		if len(fvi.outFull)+len(fvi.inFull) == len(fvi.outLinks)+len(fvi.inLinks) {
			links := make([]displayLink, 0, len(fvi.outFull)+len(fvi.inFull))
			for _, fli := range fvi.outFull {
				links = append(links, displayLink{info: fli, isOut: true})
			}
			for _, fli := range fvi.inFull {
				links = append(links, displayLink{info: fli, isOut: false})
			}
			return linksLoadedMsg{id: id, gen: gen, links: links}
		}

		type result struct {
			dl  displayLink
			err error
		}

		type linkTask struct {
			lid   linkId
			isOut bool
		}
		all := make([]linkTask, 0, len(fvi.outLinks)+len(fvi.inLinks))
		for _, lid := range fvi.outLinks {
			all = append(all, linkTask{lid, true})
		}
		for _, lid := range fvi.inLinks {
			all = append(all, linkTask{lid, false})
		}

		results := make([]result, len(all))
		workers := runtime.NumCPU() * 3
		if workers < 4 {
			workers = 4
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, task := range all {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, lid linkId, isOut bool) {
				defer wg.Done()
				defer func() { <-sem }()
				fli, err := getLinkFullInfo(lid)
				if err != nil {
					results[i] = result{err: err}
				} else {
					results[i] = result{dl: displayLink{info: fli, isOut: isOut}}
				}
			}(i, task.lid, task.isOut)
		}
		wg.Wait()

		links := make([]displayLink, 0, len(all))
		var failed int
		for _, r := range results {
			if r.err != nil {
				failed++
			} else {
				links = append(links, r.dl)
			}
		}
		var partialErr error
		if failed > 0 {
			partialErr = fmt.Errorf("%d link(s) failed to load", failed)
		}
		return linksLoadedMsg{id: id, gen: gen, links: links, partialErr: partialErr}
	}
}

// fetchDepth2TypesCmd fetches link types one hop beyond the current vertex's direct neighbours.
// For each level-1 link type, it picks one representative neighbour, fetches that vertex's
// link list, then fetches types for those links — all in parallel.
func fetchDepth2TypesCmd(gen int, links []displayLink) tea.Cmd {
	return func() tea.Msg {
		if len(links) == 0 {
			return depth2TypesMsg{gen: gen}
		}
		if err := initDBClient(); err != nil {
			return depth2TypesMsg{gen: gen}
		}

		// One representative neighbour per (direction × level-1 type).
		outReps := map[string]string{}
		inReps := map[string]string{}
		for _, dl := range links {
			target := dl.target()
			if target == "" {
				continue
			}
			tp := dl.info.tp
			if tp == "" {
				tp = "(no type)"
			}
			if dl.isOut {
				if _, ok := outReps[tp]; !ok {
					outReps[tp] = target
				}
			} else {
				if _, ok := inReps[tp]; !ok {
					inReps[tp] = target
				}
			}
		}

		type neighbor struct {
			id    string
			isOut bool
		}
		neighbors := make([]neighbor, 0, len(outReps)+len(inReps))
		for _, id := range outReps {
			neighbors = append(neighbors, neighbor{id, true})
		}
		for _, id := range inReps {
			neighbors = append(neighbors, neighbor{id, false})
		}

		// Fetch vertex info for each representative in parallel.
		type vtxResult struct {
			isOut bool
			fvi   fullVertexInfo
			ok    bool
		}
		vtxResults := make([]vtxResult, len(neighbors))
		workers := runtime.NumCPU() * 3
		if workers < 4 {
			workers = 4
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, nb := range neighbors {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, id string, isOut bool) {
				defer wg.Done()
				defer func() { <-sem }()
				fvi, err := getVertexFullInfo(id)
				vtxResults[i] = vtxResult{isOut: isOut, fvi: fvi, ok: err == nil}
			}(i, nb.id, nb.isOut)
		}
		wg.Wait()

		// Collect link IDs from all representative vertices (hard cap).
		type linkTask struct {
			lid   linkId
			isOut bool
		}
		const maxTasks = 200
		var tasks []linkTask
		for _, vr := range vtxResults {
			if !vr.ok {
				continue
			}
			if vr.isOut {
				// outgoing rep: only its outLinks reach depth-2 outgoing territory
				for _, lid := range vr.fvi.outLinks {
					tasks = append(tasks, linkTask{lid, true})
					if len(tasks) >= maxTasks {
						break
					}
				}
			} else {
				// incoming rep: only its inLinks reach depth-2 incoming territory
				for _, lid := range vr.fvi.inLinks {
					tasks = append(tasks, linkTask{lid, false})
					if len(tasks) >= maxTasks {
						break
					}
				}
			}
			if len(tasks) >= maxTasks {
				break
			}
		}
		if len(tasks) == 0 {
			return depth2TypesMsg{gen: gen}
		}

		// Fetch link types in parallel.
		type typeResult struct {
			tp    string
			isOut bool
		}
		typeResults := make([]typeResult, len(tasks))
		for i, task := range tasks {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, lid linkId, isOut bool) {
				defer wg.Done()
				defer func() { <-sem }()
				fli, err := getLinkFullInfo(lid)
				if err != nil {
					return
				}
				tp := fli.tp
				if tp == "" {
					tp = "(no type)"
				}
				typeResults[i] = typeResult{tp: tp, isOut: isOut}
			}(i, task.lid, task.isOut)
		}
		wg.Wait()

		outSet := map[string]bool{}
		inSet := map[string]bool{}
		for _, tr := range typeResults {
			if tr.tp == "" {
				continue
			}
			if tr.isOut {
				outSet[tr.tp] = true
			} else {
				inSet[tr.tp] = true
			}
		}

		out2 := make([]string, 0, len(outSet))
		for tp := range outSet {
			out2 = append(out2, tp)
		}
		sort.Strings(out2)

		in2 := make([]string, 0, len(inSet))
		for tp := range inSet {
			in2 = append(in2, tp)
		}
		sort.Strings(in2)

		return depth2TypesMsg{gen: gen, outTypes: out2, inTypes: in2}
	}
}

func runExportCmd(gen int, fromID, format string, depth int) tea.Cmd {
	return func() tea.Msg {
		if err := initDBClient(); err != nil {
			return exportResultMsg{gen: gen, err: err}
		}
		data, err := gWalkGetGraph(format, fromID, depth, nil, nil)
		if err != nil {
			return exportResultMsg{gen: gen, err: err}
		}
		ext := "graphml"
		if format == "dot" {
			ext = "dot"
		}
		safe := strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(fromID)
		filename := safe + "." + ext
		if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
			return exportResultMsg{gen: gen, err: err}
		}
		return exportResultMsg{gen: gen, file: filename}
	}
}

func runQueryCmd(gen int, fromID, query string) tea.Cmd {
	return func() tea.Msg {
		if err := initDBClient(); err != nil {
			return queryResultMsg{gen: gen, err: err}
		}
		result, err := dbClient.Query.JPGQLCtraQuery(fromID, query)
		if err != nil {
			return queryResultMsg{gen: gen, err: err}
		}
		return queryResultMsg{gen: gen, results: result}
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
	colorType     = lipgloss.Color("75")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	stylePanelActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(lipgloss.Color("255"))

	styleOut     = lipgloss.NewStyle().Foreground(colorOut)
	styleIn      = lipgloss.NewStyle().Foreground(colorIn)
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleMetaKey = lipgloss.NewStyle().Foreground(colorAccent)
	styleMetaVal = lipgloss.NewStyle().Foreground(lipgloss.Color("253"))
	styleTypeHdr = lipgloss.NewStyle().Foreground(colorType)

	styleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("253")).
			Padding(0, 1)

	styleHintKey = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	styleHintSep = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleErr     = lipgloss.NewStyle().Foreground(colorErr)
	styleLoading = lipgloss.NewStyle().Foreground(colorLoading)

	// CRUD feedback: applied vs. a mode that destroys data if misread.
	styleOk     = lipgloss.NewStyle().Foreground(colorOut)
	styleWarn   = lipgloss.NewStyle().Bold(true).Foreground(colorIn)
	styleSearch = lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("16"))
)

// ── Dimensions ────────────────────────────────────────────────────────────────

const narrowThreshold = 90

func (m tuiModel) isNarrow() bool { return m.width < narrowThreshold }

// sideW is the total outer width (including border) of each side panel.
func (m tuiModel) sideW() int {
	w := m.width / 3 // ~33% each side
	if w < 28 {
		w = 28
	}
	if w > 52 {
		w = 52
	}
	return w
}

// centerW is the total outer width of the center panel.
func (m tuiModel) centerW() int {
	w := m.width - 2*m.sideW()
	if w < 10 {
		w = 10
	}
	return w
}

// sideContentW is the inner content width for a side panel (border adds 2).
func (m tuiModel) sideContentW() int {
	w := m.sideW() - 2
	if w < 1 {
		w = 1
	}
	return w
}

// centerContentW is the inner content width for the center panel.
func (m tuiModel) centerContentW() int {
	w := m.centerW() - 2
	if w < 1 {
		w = 1
	}
	return w
}

// breadcrumbH also reserves the row for the anchor marker, which lives there
// even with no history.
func (m tuiModel) breadcrumbH() int {
	if len(m.history) > 0 || m.anchor != nil {
		return 1
	}
	return 0
}

// panelContentH is the inner content height available inside all panels.
func (m tuiModel) panelContentH() int {
	h := m.height - 1 - m.breadcrumbH() - 1 - 2
	if h < 1 {
		h = 1
	}
	return h
}

// vpWidth is the body viewport content width (full center content area).
func (m tuiModel) vpWidth() int {
	return m.centerContentW()
}

// typeMapH is the height of the type-flow map in the lower half of the center panel.
func (m tuiModel) typeMapH() int {
	available := m.panelContentH() - 2 // subtract title + divider
	if available < 2 {
		return 1
	}
	return available / 2
}

// vpHeight is the body viewport height — top half of center panel (minus title + divider + type map).
func (m tuiModel) vpHeight() int {
	h := m.panelContentH() - 2 - m.typeMapH()
	if h < 1 {
		h = 1
	}
	return h
}
