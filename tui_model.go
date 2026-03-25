package main

import (
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Messages ──────────────────────────────────────────────────────────────────

// vertexLoadedMsg delivers a fully-loaded vertex in one shot (cache hits and tests).
type vertexLoadedMsg struct {
	id         string
	gen        int // 0 = skip stale check (tests)
	fvi        *fullVertexInfo
	links      []displayLink
	err        error
	partialErr error
}

type cachedVertex struct {
	fvi   *fullVertexInfo
	links []displayLink
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

	searchMode  bool
	searchInput textinput.Model
	searchQuery string

	cache map[string]cachedVertex

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

	si := textinput.New()
	si.Placeholder = "search…"
	si.CharLimit = 128
	si.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	si.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	return tuiModel{
		currentID:   startID,
		loading:     true,
		queryInput:  ti,
		searchInput: si,
		cache:       make(map[string]cachedVertex),
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

func cacheHitCmd(id string, gen int, cv cachedVertex) tea.Cmd {
	return func() tea.Msg {
		return vertexLoadedMsg{id: id, gen: gen, fvi: cv.fvi, links: cv.links}
	}
}

// fetchLinksCmd loads all link details (phase 2). Knows total count from fvi.
// Concurrency: NumCPU*3, minimum 4.
func fetchLinksCmd(id string, gen int, fvi *fullVertexInfo) tea.Cmd {
	return func() tea.Msg {
		type result struct {
			dl  displayLink
			err error
		}

		all := make([]struct {
			lid   linkId
			isOut bool
		}, 0, len(fvi.outLinks)+len(fvi.inLinks))
		for _, lid := range fvi.outLinks {
			all = append(all, struct {
				lid   linkId
				isOut bool
			}{lid, true})
		}
		for _, lid := range fvi.inLinks {
			all = append(all, struct {
				lid   linkId
				isOut bool
			}{lid, false})
		}

		results := make([]result, len(all))
		workers := runtime.NumCPU() * 3
		if workers < 4 {
			workers = 4
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, item := range all {
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
			}(i, item.lid, item.isOut)
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
		sort.Slice(links, func(i, j int) bool {
			if links[i].isOut != links[j].isOut {
				return links[i].isOut
			}
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
	styleTagKey  = lipgloss.NewStyle().Foreground(colorIn)
	styleMetaVal = lipgloss.NewStyle().Foreground(lipgloss.Color("253"))

	styleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("253")).
			Padding(0, 1)

	styleHintKey = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	styleHintSep = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleErr     = lipgloss.NewStyle().Foreground(colorErr)
	styleLoading = lipgloss.NewStyle().Foreground(colorLoading)
	styleSearch  = lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("16"))
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

func (m tuiModel) breadcrumbH() int {
	if len(m.history) > 0 {
		return 1
	}
	return 0
}

// Total height available for panel content (inside borders).
// Layout: header(1) + panels + breadcrumbs(0-1) + status(1)
func (m tuiModel) panelContentH() int {
	h := m.height - 1 - m.breadcrumbH() - 1 - 2 // -header -breadcrumbs -status -top/bottom borders
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
