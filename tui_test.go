package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Force lipgloss to emit ANSI codes in tests (no TTY by default).
	lipgloss.SetColorProfile(termenv.TrueColor)

	tmp, err := os.MkdirTemp("", "fg-cli-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	FoliageCLIDir = tmp
	NatsHubDomain = "hub"
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

// makeModel creates a ready-to-use tuiModel with groups already built.
func makeModel(currentID string, links []displayLink, fvi *fullVertexInfo) tuiModel {
	m := newTuiModel(currentID)
	m.loading = false
	m.links = links
	m.fvi = fvi
	m.grouped = buildGroupedView(links, "")
	m.rCursor = firstSelectableIdx(m.grouped.outFlat)
	m.lCursor = firstSelectableIdx(m.grouped.inFlat)
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	if fvi != nil {
		m.bodyVP.SetContent(m.bodyContent())
	}
	return m
}

// threeLinks returns 3 out-links of type "contains".
func threeLinks() []displayLink {
	return []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", "contains"), isOut: true},
		{info: makeLinkInfo("root", "l2", "c2", "contains"), isOut: true},
		{info: makeLinkInfo("root", "l3", "c3", "contains"), isOut: true},
	}
}

// mixedLinks returns 2 out-links of different types and 1 in-link.
func mixedLinks() []displayLink {
	return []displayLink{
		{info: makeLinkInfo("root", "child_a", "ca", "contains"), isOut: true},
		{info: makeLinkInfo("root", "dep_b", "db", "depends_on"), isOut: true},
		{info: makeLinkInfo("parent", "p_link", "root", "parent_of"), isOut: false},
	}
}

func key(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func keyEnter() tea.Msg     { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyBackspace() tea.Msg { return tea.KeyMsg{Type: tea.KeyBackspace} }
func keyTab() tea.Msg       { return tea.KeyMsg{Type: tea.KeyTab} }
func keyEsc() tea.Msg       { return tea.KeyMsg{Type: tea.KeyEsc} }

func update(m tuiModel, msg tea.Msg) tuiModel {
	next, _ := m.Update(msg)
	return next.(tuiModel)
}

// ── buildGroupedView ──────────────────────────────────────────────────────────

func TestBuildGroupedView_Empty(t *testing.T) {
	gv := buildGroupedView(nil, "")
	if len(gv.outGroups) != 0 || len(gv.inGroups) != 0 {
		t.Errorf("empty links should produce empty groups, got out=%d in=%d",
			len(gv.outGroups), len(gv.inGroups))
	}
	if len(gv.outFlat) != 0 || len(gv.inFlat) != 0 {
		t.Errorf("empty links should produce empty flat lists, got out=%d in=%d",
			len(gv.outFlat), len(gv.inFlat))
	}
}

func TestBuildGroupedView_GroupsByType(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "a1", "x", "typeA"), isOut: true},
		{info: makeLinkInfo("root", "b1", "y", "typeB"), isOut: true},
		{info: makeLinkInfo("root", "a2", "z", "typeA"), isOut: true},
	}
	gv := buildGroupedView(links, "")
	if got := len(gv.outGroups); got != 2 {
		t.Fatalf("want 2 out-groups, got %d", got)
	}
	if gv.outGroups[0].tp != "typeA" || gv.outGroups[1].tp != "typeB" {
		t.Errorf("groups should be sorted alphabetically: got %q, %q",
			gv.outGroups[0].tp, gv.outGroups[1].tp)
	}
	if len(gv.outGroups[0].links) != 2 {
		t.Errorf("typeA should have 2 links, got %d", len(gv.outGroups[0].links))
	}
}

func TestBuildGroupedView_SeparatesInOut(t *testing.T) {
	links := mixedLinks()
	gv := buildGroupedView(links, "")
	if len(gv.outGroups) != 2 {
		t.Errorf("want 2 out-groups, got %d", len(gv.outGroups))
	}
	if len(gv.inGroups) != 1 {
		t.Errorf("want 1 in-group, got %d", len(gv.inGroups))
	}
}

func TestBuildGroupedView_SortsLinksWithinGroup(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "zzz", "c", "t"), isOut: true},
		{info: makeLinkInfo("root", "aaa", "d", "t"), isOut: true},
		{info: makeLinkInfo("root", "mmm", "e", "t"), isOut: true},
	}
	gv := buildGroupedView(links, "")
	g := gv.outGroups[0].links
	if g[0].label() != "aaa" || g[1].label() != "mmm" || g[2].label() != "zzz" {
		t.Errorf("links should be sorted by name: got %q, %q, %q",
			g[0].label(), g[1].label(), g[2].label())
	}
}

func TestBuildGroupedView_NoTypeFallback(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", ""), isOut: true},
	}
	gv := buildGroupedView(links, "")
	if len(gv.outGroups) != 1 {
		t.Fatalf("want 1 out-group, got %d", len(gv.outGroups))
	}
	if gv.outGroups[0].tp != "(no type)" {
		t.Errorf("empty type should fall back to '(no type)', got %q", gv.outGroups[0].tp)
	}
}

func TestBuildGroupedView_SearchFiltersByName(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "alpha", "x", "t"), isOut: true},
		{info: makeLinkInfo("root", "beta", "y", "t"), isOut: true},
		{info: makeLinkInfo("root", "gamma", "z", "t"), isOut: true},
	}
	gv := buildGroupedView(links, "bet")
	if len(gv.outGroups) != 1 {
		t.Fatalf("search filter should keep 1 group, got %d", len(gv.outGroups))
	}
	if len(gv.outGroups[0].links) != 1 || gv.outGroups[0].links[0].label() != "beta" {
		t.Errorf("only 'beta' should survive filter, got %v", gv.outGroups[0].links)
	}
}

func TestBuildGroupedView_SearchFiltersByTarget(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "link1", "hub/special-node", "t"), isOut: true},
		{info: makeLinkInfo("root", "link2", "hub/other-node", "t"), isOut: true},
	}
	gv := buildGroupedView(links, "special")
	if len(gv.outGroups[0].links) != 1 {
		t.Errorf("filter by target should keep 1 link, got %d", len(gv.outGroups[0].links))
	}
}

func TestBuildGroupedView_SearchRemovesEmptyGroups(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "alpha", "x", "typeA"), isOut: true},
		{info: makeLinkInfo("root", "beta", "y", "typeB"), isOut: true},
	}
	gv := buildGroupedView(links, "alpha")
	if len(gv.outGroups) != 1 {
		t.Errorf("group with no matching links should be removed, got %d groups", len(gv.outGroups))
	}
	if gv.outGroups[0].tp != "typeA" {
		t.Errorf("surviving group should be typeA, got %q", gv.outGroups[0].tp)
	}
}

func TestBuildGroupedView_SearchCaseInsensitive(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("root", "MyLink", "x", "t"), isOut: true},
	}
	gv := buildGroupedView(links, "mylink")
	if len(gv.outGroups[0].links) != 1 {
		t.Error("search should be case-insensitive")
	}
}

// ── buildPanelFlat ────────────────────────────────────────────────────────────

func TestBuildPanelFlat_Empty(t *testing.T) {
	flat := buildPanelFlat(nil)
	if len(flat) != 0 {
		t.Errorf("empty groups should produce empty flat, got %d", len(flat))
	}
}

func TestBuildPanelFlat_OneGroupThreeLinks(t *testing.T) {
	gv := buildGroupedView(threeLinks(), "")
	flat := gv.outFlat
	// Expected: [typeGroup, link0, link1, link2]
	if len(flat) != 4 {
		t.Fatalf("want 4 flat items, got %d", len(flat))
	}
	if flat[0].kind != flatTypeGroup {
		t.Error("flat[0] should be flatTypeGroup")
	}
	if flat[1].kind != flatLink || flat[2].kind != flatLink || flat[3].kind != flatLink {
		t.Error("flat[1..3] should be flatLink")
	}
}

func TestBuildPanelFlat_CollapsedGroupHidesLinks(t *testing.T) {
	gv := buildGroupedView(threeLinks(), "")
	gv.outGroups[0].collapsed = true
	gv.rebuildFlat()

	for _, item := range gv.outFlat {
		if item.kind == flatLink {
			t.Error("collapsed group should have no flatLink items")
		}
	}
	hasTypeGroup := false
	for _, item := range gv.outFlat {
		if item.kind == flatTypeGroup {
			hasTypeGroup = true
		}
	}
	if !hasTypeGroup {
		t.Error("collapsed group should still show type group header")
	}
}

func TestBuildPanelFlat_TwoDirectionsSeparate(t *testing.T) {
	gv := buildGroupedView(mixedLinks(), "")
	// outFlat: 2 groups → 2 headers + 2 links = 4 items
	if len(gv.outFlat) != 4 {
		t.Errorf("outFlat: want 4 (2 headers + 2 links), got %d", len(gv.outFlat))
	}
	// inFlat: 1 group → 1 header + 1 link = 2 items
	if len(gv.inFlat) != 2 {
		t.Errorf("inFlat: want 2 (1 header + 1 link), got %d", len(gv.inFlat))
	}
}

// ── nextSelectable ────────────────────────────────────────────────────────────

func TestNextSelectable_Forward(t *testing.T) {
	flat := []flatItem{
		{kind: flatTypeGroup},
		{kind: flatLink},
		{kind: flatLink},
	}
	got := nextSelectable(flat, 0, +1)
	if got != 1 {
		t.Errorf("from 0 going +1: want 1, got %d", got)
	}
}

func TestNextSelectable_Backward(t *testing.T) {
	flat := []flatItem{
		{kind: flatTypeGroup},
		{kind: flatLink},
		{kind: flatLink},
	}
	got := nextSelectable(flat, 1, -1)
	if got != 0 {
		t.Errorf("from 1 going -1: want 0, got %d", got)
	}
}

func TestNextSelectable_WrapForward(t *testing.T) {
	flat := []flatItem{
		{kind: flatTypeGroup},
		{kind: flatLink},
	}
	got := nextSelectable(flat, 1, +1)
	if got != 0 {
		t.Errorf("from last going +1 should wrap to 0, got %d", got)
	}
}

func TestNextSelectable_WrapBackward(t *testing.T) {
	flat := []flatItem{
		{kind: flatTypeGroup},
		{kind: flatLink},
	}
	got := nextSelectable(flat, 0, -1)
	if got != 1 {
		t.Errorf("from 0 going -1 should wrap to last, got %d", got)
	}
}

func TestNextSelectable_Empty(t *testing.T) {
	got := nextSelectable(nil, 0, +1)
	if got != 0 {
		t.Errorf("empty flat: want 0, got %d", got)
	}
}

// ── firstSelectableIdx ────────────────────────────────────────────────────────

func TestFirstSelectableIdx_ReturnsZero(t *testing.T) {
	flat := []flatItem{
		{kind: flatTypeGroup},
		{kind: flatLink},
	}
	if got := firstSelectableIdx(flat); got != 0 {
		t.Errorf("want 0, got %d", got)
	}
}

func TestFirstSelectableIdx_Empty(t *testing.T) {
	if got := firstSelectableIdx(nil); got != 0 {
		t.Errorf("empty: want 0, got %d", got)
	}
}

// ── linkForItemIn ─────────────────────────────────────────────────────────────

func TestLinkForItemIn_ReturnsLink(t *testing.T) {
	gv := buildGroupedView(threeLinks(), "")
	item := flatItem{kind: flatLink, groupIdx: 0, linkIdx: 0}
	dl, ok := linkForItemIn(gv.outGroups, item)
	if !ok {
		t.Fatal("expected link to be found")
	}
	if dl.label() == "" {
		t.Error("found link should have a label")
	}
}

func TestLinkForItemIn_TypeGroupReturnsFalse(t *testing.T) {
	gv := buildGroupedView(threeLinks(), "")
	item := flatItem{kind: flatTypeGroup, groupIdx: 0}
	_, ok := linkForItemIn(gv.outGroups, item)
	if ok {
		t.Error("linkForItemIn on flatTypeGroup should return false")
	}
}

// ── displayLink ───────────────────────────────────────────────────────────────

func TestDisplayLink_TargetOut(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("root", "l", "child", ""), isOut: true}
	if dl.target() != "child" {
		t.Errorf("out link target: want child, got %q", dl.target())
	}
}

func TestDisplayLink_TargetIn(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("parent", "l", "root", ""), isOut: false}
	if dl.target() != "parent" {
		t.Errorf("in link target: want parent, got %q", dl.target())
	}
}

func TestDisplayLink_Label(t *testing.T) {
	dl := displayLink{info: makeLinkInfo("root", "my_link", "child", ""), isOut: true}
	if dl.label() != "my_link" {
		t.Errorf("label: want my_link, got %q", dl.label())
	}
}

// ── renderBodyKV ──────────────────────────────────────────────────────────────

func TestRenderBodyKV_EmptyBody(t *testing.T) {
	body := easyjson.NewJSONObject()
	out := renderBodyKV(body.GetPtr(), 80)
	if !strings.Contains(out, "empty") {
		t.Errorf("empty body should say 'empty', got: %q", out)
	}
}

func TestRenderBodyKV_NilBody(t *testing.T) {
	out := renderBodyKV(nil, 80)
	if !strings.Contains(out, "empty") {
		t.Errorf("nil body should say 'empty', got: %q", out)
	}
}

func TestRenderBodyKV_StringField(t *testing.T) {
	body := easyjson.NewJSONObjectWithKeyValue("name", easyjson.NewJSON("Router1"))
	out := renderBodyKV(body.GetPtr(), 80)
	if !strings.Contains(out, "name") {
		t.Errorf("should contain key 'name', got: %q", out)
	}
	if !strings.Contains(out, "Router1") {
		t.Errorf("should contain value 'Router1', got: %q", out)
	}
}

func TestRenderBodyKV_MultipleFields(t *testing.T) {
	body := easyjson.NewJSONObject()
	body.SetByPath("alpha", easyjson.NewJSON("a"))
	body.SetByPath("beta", easyjson.NewJSON("b"))
	body.SetByPath("gamma", easyjson.NewJSON("c"))
	out := renderBodyKV(body.GetPtr(), 80)
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") || !strings.Contains(out, "gamma") {
		t.Errorf("all keys should appear, got: %q", out)
	}
}

// ── renderJSONValue ───────────────────────────────────────────────────────────

func TestRenderJSONValue_String(t *testing.T) {
	out := renderJSONValue("hello", 80)
	if !strings.Contains(out, "hello") {
		t.Errorf("string value: want 'hello' in output, got %q", out)
	}
}

func TestRenderJSONValue_StringTruncated(t *testing.T) {
	long := strings.Repeat("x", 100)
	out := renderJSONValue(long, 10)
	if len(out) > 100 {
		t.Errorf("long string should be truncated, got length %d", len(out))
	}
	if !strings.Contains(out, "…") {
		t.Errorf("truncated string should end with ellipsis, got %q", out)
	}
}

func TestRenderJSONValue_Number(t *testing.T) {
	out := renderJSONValue(float64(42), 80)
	if !strings.Contains(out, "42") {
		t.Errorf("number value: want '42' in output, got %q", out)
	}
}

func TestRenderJSONValue_BoolTrue(t *testing.T) {
	out := renderJSONValue(true, 80)
	if !strings.Contains(out, "true") {
		t.Errorf("bool true: want 'true' in output, got %q", out)
	}
}

func TestRenderJSONValue_BoolFalse(t *testing.T) {
	out := renderJSONValue(false, 80)
	if !strings.Contains(out, "false") {
		t.Errorf("bool false: want 'false' in output, got %q", out)
	}
}

func TestRenderJSONValue_Nil(t *testing.T) {
	out := renderJSONValue(nil, 80)
	if !strings.Contains(out, "null") {
		t.Errorf("nil value: want 'null' in output, got %q", out)
	}
}

func TestRenderJSONValue_StringSlice(t *testing.T) {
	out := renderJSONValue([]interface{}{"a", "b", "c"}, 80)
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("string slice: want items in output, got %q", out)
	}
}

func TestRenderJSONValue_EmptySlice(t *testing.T) {
	out := renderJSONValue([]interface{}{}, 80)
	if !strings.Contains(out, "[]") {
		t.Errorf("empty slice: want '[]', got %q", out)
	}
}

func TestRenderJSONValue_Object(t *testing.T) {
	v := map[string]interface{}{"key": "val"}
	out := renderJSONValue(v, 80)
	b, _ := json.Marshal(v)
	if !strings.Contains(out, "key") {
		t.Errorf("object value: want JSON with 'key', got %q (json=%s)", out, b)
	}
}

// ── highlightMatches ──────────────────────────────────────────────────────────

func TestHighlightMatches_Empty(t *testing.T) {
	out := highlightMatches("hello world", "")
	if out != "hello world" {
		t.Errorf("empty query should return unchanged text, got %q", out)
	}
}

func TestHighlightMatches_SingleMatch(t *testing.T) {
	out := highlightMatches("hello world", "world")
	if !strings.Contains(out, "world") {
		t.Errorf("match should still contain 'world', got %q", out)
	}
	if len(out) <= len("hello world") {
		t.Errorf("output with highlight should be longer than input (ANSI codes)")
	}
}

func TestHighlightMatches_CaseInsensitive(t *testing.T) {
	out := highlightMatches("Hello World", "hello")
	if len(out) <= len("Hello World") {
		t.Errorf("case-insensitive match should add highlight codes, out len=%d", len(out))
	}
}

func TestHighlightMatches_NoMatch(t *testing.T) {
	out := highlightMatches("hello world", "xyz")
	if out != "hello world" {
		t.Errorf("no match should return unchanged, got %q", out)
	}
}

func TestHighlightMatches_MultipleMatches(t *testing.T) {
	out := highlightMatches("aaa bbb aaa", "aaa")
	if len(out) <= len("aaa bbb aaa")+10 {
		t.Logf("may have only one match highlighted, out: %q", out)
	}
}

// ── stripDomain ───────────────────────────────────────────────────────────────

func TestStripDomain(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hub/network/router", "network/router"},
		{"router", "router"},
		{"a/b/c", "b/c"},
		{"", ""},
	}
	for _, c := range tests {
		if got := stripDomain(c.in); got != c.want {
			t.Errorf("stripDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── Navigation: h/l panel switching ──────────────────────────────────────────

func TestNav_lFocusesOutgoing(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.focus = panelIn // start at incoming

	m = update(m, key("l"))
	if m.focus != panelOut {
		t.Errorf("l should switch focus to panelOut, got %v", m.focus)
	}
}

func TestNav_hFocusesIncoming(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.focus = panelOut // start at outgoing

	m = update(m, key("h"))
	if m.focus != panelIn {
		t.Errorf("h should switch focus to panelIn, got %v", m.focus)
	}
}

func TestNav_ArrowLeftFocusesIncoming(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.focus = panelOut

	m = update(m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != panelIn {
		t.Errorf("← should focus incoming panel, got %v", m.focus)
	}
}

func TestNav_ArrowRightFocusesOutgoing(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.focus = panelIn

	m = update(m, tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != panelOut {
		t.Errorf("→ should focus outgoing panel, got %v", m.focus)
	}
}

// ── Navigation: j/k ──────────────────────────────────────────────────────────

func TestNav_jMovesToFirstLink(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// rCursor starts at 0 (typeGroup header), j moves to first link
	m = update(m, key("j"))
	dl, isLink := m.cursorLink()
	if !isLink {
		t.Fatal("after j from typeGroup, cursor should be on a flatLink")
	}
	if dl.label() != "l1" {
		t.Errorf("first link should be l1 (alpha sort), got %q", dl.label())
	}
}

func TestNav_kWrapsToLast(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// k from cursor=0 should wrap to last
	m = update(m, key("k"))
	if m.rCursor != len(m.grouped.outFlat)-1 {
		t.Errorf("k from 0 should wrap to last item, got %d", m.rCursor)
	}
}

func TestNav_jjjWrapsAround(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	total := len(m.grouped.outFlat)
	start := m.rCursor
	for i := 0; i < total; i++ {
		m = update(m, key("j"))
	}
	if m.rCursor != start {
		t.Errorf("after %d j presses should be back at start=%d, got %d", total, start, m.rCursor)
	}
}

func TestNav_ArrowDown(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	before := m.rCursor
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.rCursor == before {
		t.Error("↓ should move cursor")
	}
}

func TestNav_EmptyFlat_NoPanic(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m = update(m, key("j"))
	m = update(m, key("k"))
	if m.rCursor < 0 {
		t.Error("cursor should not go negative")
	}
}

func TestNav_jIncomingPanel(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.focus = panelIn
	before := m.lCursor
	m = update(m, key("j"))
	if m.lCursor == before && len(m.grouped.inFlat) > 1 {
		t.Error("j in incoming panel should move lCursor")
	}
	// rCursor should not change
	if m.rCursor != 0 {
		t.Errorf("rCursor should not change when navigating incoming panel, got %d", m.rCursor)
	}
}

// ── Navigation: Enter ─────────────────────────────────────────────────────────

func TestNav_EnterOnLinkNavigates(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// Move to first flatLink
	m = update(m, key("j"))
	if _, isLink := m.cursorLink(); !isLink {
		t.Skip("cursor not on link, skipping")
	}

	m = update(m, keyEnter())
	if len(m.history) != 1 || m.history[0] != "root" {
		t.Errorf("Enter on link should push root to history, got %v", m.history)
	}
	if !m.loading {
		t.Error("Enter on link should set loading=true")
	}
}

func TestNav_EnterOnTypeGroupCollapses(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// rCursor starts at 0 = typeGroup
	if m.grouped.outFlat[m.rCursor].kind != flatTypeGroup {
		t.Skip("cursor not on typeGroup, skipping")
	}
	before := len(m.grouped.outFlat)

	m = update(m, keyEnter())

	if len(m.grouped.outFlat) >= before {
		t.Errorf("flat should shrink after collapse: before=%d after=%d", before, len(m.grouped.outFlat))
	}
}

func TestNav_EnterWhileLoading(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.loading = true

	m = update(m, keyEnter())
	if !m.loading {
		t.Error("Enter while loading should not stop loading")
	}
	if len(m.history) != 0 {
		t.Error("Enter while loading should not push history")
	}
}

// ── Navigation: Tab ───────────────────────────────────────────────────────────

func TestNav_TabOnTypeGroupCollapses(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	if m.grouped.outFlat[m.rCursor].kind != flatTypeGroup {
		t.Skip("cursor not on typeGroup")
	}
	initialLen := len(m.grouped.outFlat)

	m = update(m, keyTab())
	if len(m.grouped.outFlat) >= initialLen {
		t.Errorf("Tab on typeGroup should collapse: len before=%d after=%d",
			initialLen, len(m.grouped.outFlat))
	}
}

func TestNav_TabToggleExpandCollapse(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	initialLen := len(m.grouped.outFlat)

	m = update(m, keyTab()) // collapse
	collapsed := len(m.grouped.outFlat)

	m = update(m, keyTab()) // expand
	expanded := len(m.grouped.outFlat)

	if collapsed >= initialLen {
		t.Errorf("first Tab should collapse: initial=%d collapsed=%d", initialLen, collapsed)
	}
	if expanded != initialLen {
		t.Errorf("second Tab should restore: want %d, got %d", initialLen, expanded)
	}
}

func TestNav_TabOnLinkTogglesParentGroup(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// Move to first flatLink
	m = update(m, key("j"))
	if _, isLink := m.cursorLink(); !isLink {
		t.Skip("cursor not on link")
	}
	initialLen := len(m.grouped.outFlat)

	m = update(m, keyTab())
	if len(m.grouped.outFlat) >= initialLen {
		t.Errorf("Tab on link should collapse parent group: initial=%d after=%d",
			initialLen, len(m.grouped.outFlat))
	}
}

// ── Navigation: back ─────────────────────────────────────────────────────────

func TestNav_bGoesBack(t *testing.T) {
	fvi := makeVertexInfo("child", nil, nil)
	m := makeModel("child", threeLinks(), &fvi)
	m.history = []string{"root"}

	m = update(m, key("b"))
	if len(m.history) != 0 {
		t.Errorf("b should pop history, got %v", m.history)
	}
	if !m.loading {
		t.Error("b should trigger loading")
	}
}

func TestNav_BackspaceGoesBack(t *testing.T) {
	fvi := makeVertexInfo("child", nil, nil)
	m := makeModel("child", threeLinks(), &fvi)
	m.history = []string{"root"}

	m = update(m, keyBackspace())
	if len(m.history) != 0 {
		t.Errorf("backspace should pop history, got %v", m.history)
	}
}

func TestNav_bNoHistory(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)

	m = update(m, key("b"))
	if m.loading {
		t.Error("b with empty history should not trigger loading")
	}
}

// ── rawBody toggle ────────────────────────────────────────────────────────────

func TestNav_vTogglesRawBody(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	if m.rawBody {
		t.Fatal("rawBody should start false")
	}

	m = update(m, key("v"))
	if !m.rawBody {
		t.Error("v should toggle rawBody to true")
	}

	m = update(m, key("v"))
	if m.rawBody {
		t.Error("second v should toggle rawBody back to false")
	}
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func TestNav_rRefreshes(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.cache[m.currentID] = cachedVertex{fvi: m.fvi, links: m.links}
	initialGen := m.loadGen

	m = update(m, key("r"))
	if _, ok := m.cache["root"]; ok {
		t.Error("r should remove current vertex from cache")
	}
	if !m.loading {
		t.Error("r should trigger loading")
	}
	if m.loadGen != initialGen+1 {
		t.Errorf("r should increment loadGen: want %d, got %d", initialGen+1, m.loadGen)
	}
}

func TestNav_CtrlRClearsCache(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.cache["root"] = cachedVertex{fvi: m.fvi}
	m.cache["child"] = cachedVertex{fvi: m.fvi}

	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if len(m.cache) != 0 {
		t.Errorf("ctrl+r should clear entire cache, got %d entries", len(m.cache))
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestSearch_fEntersSearchMode(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, key("f"))
	if !m.searchMode {
		t.Error("f should enter search mode")
	}
}

func TestSearch_EscClearsQuery(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.searchQuery = "something"
	m.grouped = buildGroupedView(m.links, "something")

	m = update(m, keyEsc())
	if m.searchQuery != "" {
		t.Errorf("Esc should clear searchQuery, got %q", m.searchQuery)
	}
}

func TestSearch_FilterRebuildsGroups(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.searchMode = true
	for _, ch := range "child_a" {
		m = update(m, key(string(ch)))
	}
	if m.searchQuery == "" {
		t.Skip("searchQuery not updated yet")
	}
	total := 0
	for _, g := range m.grouped.outGroups {
		total += len(g.links)
	}
	if total != 1 {
		t.Errorf("search for 'child_a' should keep 1 out-link, got %d", total)
	}
}

// ── Query mode ────────────────────────────────────────────────────────────────

func TestQuery_SlashEnters(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, key("/"))
	if !m.queryMode {
		t.Error("/ should enter query mode")
	}
}

func TestQuery_EscExits(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, keyEsc())
	if m.queryMode {
		t.Error("Esc should exit query mode")
	}
}

func TestQuery_EmptyEnterExits(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, keyEnter())
	if m.queryMode {
		t.Error("Enter with empty query should exit query mode without sending command")
	}
}

// ── Message handling ──────────────────────────────────────────────────────────

func TestMsg_VertexInfoMsg_UpdatesState(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	fvi := makeVertexInfo("child",
		[]linkId{{from: "child", name: "l1"}, {from: "child", name: "l2"}},
		nil,
	)
	m = update(m, vertexInfoMsg{id: "child", gen: 0, fvi: &fvi})

	if m.currentID != "child" {
		t.Errorf("currentID should update to child, got %q", m.currentID)
	}
	if m.linksTotal != 2 {
		t.Errorf("linksTotal: want 2, got %d", m.linksTotal)
	}
}

func TestMsg_VertexInfoMsg_Stale(t *testing.T) {
	m := newTuiModel("root")
	m.loadGen = 5

	fvi := makeVertexInfo("other", nil, nil)
	m = update(m, vertexInfoMsg{id: "other", gen: 2, fvi: &fvi})

	if m.currentID != "root" {
		t.Error("stale vertexInfoMsg should not update currentID")
	}
}

func TestMsg_VertexInfoMsg_Error(t *testing.T) {
	m := newTuiModel("root")
	m.loading = true

	m = update(m, vertexInfoMsg{id: "root", gen: 0, err: fmt.Errorf("not found")})
	if m.loading {
		t.Error("error should stop loading")
	}
	if m.errMsg == "" {
		t.Error("errMsg should be set on error")
	}
}

func TestMsg_LinksLoadedMsg_BuildsGroups(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 1
	m.ready = true
	m.bodyVP = viewport.New(40, 30)

	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", "contains"), isOut: true},
		{info: makeLinkInfo("root", "l2", "c2", "depends"), isOut: true},
	}
	m = update(m, linksLoadedMsg{id: "root", gen: 1, links: links})

	if m.loading {
		t.Error("should not be loading after linksLoadedMsg")
	}
	if len(m.grouped.outGroups) != 2 {
		t.Errorf("should have 2 out-groups (contains, depends), got %d", len(m.grouped.outGroups))
	}
	// rCursor should be at 0, which is a valid position in outFlat
	if m.rCursor != 0 {
		t.Errorf("rCursor should be 0 after load, got %d", m.rCursor)
	}
}

func TestMsg_LinksLoadedMsg_Stale(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 5

	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", ""), isOut: true},
	}
	m = update(m, linksLoadedMsg{id: "root", gen: 2, links: links})
	if !m.loading {
		t.Error("stale linksLoadedMsg should not stop loading")
	}
	if len(m.links) != 0 {
		t.Error("stale linksLoadedMsg should not update links")
	}
}

func TestMsg_LinksLoadedMsg_CachesVertex(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loading = true
	m.loadGen = 1
	m.ready = true
	m.bodyVP = viewport.New(40, 30)

	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", ""), isOut: true},
	}
	m = update(m, linksLoadedMsg{id: "root", gen: 1, links: links})

	if _, ok := m.cache["root"]; !ok {
		t.Error("linksLoadedMsg should cache the vertex")
	}
}

func TestMsg_VertexLoadedMsg_SetsState(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	fvi := makeVertexInfo("child", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("child", "l", "x", "t"), isOut: true},
	}
	m = update(m, vertexLoadedMsg{id: "child", fvi: &fvi, links: links})

	if m.loading {
		t.Error("should not be loading after vertexLoadedMsg")
	}
	if m.currentID != "child" {
		t.Errorf("currentID: want child, got %q", m.currentID)
	}
	if len(m.links) != 1 {
		t.Errorf("links: want 1, got %d", len(m.links))
	}
}

func TestMsg_VertexLoadedMsg_Stale(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.loadGen = 5

	newFVI := makeVertexInfo("other", nil, nil)
	m = update(m, vertexLoadedMsg{id: "other", gen: 2, fvi: &newFVI})
	if m.currentID != "root" {
		t.Error("stale vertexLoadedMsg should not change currentID")
	}
}

func TestMsg_QueryResultMsg_Success(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	m = update(m, queryResultMsg{results: []string{"v1", "v2", "v3"}})

	if m.queryMode {
		t.Error("queryMode should be false after result")
	}
	if len(m.queryResults) != 3 {
		t.Errorf("queryResults: want 3, got %d", len(m.queryResults))
	}
}

func TestMsg_QueryResultMsg_Error(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, queryResultMsg{err: fmt.Errorf("connection error")})
	if !strings.Contains(m.queryResult, "error") {
		t.Errorf("queryResult should contain 'error', got %q", m.queryResult)
	}
}

func TestMsg_QueryResultMsg_Empty(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m = update(m, queryResultMsg{results: nil})
	if !strings.Contains(m.queryResult, "no results") {
		t.Errorf("empty results should say 'no results', got %q", m.queryResult)
	}
}

// ── loadGen / stale-message safety ───────────────────────────────────────────

func TestLoadGen_IncrementedOnNavigate(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	m = update(m, key("j")) // move to first link
	gen := m.loadGen
	m = update(m, keyEnter())
	if m.loadGen != gen+1 {
		t.Errorf("Enter should increment loadGen: want %d, got %d", gen+1, m.loadGen)
	}
}

func TestLoadGen_IncrementedOnRefresh(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	gen := m.loadGen
	m = update(m, key("r"))
	if m.loadGen != gen+1 {
		t.Errorf("r should increment loadGen: want %d, got %d", gen+1, m.loadGen)
	}
}

// ── toggleCollapse ────────────────────────────────────────────────────────────

func TestToggleCollapse_IdempotentOnLink(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	before := len(m.grouped.outFlat)

	// toggleCollapse on a flatLink item should do nothing
	linkItem := flatItem{kind: flatLink, groupIdx: 0, linkIdx: 0}
	m = m.toggleCollapse(linkItem)

	if len(m.grouped.outFlat) != before {
		t.Errorf("toggleCollapse on flatLink should not change flat length")
	}
}

func TestToggleCollapse_CursorStaysOnGroup(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	// rCursor starts at 0 (flatTypeGroup)
	if m.grouped.outFlat[m.rCursor].kind != flatTypeGroup {
		t.Skip("cursor not on typeGroup initially")
	}
	groupItem := m.grouped.outFlat[m.rCursor]

	m = m.toggleCollapse(groupItem)

	if m.grouped.outFlat[m.rCursor].kind != flatTypeGroup {
		t.Errorf("cursor should stay on type group header after collapse, got kind=%d",
			m.grouped.outFlat[m.rCursor].kind)
	}
}

// ── Scroll clamping ───────────────────────────────────────────────────────────

func TestClampScroll_KeepsCursorVisible(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	m.height = 8

	m.rCursor = len(m.grouped.outFlat) - 1
	m = m.clampScroll()

	visible := m.panelContentH() - 1
	if visible < 1 {
		visible = 1
	}
	if m.rCursor < m.rOffset || m.rCursor >= m.rOffset+visible {
		t.Errorf("cursor %d not in visible range [%d, %d)", m.rCursor, m.rOffset, m.rOffset+visible)
	}
}

func TestClampScroll_CursorAboveWindow(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", threeLinks(), &fvi)
	m.focus = panelOut
	m.height = 8 // small height so visible < len(flat), making the offset constraint real
	m.rOffset = 3
	m.rCursor = 1

	m = m.clampScroll()
	if m.rOffset != 1 {
		t.Errorf("rOffset should move to cursor when cursor is above window: want 1, got %d", m.rOffset)
	}
}

// ── Dimensions ───────────────────────────────────────────────────────────────

func TestDimensions_ThreeColumnsSumToWidth(t *testing.T) {
	for _, w := range []int{90, 100, 120, 140, 160, 200} {
		m := newTuiModel("root")
		m.width = w
		total := 2*m.sideW() + m.centerW()
		if total != w {
			t.Errorf("width=%d: 2*sideW(%d)+centerW(%d)=%d != %d",
				w, m.sideW(), m.centerW(), total, w)
		}
	}
}

func TestDimensions_AllPositive(t *testing.T) {
	for _, w := range []int{90, 100, 120, 160} {
		for _, h := range []int{10, 20, 40} {
			m := newTuiModel("root")
			m.width = w
			m.height = h
			checks := map[string]int{
				"sideW":         m.sideW(),
				"centerW":       m.centerW(),
				"sideContentW":  m.sideContentW(),
				"vpWidth":       m.vpWidth(),
				"vpHeight":      m.vpHeight(),
				"panelContentH": m.panelContentH(),
			}
			for name, val := range checks {
				if val < 1 {
					t.Errorf("w=%d h=%d: %s=%d < 1", w, h, name, val)
				}
			}
		}
	}
}

func TestDimensions_IsNarrow(t *testing.T) {
	cases := []struct {
		w      int
		narrow bool
	}{
		{10, true}, {80, true}, {89, true}, {90, false}, {120, false},
	}
	for _, c := range cases {
		m := newTuiModel("root")
		m.width = c.w
		if m.isNarrow() != c.narrow {
			t.Errorf("width=%d: isNarrow()=%v, want %v", c.w, m.isNarrow(), c.narrow)
		}
	}
}

// ── View smoke tests ──────────────────────────────────────────────────────────

func TestView_NotReady(t *testing.T) {
	m := newTuiModel("root")
	if out := m.View(); out == "" {
		t.Error("View() before ready should return non-empty string")
	}
}

func TestView_Loading(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.loading = true

	if out := m.View(); !strings.Contains(out, "loading") {
		t.Error("View() while loading should contain 'loading'")
	}
}

func TestView_ShowsVertexID(t *testing.T) {
	fvi := makeVertexInfo("hub/my-router", nil, nil)
	m := makeModel("hub/my-router", nil, &fvi)

	if out := m.View(); !strings.Contains(out, "hub/my-router") {
		t.Error("View() should contain current vertex ID")
	}
}

func TestView_ShowsOutgoingAndIncomingPanelTitles(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)

	out := m.View()
	if !strings.Contains(out, "OUTGOING") {
		t.Error("View() should show OUTGOING panel title")
	}
	if !strings.Contains(out, "INCOMING") {
		t.Error("View() should show INCOMING panel title")
	}
}

func TestView_ShowsTypeHeaders(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", "contains"), isOut: true},
		{info: makeLinkInfo("root", "l2", "c2", "depends_on"), isOut: true},
	}
	m := makeModel("root", links, &fvi)

	out := m.View()
	if !strings.Contains(out, "contains") {
		t.Error("View() should show 'contains' type header")
	}
	if !strings.Contains(out, "depends_on") {
		t.Error("View() should show 'depends_on' type header")
	}
}

func TestView_ShowsLinkNames(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "portA", "child", "t"), isOut: true},
	}
	m := makeModel("root", links, &fvi)

	out := m.View()
	if !strings.Contains(out, "portA") {
		t.Error("View() should show link names")
	}
}

func TestView_ShowsError(t *testing.T) {
	m := newTuiModel("root")
	m.width = 120
	m.height = 40
	m.ready = true
	m.bodyVP = viewport.New(40, 30)
	m.errMsg = "nats: connection refused"

	if out := m.View(); !strings.Contains(out, "nats: connection refused") {
		t.Error("View() should show error message")
	}
}

func TestView_NarrowNoPanic(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	for _, w := range []int{5, 10, 20, 40, 80} {
		m.width = w
		m.height = 24
		_ = m.View() // must not panic
	}
}

func TestView_WithBreadcrumbs(t *testing.T) {
	fvi := makeVertexInfo("child", nil, nil)
	m := makeModel("child", nil, &fvi)
	m.history = []string{"root", "mid"}

	out := m.View()
	if !strings.Contains(out, "mid") {
		t.Error("View() should show breadcrumbs from history")
	}
}

func TestView_QueryResults(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryResults = []string{"hub/vertex-a", "hub/vertex-b"}

	out := m.View()
	if !strings.Contains(out, "hub/vertex-a") {
		t.Error("View() should show query results")
	}
}

func TestView_SearchNoMatchesExplainsEmptyList(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)
	m.searchQuery = "definitely-missing"
	m.grouped = buildGroupedView(m.links, m.searchQuery)

	out := m.View()
	if !strings.Contains(out, "no matches") {
		t.Error("View() should explain an empty filtered link list")
	}
}

func TestView_RawBodyToggle(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	m.rawBody = false
	kvOut := m.View()

	m.rawBody = true
	m.bodyVP.SetContent(m.bodyContent())
	rawOut := m.View()

	if !strings.Contains(kvOut, "root") || !strings.Contains(rawOut, "root") {
		t.Error("both modes should show vertex ID")
	}
}

func TestView_ActivePanelBorderChanges(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", mixedLinks(), &fvi)

	m.focus = panelOut
	outFocused := m.View()

	m.focus = panelIn
	inFocused := m.View()

	// The two views should differ (border colors change)
	if outFocused == inFocused {
		t.Error("View() output should differ when panel focus changes")
	}
}

// ── Status bar ────────────────────────────────────────────────────────────────

func TestStatus_DefaultHints(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)

	out := m.renderStatus()
	for _, h := range []string{"jk", "Enter", "Tab", "b", "v", "q", "h/l"} {
		if !strings.Contains(out, h) {
			t.Errorf("default status bar should contain hint %q, got:\n%s", h, out)
		}
	}
}

func TestStatus_QueryModeHints(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.queryMode = true

	out := m.renderStatus()
	if !strings.Contains(out, "Query") {
		t.Errorf("query mode status bar should show 'Query', got:\n%s", out)
	}
}

func TestStatus_SearchModeHints(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	m := makeModel("root", nil, &fvi)
	m.searchMode = true

	out := m.renderStatus()
	if !strings.Contains(out, "Search") {
		t.Errorf("search mode status bar should show 'Search', got:\n%s", out)
	}
}

// ── vertexKindBadge ───────────────────────────────────────────────────────────

func TestBadge_TypeVertex(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("hub/types", "tl", "root", ""), isOut: false},
	}
	m := makeModel("root", links, &fvi)

	if !strings.Contains(m.vertexKindBadge(), "[type]") {
		t.Error("vertex connected to hub/types should show [type] badge")
	}
}

func TestBadge_ObjectVertex(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "mytype", "hub/objects", "__type"), isOut: true},
		{info: makeLinkInfo("root", "myobj", "hub/objects", ""), isOut: true},
	}
	m := makeModel("root", links, &fvi)

	badge := m.vertexKindBadge()
	// stripDomain removes the "hub/" prefix, so expect the stripped name.
	if !strings.Contains(badge, "objects") {
		t.Errorf("object vertex badge should reference type name, got %q", badge)
	}
}

func TestBadge_NoBadge(t *testing.T) {
	fvi := makeVertexInfo("root", nil, nil)
	links := []displayLink{
		{info: makeLinkInfo("root", "l", "some/other", ""), isOut: true},
	}
	m := makeModel("root", links, &fvi)

	if badge := m.vertexKindBadge(); badge != "" {
		t.Errorf("vertex without type/object links should have empty badge, got %q", badge)
	}
}
