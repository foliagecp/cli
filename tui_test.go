package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "fg-cli-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	FoliageCLIDir = tmp
	os.Exit(m.Run())
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

func makeVertexInfo(id string, outLinks, inLinks []linkId) fullVertexInfo {
	body := easyjson.NewJSONObjectWithKeyValue("name", easyjson.NewJSON(id))
	return fullVertexInfo{
		id:       id,
		body:     body.GetPtr(),
		outLinks: outLinks,
		inLinks:  inLinks,
	}
}

func makeLinkInfo(from, name, to, tp string) fullLinkInfo {
	body := easyjson.NewJSONObject()
	return fullLinkInfo{
		id:   linkId{from: from, name: name},
		body: body.GetPtr(),
		to:   to,
		tp:   tp,
		tags: []string{},
	}
}

func makeModel(currentID string, links []displayLink, fvi *fullVertexInfo) tuiModel {
	m := newTuiModel(currentID)
	m.loading = false
	m.links = links
	m.fvi = fvi
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	return m
}

func key(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyEnter() tea.Msg     { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyBackspace() tea.Msg { return tea.KeyMsg{Type: tea.KeyBackspace} }

func update(m tuiModel, msg tea.Msg) tuiModel {
	next, _ := m.Update(msg)
	return next.(tuiModel)
}

// ── displayLink ───────────────────────────────────────────────────────────────

func TestDisplayLinkTarget_Out(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("root", "child_link", "child1", ""), isOut: true}
	if got := dl.target(); got != "child1" {
		t.Errorf("out link target: want child1, got %q", got)
	}
}

func TestDisplayLinkTarget_In(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("parent", "p_link", "root", ""), isOut: false}
	if got := dl.target(); got != "parent" {
		t.Errorf("in link target: want parent, got %q", got)
	}
}

func TestDisplayLinkLabel(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("root", "my_link", "child1", ""), isOut: true}
	if got := dl.label(); got != "my_link" {
		t.Errorf("label: want my_link, got %q", got)
	}
}

// ── Counts ────────────────────────────────────────────────────────────────────

func TestOutInCount_Nil(t *testing.T) {
	m := newTuiModel("root")
	if m.outCount() != 0 || m.inCount() != 0 {
		t.Error("counts should be 0 when fvi is nil")
	}
}

func TestOutInCount(t *testing.T) {
	fvi := makeVertexInfo("root",
		[]linkId{{from: "root", name: "l1"}, {from: "root", name: "l2"}},
		[]linkId{{from: "parent", name: "pl"}},
	)
	m := makeModel("root", nil, &fvi)
	if got := m.outCount(); got != 2 {
		t.Errorf("outCount: want 2, got %d", got)
	}
	if got := m.inCount(); got != 1 {
		t.Errorf("inCount: want 1, got %d", got)
	}
}

// ── Cursor navigation ─────────────────────────────────────────────────────────

func threeLinks() []displayLink {
	return []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", ""), isOut: true},
		{info: makeLinkInfo("root", "l2", "c2", ""), isOut: true},
		{info: makeLinkInfo("root", "l3", "c3", ""), isOut: true},
	}
}

func TestCursor_DownAndUp(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)

	m = update(m, key("j"))
	if m.cursor != 1 {
		t.Errorf("after j: want cursor 1, got %d", m.cursor)
	}

	m = update(m, key("j"))
	if m.cursor != 2 {
		t.Errorf("after jj: want cursor 2, got %d", m.cursor)
	}

	m = update(m, key("k"))
	if m.cursor != 1 {
		t.Errorf("after jjk: want cursor 1, got %d", m.cursor)
	}
}

func TestCursor_DownBoundary(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.cursor = 2

	m = update(m, key("j"))
	if m.cursor != 0 {
		t.Errorf("j at last item should wrap to 0, got %d", m.cursor)
	}
}

func TestCursor_UpBoundary(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	// cursor already 0

	m = update(m, key("k"))
	if m.cursor != 2 {
		t.Errorf("k at first item should wrap to last (2), got %d", m.cursor)
	}
}

func TestCursor_ArrowKeys(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)

	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("↓: want cursor 1, got %d", m.cursor)
	}

	m = update(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("↑: want cursor 0, got %d", m.cursor)
	}
}

func TestCursor_EmptyLinks(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	// Should not panic
	m = update(m, key("j"))
	m = update(m, key("k"))
	if m.cursor != 0 {
		t.Errorf("cursor should stay 0 on empty links, got %d", m.cursor)
	}
}

// ── History / back navigation ─────────────────────────────────────────────────

func TestHistory_EnterPushes(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)

	m = update(m, keyEnter())
	if len(m.history) != 1 || m.history[0] != "root" {
		t.Errorf("Enter should push current ID to history, got %v", m.history)
	}
	if !m.loading {
		t.Error("Enter should set loading=true")
	}
}

func TestHistory_BackPops(t *testing.T) {
	fvi := makeVertexInfo("child1", nil, nil)
	m := makeModel("child1", threeLinks(), &fvi)
	m.history = []string{"root"}

	m = update(m, key("b"))
	if len(m.history) != 0 {
		t.Errorf("b should pop history, got %v", m.history)
	}
	if !m.loading {
		t.Error("b should set loading=true")
	}
}

func TestHistory_BackEmpty(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	// no history

	m = update(m, key("b"))
	if m.loading {
		t.Error("b with no history should not trigger loading")
	}
}

func TestHistory_BackspaceKey(t *testing.T) {
	fvi := makeVertexInfo("child1", nil, nil)
	m := makeModel("child1", threeLinks(), &fvi)
	m.history = []string{"root"}

	m = update(m, keyBackspace())
	if len(m.history) != 0 {
		t.Errorf("backspace should pop history, got %v", m.history)
	}
}

// ── two-phase loading ─────────────────────────────────────────────────────────

func TestVertexInfoMsg_ShowsLinkCount(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	fvi := makeVertexInfo("child1",
		[]linkId{{from: "child1", name: "l1"}, {from: "child1", name: "l2"}},
		[]linkId{{from: "parent", name: "pl"}},
	)
	msg := vertexInfoMsg{id: "child1", gen: 0, fvi: &fvi}
	m = update(m, msg)

	if m.linksTotal != 3 {
		t.Errorf("linksTotal: want 3, got %d", m.linksTotal)
	}
	if m.loading == false {
		t.Error("should still be loading (links not fetched yet)")
	}
	if m.currentID != "child1" {
		t.Errorf("currentID should update after phase 1, got %q", m.currentID)
	}
}

func TestVertexInfoMsg_Error(t *testing.T) {
	m := newTuiModel("root")
	m.loading = true

	msg := vertexInfoMsg{id: "root", gen: 0, err: os.ErrNotExist}
	m = update(m, msg)

	if m.loading {
		t.Error("loading should stop on vertexInfoMsg error")
	}
	if m.errMsg == "" {
		t.Error("errMsg should be set on error")
	}
}

func TestVertexInfoMsg_Stale(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loadGen = 3

	msg := vertexInfoMsg{id: "other", gen: 1, fvi: &fvi} // stale gen
	m = update(m, msg)

	if m.currentID != "root" {
		t.Error("stale vertexInfoMsg should not change currentID")
	}
}

func TestLinksLoadedMsg_UpdatesLinks(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 1
	m.ready = true
	m.bodyVP = viewport.New(40, 30)

	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "child1", ""), isOut: true},
	}
	msg := linksLoadedMsg{id: "root", gen: 1, links: links}
	m = update(m, msg)

	if m.loading {
		t.Error("loading should stop after linksLoadedMsg")
	}
	if len(m.links) != 1 {
		t.Errorf("links: want 1, got %d", len(m.links))
	}
	if m.linksTotal != 0 {
		t.Errorf("linksTotal should reset to 0 after load, got %d", m.linksTotal)
	}
}

func TestLinksLoadedMsg_Stale(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 5

	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "child1", ""), isOut: true},
	}
	msg := linksLoadedMsg{id: "root", gen: 2, links: links} // stale
	m = update(m, msg)

	if !m.loading {
		t.Error("stale linksLoadedMsg should not stop loading")
	}
	if len(m.links) != 0 {
		t.Error("stale linksLoadedMsg should not update links")
	}
}

func TestLinksLoadedMsg_PartialError(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 1
	m.ready = true
	m.bodyVP = viewport.New(40, 30)

	msg := linksLoadedMsg{
		id:         "root",
		gen:        1,
		links:      nil,
		partialErr: fmt.Errorf("2 link(s) failed to load"),
	}
	m = update(m, msg)

	if m.loading {
		t.Error("should not be loading after linksLoadedMsg")
	}
	if !strings.Contains(m.errMsg, "link(s) failed") {
		t.Errorf("errMsg should mention failed links, got %q", m.errMsg)
	}
}

func TestNavigation_IncrementsLoadGen(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	initialGen := m.loadGen

	m = update(m, keyEnter())

	if m.loadGen != initialGen+1 {
		t.Errorf("Enter should increment loadGen: want %d, got %d", initialGen+1, m.loadGen)
	}
}

func TestRefresh_IncrementsLoadGen(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	initialGen := m.loadGen

	m = update(m, key("r"))

	if m.loadGen != initialGen+1 {
		t.Errorf("r should increment loadGen: want %d, got %d", initialGen+1, m.loadGen)
	}
}

// ── partial load errors ───────────────────────────────────────────────────────

func TestVertexLoadedMsg_PartialError(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	fvi := makeVertexInfo("root", nil, nil)
	msg := vertexLoadedMsg{
		id:         "root",
		fvi:        &fvi,
		links:      nil,
		partialErr: fmt.Errorf("2 link(s) failed to load"),
	}

	m = update(m, msg)

	if m.loading {
		t.Error("should not be loading after partialErr msg")
	}
	if !strings.Contains(m.errMsg, "link(s) failed") {
		t.Errorf("errMsg should mention failed links, got %q", m.errMsg)
	}
}

// ── vertexLoadedMsg ───────────────────────────────────────────────────────────

func TestVertexLoadedMsg_Success(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	fvi := makeVertexInfo("child1",
		[]linkId{{from: "child1", name: "l1"}},
		nil,
	)
	links := []displayLink{
		{info: makeLinkInfo("child1", "l1", "grandchild", ""), isOut: true},
	}
	msg := vertexLoadedMsg{id: "child1", fvi: &fvi, links: links}

	m = update(m, msg)

	if m.loading {
		t.Error("should not be loading after vertexLoadedMsg")
	}
	if m.currentID != "child1" {
		t.Errorf("currentID: want child1, got %q", m.currentID)
	}
	if len(m.links) != 1 {
		t.Errorf("links: want 1, got %d", len(m.links))
	}
	if m.errMsg != "" {
		t.Errorf("errMsg should be empty, got %q", m.errMsg)
	}
}

func TestVertexLoadedMsg_Error(t *testing.T) {
	m := newTuiModel("root")
	m.loading = true

	msg := vertexLoadedMsg{id: "root", err: os.ErrNotExist}
	m = update(m, msg)

	if m.loading {
		t.Error("should not be loading after error")
	}
	if m.errMsg == "" {
		t.Error("errMsg should be set on error")
	}
}

func TestVertexLoadedMsg_ResetsCursor(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.cursor = 2

	newFVI := makeVertexInfo("leaf", nil, nil)
	msg := vertexLoadedMsg{id: "leaf", fvi: &newFVI, links: nil}
	m = update(m, msg)

	if m.cursor != 0 {
		t.Errorf("cursor should reset to 0 when links < old cursor, got %d", m.cursor)
	}
}

// ── queryResultMsg ────────────────────────────────────────────────────────────

func TestQueryResultMsg_Success(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, queryResultMsg{results: []string{"v1", "v2"}})

	if m.queryMode {
		t.Error("queryMode should be false after result")
	}
	if len(m.queryResults) != 2 || m.queryResults[0] != "v1" || m.queryResults[1] != "v2" {
		t.Errorf("queryResults should be [v1 v2], got %v", m.queryResults)
	}
	if m.queryResult != "" {
		t.Errorf("queryResult should be empty on success, got %q", m.queryResult)
	}
}

func TestQueryResultMsg_Error(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, queryResultMsg{err: os.ErrInvalid})

	if m.queryMode {
		t.Error("queryMode should be false after error")
	}
	if !strings.Contains(m.queryResult, "error") {
		t.Errorf("queryResult should contain 'error', got %q", m.queryResult)
	}
}

func TestQueryResultMsg_Empty(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, queryResultMsg{results: nil})

	if !strings.Contains(m.queryResult, "no results") {
		t.Errorf("empty result should show 'no results', got %q", m.queryResult)
	}
}

// ── Query mode ────────────────────────────────────────────────────────────────

func TestQueryMode_SlashEnters(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, key("/"))
	if !m.queryMode {
		t.Error("/ should enter query mode")
	}
}

func TestQueryMode_EscExits(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.queryMode {
		t.Error("Esc should exit query mode")
	}
}

func TestQueryMode_EmptyEnterExits(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true
	// queryInput is empty

	m = update(m, keyEnter())
	if m.queryMode {
		t.Error("Enter on empty query should exit query mode")
	}
}

// ── clampScroll ───────────────────────────────────────────────────────────────

func TestClampScroll_CursorBelowView(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.height = 10 // small height → panelContentH is small

	m.cursor = 0
	m.linkOff = 0
	// Move cursor to end
	m.cursor = 2
	m = m.clampScroll()

	// linkOff should have increased to keep cursor in view
	visible := m.panelContentH() - 1
	if m.cursor < m.linkOff || m.cursor >= m.linkOff+visible {
		t.Errorf("cursor %d not visible in window [%d, %d)", m.cursor, m.linkOff, m.linkOff+visible)
	}
}

func TestClampScroll_CursorAboveView(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.linkOff = 2
	m.cursor = 0

	m = m.clampScroll()

	if m.linkOff != 0 {
		t.Errorf("linkOff should reset to 0 when cursor moves above view, got %d", m.linkOff)
	}
}

// ── Dimensions ────────────────────────────────────────────────────────────────

func TestDimensions_SumToWidth(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40

	if m.bodyOuterW()+m.linksOuterW() != m.width {
		t.Errorf("bodyOuterW(%d) + linksOuterW(%d) != width(%d)",
			m.bodyOuterW(), m.linksOuterW(), m.width)
	}
}

func TestDimensions_PanelContentH_Positive(t *testing.T) {
	m := newTuiModel("root")
	m.height = 10
	if h := m.panelContentH(); h <= 0 {
		t.Errorf("panelContentH should be positive, got %d", h)
	}
}

func TestDimensions_NarrowWidths(t *testing.T) {
	for _, w := range []int{1, 5, 10, 15, 19, 20, 40, 59} {
		m := newTuiModel("root")
		m.width = w
		m.height = 24

		if got := m.bodyOuterW(); got < 1 {
			t.Errorf("width=%d: bodyOuterW()=%d < 1", w, got)
		}
		if got := m.linksOuterW(); got < 1 {
			t.Errorf("width=%d: linksOuterW()=%d < 1", w, got)
		}
		if got := m.vpWidth(); got < 1 {
			t.Errorf("width=%d: vpWidth()=%d < 1", w, got)
		}
		if got := m.vpHeight(); got < 1 {
			t.Errorf("width=%d: vpHeight()=%d < 1", w, got)
		}
		if got := m.panelContentH(); got < 1 {
			t.Errorf("width=%d: panelContentH()=%d < 1", w, got)
		}
	}
}

func TestDimensions_IsNarrow(t *testing.T) {
	cases := []struct {
		width  int
		narrow bool
	}{
		{10, true},
		{19, true},
		{59, true},
		{60, false},
		{120, false},
	}
	for _, c := range cases {
		m := newTuiModel("root")
		m.width = c.width
		if got := m.isNarrow(); got != c.narrow {
			t.Errorf("width=%d: isNarrow()=%v, want %v", c.width, got, c.narrow)
		}
	}
}

// ── View smoke tests ──────────────────────────────────────────────────────────

func TestView_NotReady(t *testing.T) {
	m := newTuiModel("root")
	out := m.View()
	if out == "" {
		t.Error("View() should return non-empty string even when not ready")
	}
}

func TestView_Loading(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	out := m.View()
	if !strings.Contains(out, "loading") {
		t.Errorf("View() should show 'loading' indicator, got:\n%s", out)
	}
}

func TestView_WithVertex(t *testing.T) {
	fvi := makeVertexInfo("root",
		[]linkId{{from: "root", name: "l1"}},
		[]linkId{{from: "parent", name: "pl"}},
	)
	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "child1", "contains"), isOut: true},
		{info: makeLinkInfo("parent", "pl", "root", "parent"), isOut: false},
	}
	m := makeModel("root", links, &fvi)
	m.bodyVP.SetContent(m.bodyContent())

	out := m.View()
	if !strings.Contains(out, "root") {
		t.Errorf("View() should contain vertex ID 'root'")
	}
	if !strings.Contains(out, "l1") {
		t.Errorf("View() should contain link name 'l1'")
	}
}

func TestView_WithError(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.errMsg = "connection refused"

	out := m.View()
	if !strings.Contains(out, "connection refused") {
		t.Errorf("View() should show error message, got:\n%s", out)
	}
}

func TestView_QueryMode(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m = update(m, key("/"))

	out := m.View()
	if !strings.Contains(out, "Query") {
		t.Errorf("View() in query mode should show 'Query', got:\n%s", out)
	}
}

func TestView_WithQueryResult(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryResults = []string{"vertex_a", "vertex_b"}

	out := m.View()
	if !strings.Contains(out, "vertex_a") {
		t.Errorf("View() should show query results, got:\n%s", out)
	}
}

func TestView_WithHistory(t *testing.T) {
	fvi := makeVertexInfo("child1", nil, nil)
	m := makeModel("child1", nil, &fvi)
	m.history = []string{"root"}

	out := m.View()
	if !strings.Contains(out, "root") {
		t.Errorf("View() should show previous vertex in header, got:\n%s", out)
	}
}

func TestView_NarrowWidths(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "child1", "contains"), isOut: true},
	}
	for _, w := range []int{10, 15, 19, 20, 40, 59} {
		m := makeModel("root", links, &fvi)
		m.width = w
		m.height = 24

		// must not panic
		out := m.View()
		if out == "" {
			t.Errorf("width=%d: View() returned empty string", w)
		}
		if !m.isNarrow() {
			t.Errorf("width=%d: expected isNarrow()=true", w)
		}
	}
}

func TestView_NarrowContainsVertexID(t *testing.T) {
	fvi := makeVertexInfo("myvertex", nil, nil)
	m := makeModel("myvertex", nil, &fvi)
	m.width = 20
	m.height = 24

	out := m.View()
	if !strings.Contains(out, "myvertex") {
		t.Errorf("narrow View() should contain vertex ID, got:\n%s", out)
	}
}

// ── Header rendering ──────────────────────────────────────────────────────────

func TestHeader_ContainsCurrentID(t *testing.T) {
	fvi := makeVertexInfo("pak1/my-vertex", nil, nil)
	m := makeModel("pak1/my-vertex", nil, &fvi)

	out := m.renderHeader()
	if !strings.Contains(out, "pak1/my-vertex") {
		t.Errorf("header should always contain current vertex ID, got:\n%s", out)
	}
}

func TestHeader_ShowsHistoryBack(t *testing.T) {
	fvi := makeVertexInfo("child", nil, nil)
	m := makeModel("child", nil, &fvi)
	m.history = []string{"root"}

	out := m.renderHeader()
	if !strings.Contains(out, "root") {
		t.Errorf("header should show previous vertex when history non-empty, got:\n%s", out)
	}
}

// ── Body panel title rendering ────────────────────────────────────────────────

func TestVertexKindBadge_Object(t *testing.T) {
	NatsHubDomain = "hub"
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "type", "hub/ui_controller", "__type"), isOut: true},
		{info: makeLinkInfo("hub/objects", "obj_link", "root", ""), isOut: false},
	}
	m := makeModel("root", links, &fvi)

	badge := m.vertexKindBadge()
	if !strings.Contains(badge, "hub/ui_controller") {
		t.Errorf("object badge should contain type name, got: %q", badge)
	}
}

func TestVertexKindBadge_Type(t *testing.T) {
	NatsHubDomain = "hub"
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("hub/types", "type_link", "root", ""), isOut: false},
	}
	m := makeModel("root", links, &fvi)

	badge := m.vertexKindBadge()
	if !strings.Contains(badge, "[type]") {
		t.Errorf("type badge should contain [type], got: %q", badge)
	}
}

func TestVertexKindBadge_NeitherEmpty(t *testing.T) {
	NatsHubDomain = "hub"
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "child", "hub/other", "contains"), isOut: true},
	}
	m := makeModel("root", links, &fvi)

	if badge := m.vertexKindBadge(); badge != "" {
		t.Errorf("badge should be empty when no objects/types link, got: %q", badge)
	}
}

func TestVertexKindBadge_ObjectWithoutType(t *testing.T) {
	NatsHubDomain = "hub"
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("hub/objects", "obj_link", "root", ""), isOut: false},
	}
	m := makeModel("root", links, &fvi)

	if badge := m.vertexKindBadge(); badge != "" {
		t.Errorf("object without __type should produce no badge, got: %q", badge)
	}
}

func TestVertexKindBadge_NoLinksLoaded(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	if badge := m.vertexKindBadge(); badge != "" {
		t.Errorf("badge should be empty when links not loaded, got: %q", badge)
	}
}
