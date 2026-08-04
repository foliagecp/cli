package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
)

// ── The centre column is where the vertex lives ───────────────────────────────

// TestFocus_ArrivesOnTheCentreColumn. A highlighted row is a claim about what
// the next key acts on. The panels used to open with row 0 highlighted while
// the thing actually under the cursor was the vertex — so the claim was false
// until the user walked into a list. Focus answers it instead: an unfocused
// panel draws no highlight at all, so standing on the centre column asserts
// nothing about either list.
func TestFocus_ArrivesOnTheCentreColumn(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)

	if m.focus != panelCenter {
		t.Fatalf("focus = %v on arrival, want the centre column", m.focus)
	}
	if m.subject().kind != subjVertex {
		t.Error("on the centre column the subject is the vertex")
	}
	if _, onLink := m.cursorLink(); onLink {
		t.Error("no link is selected while the centre column has focus")
	}
}

func TestFocus_NeitherSidePanelIsHighlightedFromTheCentre(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", mixedLinks(), &fvi)

	for _, isOut := range []bool{true, false} {
		if strings.Contains(m.renderSidePanel(isOut), highlightBG) {
			t.Errorf("isOut=%v: an unfocused panel must not highlight a row", isOut)
		}
	}
}

func TestFocus_MovesAcrossThreeColumnsAndClamps(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", mixedLinks(), &fvi)

	m = update(m, key("l"))
	if m.focus != panelOut {
		t.Fatalf("l from the centre = %v, want the outgoing column", m.focus)
	}
	m = update(m, key("l"))
	if m.focus != panelOut {
		t.Error("the columns are a position on screen, not a ring — l must clamp")
	}
	m = update(m, key("h"))
	m = update(m, key("h"))
	if m.focus != panelIn {
		t.Fatalf("two h presses = %v, want the incoming column", m.focus)
	}
	m = update(m, key("h"))
	if m.focus != panelIn {
		t.Error("h must clamp at the left column")
	}
}

func TestFocus_SteppingIntoALinkPanelSelectsItsFirstRow(t *testing.T) {
	// This is what the focus model buys: the highlight can be there from the
	// first keystroke, because by then it is true.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m = update(m, key("l"))

	if m.rCursor != 0 {
		t.Errorf("rCursor = %d, want the first row", m.rCursor)
	}
	if !strings.Contains(m.renderSidePanel(true), highlightBG) {
		t.Error("the focused panel should highlight its cursor row")
	}
}

func TestFocus_TheCentreColumnShowsItHasFocus(t *testing.T) {
	// stylePanelActive used to apply to the side panels only, so the centre
	// column had no focused state — it was not in the cycle.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)

	focused := m.renderCenterPanel()
	m.focus = panelOut
	unfocused := m.renderCenterPanel()
	if focused == unfocused {
		t.Error("the centre column must look different when it has focus")
	}
}

func TestFocus_AnEmptyLinkPanelHasNoSubjectAtAll(t *testing.T) {
	// Not a fallback to the vertex. The vertex lives in the centre column, and
	// letting a side column reach it would undo the whole point of putting the
	// centre in the cycle — you could stand in the outgoing panel, never touch
	// the cursor, press i, and be editing the vertex.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi) // out-links only
	m = update(m, key("h"))                     // the empty incoming column

	if m.subject().kind != subjNone {
		t.Error("an empty link panel has no subject")
	}
	m = update(m, key("i"))
	if m.form != nil {
		t.Error("i from a side column must not reach the vertex")
	}
	if !strings.Contains(stripANSI(m.queryResult), "centre column") {
		t.Errorf("the refusal should point at the centre column, got %q", stripANSI(m.queryResult))
	}
}

func TestFocus_SurvivesNavigation(t *testing.T) {
	// Walking a chain of links should not need an l press per hop.
	m := makeModel("hub/x", threeLinks(), nil)
	m = update(m, key("l"))
	m = update(m, linksLoadedMsg{id: "hub/y", gen: m.loadGen, links: threeLinks()})

	if m.focus != panelOut {
		t.Errorf("focus = %v after a load, want it kept", m.focus)
	}
}

// ── Tab is labelled wherever it does something ────────────────────────────────

// highlightBG is the selected-row background as lipgloss emits it. It combines
// foreground and background into one sequence, so the background code is a
// substring rather than a sequence of its own.
const highlightBG = "48;5;57"

func TestHints_TabIsAnnouncedOnlyInALinkPanel(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 140

	if strings.Contains(stripANSI(m.renderStatus()), "Tab") {
		t.Error("Tab does nothing on the centre column — advertising it is the same lie as a key that does not exist")
	}
	m = update(m, key("l"))
	if !strings.Contains(stripANSI(m.renderStatus()), "Tab:collapse") {
		t.Error("Tab collapses the group under the cursor and must say so")
	}
}

func TestHints_EveryTabCyclerLabelsItself(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	base := func() tuiModel {
		m := makeModel("hub/x", threeLinks(), &fvi)
		m.width = 140
		return m
	}
	linkForm := base()
	linkForm.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	linkForm = update(linkForm, key("L"))

	cases := map[string]tuiModel{
		"go to":  update(base(), key(":")),
		"export": update(base(), key("e")),
		"form":   linkForm,
	}
	for name, m := range cases {
		if !strings.Contains(stripANSI(m.renderStatus()), "Tab") {
			t.Errorf("%s: Tab cycles here and the status bar does not say so:\n%s",
				name, stripANSI(m.renderStatus()))
		}
	}
}

func TestHints_TheCreateMenuDoesNotClaimTabDoesAnything(t *testing.T) {
	// It is letter accelerators, not a field form. Advertising Tab because
	// "forms have Tab" would be the same drift the keymap table exists to
	// prevent, one level down.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 140
	m = update(m, key("n"))

	if strings.Contains(stripANSI(m.renderStatus()), "Tab") {
		t.Error("nothing cycles in the create menu")
	}
}

func TestHints_AFormAdvertisesItsOwnKeysNotTheBrowseOnes(t *testing.T) {
	// The status bar used to fall through to the browse hints over a form, so
	// it advertised `jk:nav  Enter:go  d:del` while every one of those keys was
	// being typed into a text field.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 140
	m = update(m, key("n"))

	out := stripANSI(m.renderStatus())
	if strings.Contains(out, "jk:nav") || strings.Contains(out, "d:del") {
		t.Errorf("a form must not advertise the browse keys:\n%s", out)
	}
	if !strings.Contains(out, "Esc") {
		t.Errorf("a form must say how to get out of it:\n%s", out)
	}
}

// ── The hint line fits ────────────────────────────────────────────────────────

func TestHints_FitTheTerminalAndKeepWhatMatters(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	for _, w := range []int{60, 80, 100, 140} {
		m := makeModel("hub/x", threeLinks(), &fvi)
		m.width = w
		line := m.renderStatus()
		if got := lipgloss.Width(line); got > w {
			t.Errorf("w=%d: status bar is %d cells wide", w, got)
		}
		plain := stripANSI(line)
		// Whatever gets dropped, these two never do: one is the API every
		// write depends on, the other is where the dropped ones still live.
		if !strings.Contains(plain, "CRUD:") {
			t.Errorf("w=%d: the CRUD API must always be stated: %q", w, plain)
		}
		if !strings.Contains(plain, "?:help") {
			t.Errorf("w=%d: help must always be reachable: %q", w, plain)
		}
	}
}

func TestHints_AnErrorTakesTheLineFromTheHints(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 100
	m.errMsg = "the server said no"

	if !strings.Contains(stripANSI(m.renderStatus()), "the server said no") {
		t.Error("an error must be readable in full — clipping it to a stub tells nobody anything")
	}
}

// ── The CRUD API is always stated ─────────────────────────────────────────────

func TestCrudMode_IsVisibleInBothPositions(t *testing.T) {
	// It used to appear only when armed, as a small [LL] beside the id, so the
	// default was an unlabelled mode — and "which API is this about to call"
	// is the question every create, edit and delete turns on.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", nil, &fvi)
	m.width = 140

	if !strings.Contains(stripANSI(m.renderStatus()), "high-level") {
		t.Error("the default API must be stated, not implied by the absence of a marker")
	}
	m = update(m, key("x"))
	if !strings.Contains(stripANSI(m.renderStatus()), "low-level") {
		t.Error("the switched API must be stated")
	}
}

func TestDelete_NamesWhatItIsAboutToRemove(t *testing.T) {
	// The same vertex is a "type" through the CMDB API and a bare "vertex"
	// through the low-level one, and those remove very different amounts of
	// graph. The confirmation has to say which.
	typeLinks := []displayLink{
		{info: makeLinkInfo(hubID("types"), "srv", "hub/srv", ltInstanceOf), isOut: false},
	}
	m := makeModel("hub/srv", typeLinks, nil)
	m = update(m, key("d"))
	if m.form == nil || !strings.Contains(m.form.title, "TYPE") {
		t.Fatalf("high-level delete should say TYPE, got %q", formTitle(m))
	}

	m2 := makeModel("hub/srv", typeLinks, nil)
	m2 = update(m2, key("x")) // low-level
	m2 = update(m2, key("d"))
	if m2.form == nil || !strings.Contains(m2.form.title, "LOW-LEVEL") {
		t.Errorf("low-level delete must say so — it leaves the CMDB record behind, got %q", formTitle(m2))
	}
}

func TestDelete_ObjectSaysObject(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("hub/srv-1", instanceOfLinkName, "hub/srv", ltInstanceOf), isOut: true},
	}
	m := makeModel("hub/srv-1", links, nil)
	m = update(m, key("d"))
	if m.form == nil || !strings.Contains(m.form.title, "OBJECT") {
		t.Errorf("deleting an object should say so, got %q", formTitle(m))
	}
}

func formTitle(m tuiModel) string {
	if m.form == nil {
		return "<no form>"
	}
	return m.form.title
}

// ── Jump to an id ─────────────────────────────────────────────────────────────

// TestGoto_ReachesAnIdWithoutWalking. A graph browser with no address bar
// means the only way to a vertex you can name is to remember the path there —
// and R, the sole shortcut, resets the history, the filter and any pending
// link along with it.
func TestGoto_ReachesAnIdWithoutWalking(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	m = update(m, key(":"))
	if m.mode() != modeGoto {
		t.Fatal(": should open the id prompt")
	}

	for _, r := range "srv-1" {
		m = update(m, key(string(r)))
	}
	m, cmd := updateCmd(m, keyEnter())

	if m.mode() != modeBrowse {
		t.Error("Enter should leave the prompt")
	}
	if cmd == nil {
		t.Fatal("Enter should start the load")
	}
	if len(m.history) != 1 || m.history[0] != "hub/x" {
		t.Errorf("history = %v — a jump should be undoable with b", m.history)
	}
}

func TestGoto_QualifiesABareId(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key(":"))
	for _, r := range "root" {
		m = update(m, key(string(r)))
	}
	_, cmd := updateCmd(m, keyEnter())

	msg := runCmd(cmd)
	if info, ok := msg.(vertexInfoMsg); ok && info.id != hubID("root") {
		t.Errorf("jumped to %q, want the canonical form", info.id)
	}
}

func TestGoto_TabCyclesTheBuiltIns(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key(":"))

	m = update(m, keyTab())
	if got := m.gotoInput.Value(); got != "root" {
		t.Errorf("first Tab = %q, want root", got)
	}
	m = update(m, keyTab())
	if got := m.gotoInput.Value(); got != "types" {
		t.Errorf("second Tab = %q, want types", got)
	}
}

func TestGoto_RefusesAnInvalidIdWithoutLeavingTheUserGuessing(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key(":"))
	for _, r := range "a.b" { // dots are the KV key separator
		m = update(m, key(string(r)))
	}
	m, cmd := updateCmd(m, keyEnter())

	if cmd != nil {
		t.Error("an invalid id must not be sent to the server")
	}
	if !strings.Contains(stripANSI(m.queryResult), "dot") {
		t.Errorf("the refusal should say what is wrong, got %q", stripANSI(m.queryResult))
	}
}

func TestGoto_EscLeavesEverythingAlone(t *testing.T) {
	body := easyjson.NewJSONObject()
	fvi := fullVertexInfo{id: "hub/x", body: body.GetPtr()}
	m := makeModel("hub/x", threeLinks(), &fvi)
	m = update(m, key(":"))
	m = typeText(m, "elsewhere")
	m = update(m, keyEsc())

	if m.mode() != modeBrowse {
		t.Error("Esc should close the prompt")
	}
	if m.currentID != "hub/x" || len(m.history) != 0 {
		t.Error("Esc must not move you")
	}
}

func TestFocus_NarrowLayoutSaysWhichColumnHasFocus(t *testing.T) {
	// Stacked in one column there are no borders, so the thing the wide layout
	// says with a highlighted frame has to be written out.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width, m.height = 60, 24

	if !strings.Contains(stripANSI(m.View()), "the vertex") {
		t.Error("the narrow layout must say the centre column has focus")
	}
	m = update(m, key("l"))
	if !strings.Contains(stripANSI(m.View()), "outgoing") {
		t.Error("the narrow layout must say which link panel has focus")
	}
}

func TestFocus_NarrowLayoutShowsTheSubjectAboveTheLists(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width, m.height = 60, 24
	m = m.refreshBody()

	out := stripANSI(m.View())
	if !strings.Contains(out, "name") {
		t.Errorf("the vertex body should be reachable at 60 columns:\n%s", out)
	}
	if !strings.Contains(out, "l1") {
		t.Errorf("the link list should still be there:\n%s", out)
	}
}

// ── The armed API governs what the CRUD keys can touch ────────────────────────

// TestCrud_HighLevelRefusesAPlainVertex. `i` on a plain vertex used to fall
// through to ops.vertexUpdate — a LOW-LEVEL write issued while the status bar
// said CRUD: high-level. The chip exists to end exactly that guessing, and
// instead it was describing something that was not happening.
func TestCrud_HighLevelRefusesAPlainVertex(t *testing.T) {
	withOps(t, graphOps{}) // any call panics

	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", nil, &fvi)

	m = update(m, key("i"))
	if m.form != nil {
		t.Fatal("the high-level API has nothing to edit on a plain vertex")
	}
	if !strings.Contains(stripANSI(m.queryResult), "press x") {
		t.Errorf("a refusal must name the key that makes it possible, got %q", stripANSI(m.queryResult))
	}

	m = update(m, key("x"))
	m = update(m, key("i"))
	if m.form == nil {
		t.Error("with the low-level API armed a plain vertex is exactly what can be edited")
	}
}

func TestCrud_HighLevelRefusesARawLink(t *testing.T) {
	withOps(t, graphOps{})

	m := makeModel("hub/a", []displayLink{rawLink()}, nil)
	m.focus = panelOut
	m.rCursor = 1
	m = m.applyLinkDetail(linkDetailMsg{
		key:    keyOf(rawLink()),
		detail: linkDetail{loaded: true, body: easyjson.NewJSONObject()},
	})

	m = update(m, key("i"))
	if m.form != nil {
		t.Fatal("a raw link has no high-level form")
	}
	if !strings.Contains(stripANSI(m.queryResult), "press x") {
		t.Errorf("the refusal should name the switch, got %q", stripANSI(m.queryResult))
	}
}

func TestCrud_HighLevelStillEditsTypesAndObjects(t *testing.T) {
	body := easyjson.NewJSONObject()
	for _, c := range []struct {
		name  string
		id    string
		links []displayLink
	}{
		{"type", "hub/srv", []displayLink{
			{info: makeLinkInfo(hubID("types"), "srv", "hub/srv", ltInstanceOf), isOut: false},
		}},
		{"object", "hub/srv-1", []displayLink{
			{info: makeLinkInfo("hub/srv-1", instanceOfLinkName, "hub/srv", ltInstanceOf), isOut: true},
		}},
	} {
		fvi := fullVertexInfo{id: c.id, body: body.GetPtr()}
		m := makeModel(c.id, c.links, &fvi)
		m = update(m, key("i"))
		if m.form == nil {
			t.Errorf("%s: the high-level API should edit this", c.name)
		}
	}
}

func TestCrud_ABrokenObjectPointsAtTheRepairPath(t *testing.T) {
	// The one case where the low-level API earns its keep: the high-level one
	// will refuse this vertex too, and repairing it is what the raw one is for.
	fvi := makeVertexInfo("hub/srv-1", nil, nil)
	m := makeModel("hub/srv-1", []displayLink{
		{info: makeLinkInfo(hubID("objects"), "hub/srv-1", "hub/srv-1", ltInstance), isOut: false},
	}, &fvi)

	m = update(m, key("i"))
	if m.form != nil {
		t.Fatal("a broken object cannot be addressed by the high-level API")
	}
	out := stripANSI(m.queryResult)
	if !strings.Contains(out, "broken") || !strings.Contains(out, "press x") {
		t.Errorf("the refusal should name the problem and the way out, got %q", out)
	}
}
