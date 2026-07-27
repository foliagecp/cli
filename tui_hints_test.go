package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
)

// ── Nothing selected by default ───────────────────────────────────────────────

// TestSelection_NothingIsHighlightedOnArrival. The panels used to open with
// row 0 highlighted, which reads as "this group is selected and is what the
// next key acts on" — while the thing actually under the cursor was the
// vertex. The highlight was a claim about the subject, and a false one.
func TestSelection_NothingIsHighlightedOnArrival(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)

	if m.rCursor != noSelection || m.lCursor != noSelection {
		t.Fatalf("cursors = (%d,%d), want nothing selected", m.rCursor, m.lCursor)
	}
	if _, onLink := m.cursorLink(); onLink {
		t.Error("no link should be selected before the user walks the list")
	}
	if m.subject().kind != subjVertex {
		t.Error("with nothing selected the subject is the vertex")
	}
}

func TestSelection_ArrivesAndLeavesWithTheSameKey(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)

	m = update(m, key("j"))
	if m.rCursor != 0 {
		t.Fatalf("j should enter the list, got %d", m.rCursor)
	}
	// Past the last row it cycles back out, so the same key that walks in
	// walks back out — there is nothing extra to learn about deselecting.
	for i := 0; i < len(m.grouped.outFlat); i++ {
		m = update(m, key("j"))
	}
	if m.rCursor != noSelection {
		t.Errorf("cursor = %d after walking off the end, want nothing selected", m.rCursor)
	}
}

func TestSelection_ALoadClearsIt(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	m = update(m, key("j"))
	m = update(m, key("j"))
	if m.rCursor < 0 {
		t.Fatal("fixture: something should be selected")
	}

	m = update(m, linksLoadedMsg{id: "hub/x", gen: m.loadGen, links: threeLinks()})
	if m.rCursor != noSelection {
		t.Errorf("a fresh load should select nothing, got %d", m.rCursor)
	}
}

func TestSelection_NoRowIsRenderedHighlighted(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)

	panel := m.renderSidePanel(true)
	if strings.Contains(panel, styleSelected.Render("")) && strings.Contains(panel, "\x1b[48;5;57m") {
		t.Error("no row should be highlighted while nothing is selected")
	}
}

// TestSelection_ASelectedLinkHasNoChevron. The ► marking a selected link is a
// near-twin of the ▸ a COLLAPSED GROUP shows, so a selected link looked like
// something Tab would expand.
func TestSelection_ASelectedLinkHasNoChevron(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m = update(m, key("j")) // group header
	m = update(m, key("j")) // first link

	row := ""
	for _, l := range strings.Split(stripANSI(m.renderSidePanel(true)), "\n") {
		if strings.Contains(l, "l1") {
			row = l
		}
	}
	if row == "" {
		t.Fatal("the first link row should be on screen")
	}
	if strings.ContainsAny(row, "►▸▾") {
		t.Errorf("a selected link must not carry a collapse-shaped marker: %q", row)
	}
}

// ── Tab is labelled wherever it does something ────────────────────────────────

func TestHints_TabIsAnnouncedOnlyWhileARowIsSelected(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 140

	if strings.Contains(stripANSI(m.renderStatus()), "Tab") {
		t.Error("Tab does nothing with no row selected — advertising it is the same lie as a key that does not exist")
	}
	m = update(m, key("j"))
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
	m = update(m, key("D"))
	if m.form == nil || !strings.Contains(m.form.title, "TYPE") {
		t.Fatalf("high-level delete should say TYPE, got %q", formTitle(m))
	}

	m2 := makeModel("hub/srv", typeLinks, nil)
	m2 = update(m2, key("x")) // low-level
	m2 = update(m2, key("D"))
	if m2.form == nil || !strings.Contains(m2.form.title, "LOW-LEVEL") {
		t.Errorf("low-level delete must say so — it leaves the CMDB record behind, got %q", formTitle(m2))
	}
}

func TestDelete_ObjectSaysObject(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("hub/srv-1", instanceOfLinkName, "hub/srv", ltInstanceOf), isOut: true},
	}
	m := makeModel("hub/srv-1", links, nil)
	m = update(m, key("D"))
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
