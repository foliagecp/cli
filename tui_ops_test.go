package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// ── Additive test helpers ─────────────────────────────────────────────────────
//
// The existing update() in tui_test.go discards the tea.Cmd, which is right for
// the pure key-handling tests it was written for. Mutations need the command, so
// these live alongside rather than replacing it.

func updateCmd(m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(tuiModel), cmd
}

func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// withOps swaps the live operation layer for the duration of one test. Fields
// left nil in `o` will panic if called — deliberately: that means the flow
// under test reached for an operation it should not have.
func withOps(t *testing.T, o graphOps) {
	t.Helper()
	old := ops
	ops = o
	t.Cleanup(func() { ops = old })
}

// ── opResult mapping ──────────────────────────────────────────────────────────

func TestResultFromErr_NilIsApplied(t *testing.T) {
	r := resultFromErr(nil)
	if r.status != opApplied {
		t.Fatalf("status = %v, want opApplied", r.status)
	}
	if !r.ok() {
		t.Error("ok() should be true")
	}
}

func TestResultFromErr_ErrorIsFailed(t *testing.T) {
	r := resultFromErr(errors.New("boom"))
	if r.status != opFailed {
		t.Fatalf("status = %v, want opFailed", r.status)
	}
	if r.details != "boom" {
		t.Errorf("details = %q, want %q", r.details, "boom")
	}
	if r.ok() {
		t.Error("ok() should be false")
	}
}

func TestResultFromDetails_EmptyOpStackIsNoop(t *testing.T) {
	// The whole point of the *WithDetails twins: the client's lenient mapping
	// returns nil for both ok and idle, so an empty op_stack is the only
	// honest signal that nothing was written.
	data := easyjson.NewJSONObjectWithKeyValue("op_stack", easyjson.NewJSONArray())
	r := resultFromDetails(data, nil)
	if r.status != opNoop {
		t.Fatalf("status = %v, want opNoop for an empty op_stack", r.status)
	}
}

func TestResultFromDetails_MissingOpStackIsNoop(t *testing.T) {
	r := resultFromDetails(easyjson.NewJSONObject(), nil)
	if r.status != opNoop {
		t.Fatalf("status = %v, want opNoop when op_stack is absent", r.status)
	}
}

func TestResultFromDetails_NonEmptyOpStackIsApplied(t *testing.T) {
	stack := easyjson.NewJSONArray()
	stack.AddToArray(easyjson.NewJSONObjectWithKeyValue("op", easyjson.NewJSON("vertex.update")))
	data := easyjson.NewJSONObjectWithKeyValue("op_stack", stack)

	r := resultFromDetails(data, nil)
	if r.status != opApplied {
		t.Fatalf("status = %v, want opApplied", r.status)
	}
}

func TestResultFromDetails_ErrorWins(t *testing.T) {
	stack := easyjson.NewJSONArray()
	stack.AddToArray(easyjson.NewJSON("x"))
	data := easyjson.NewJSONObjectWithKeyValue("op_stack", stack)

	r := resultFromDetails(data, errors.New("nope"))
	if r.status != opFailed {
		t.Fatalf("status = %v, want opFailed even with a populated op_stack", r.status)
	}
}

// ── vertexKind ────────────────────────────────────────────────────────────────

func TestVertexKind_Plain(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	if k, tp := m.vertexKind(); k != vkPlain || tp != "" {
		t.Fatalf("vertexKind() = (%v,%q), want (vkPlain,\"\")", k, tp)
	}
}

func TestVertexKind_Type(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
	}
	m := makeModel("hub/srv", links, nil)
	if k, _ := m.vertexKind(); k != vkType {
		t.Fatalf("vertexKind() = %v, want vkType", k)
	}
}

func TestVertexKind_Object(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("hub/objects", "srv-1", "hub/srv-1", "__object"), isOut: false},
		{info: makeLinkInfo("hub/srv-1", "type", "hub/srv", "__type"), isOut: true},
	}
	m := makeModel("hub/srv-1", links, nil)
	k, tp := m.vertexKind()
	if k != vkObject {
		t.Fatalf("vertexKind() = %v, want vkObject", k)
	}
	if tp != "srv" {
		t.Errorf("type name = %q, want %q", tp, "srv")
	}
}

func TestVertexKind_TypeWinsOverTypeOutLink(t *testing.T) {
	// A types-link is stored as a __type edge, exactly like an object's
	// instance-of edge. Membership in the types topology must decide, or a
	// type that declares a types-link would be misread as an object.
	links := []displayLink{
		{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
		{info: makeLinkInfo("hub/srv", "rack", "hub/rack", "__type"), isOut: true},
	}
	m := makeModel("hub/srv", links, nil)
	if k, _ := m.vertexKind(); k != vkType {
		t.Fatalf("vertexKind() = %v, want vkType (types membership must win)", k)
	}
}

func TestVertexKind_ObjectWithoutTypeLinkStaysPlain(t *testing.T) {
	// Half-written object: in the objects topology but its __type link never
	// landed. We cannot name its type, so HL calls would fail — report plain.
	links := []displayLink{
		{info: makeLinkInfo("hub/objects", "srv-1", "hub/srv-1", "__object"), isOut: false},
	}
	m := makeModel("hub/srv-1", links, nil)
	if k, _ := m.vertexKind(); k != vkPlain {
		t.Fatalf("vertexKind() = %v, want vkPlain", k)
	}
}

// vertexKindBadge must keep behaving exactly as before the extraction.
func TestVertexKindBadge_MatchesKind(t *testing.T) {
	typeLinks := []displayLink{
		{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
	}
	if got := makeModel("hub/srv", typeLinks, nil).vertexKindBadge(); !strings.Contains(got, "[type]") {
		t.Errorf("type badge = %q, want it to contain [type]", got)
	}
	if got := makeModel("hub/x", threeLinks(), nil).vertexKindBadge(); got != "" {
		t.Errorf("plain badge = %q, want empty", got)
	}
}

// ── cache invalidation ────────────────────────────────────────────────────────

func TestInvalidate_EvictsListedAndIgnoresEmpty(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m.cache["hub/a"] = cachedVertex{}
	m.cache["hub/b"] = cachedVertex{}
	m.cache["hub/c"] = cachedVertex{}

	m = m.invalidate("hub/a", "", "hub/c")

	if _, ok := m.cache["hub/a"]; ok {
		t.Error("hub/a should have been evicted")
	}
	if _, ok := m.cache["hub/c"]; ok {
		t.Error("hub/c should have been evicted")
	}
	if _, ok := m.cache["hub/b"]; !ok {
		t.Error("hub/b should have survived")
	}
}

func TestInvalidateAll_ClearsEverything(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m.cache["hub/a"] = cachedVertex{}
	m.cache["hub/b"] = cachedVertex{}

	m = m.invalidateAll()

	if len(m.cache) != 0 {
		t.Fatalf("cache size = %d, want 0", len(m.cache))
	}
}

// ── viewRestore ───────────────────────────────────────────────────────────────

func TestCaptureApplyRestore_KeepsSelection(t *testing.T) {
	m := makeModel("root", threeLinks(), nil)
	m.rCursor = 2 // header + l1 → this is link "l2"

	sel, ok := selectedLinkIn(m.grouped.outGroups, m.grouped.outFlat, m.rCursor)
	if !ok {
		t.Fatalf("fixture: cursor %d is not on a link", m.rCursor)
	}
	r := captureRestore(m)

	// Simulate a reload: groups rebuilt from scratch, cursors zeroed.
	m.grouped = buildGroupedView(threeLinks(), "")
	m.rCursor, m.lCursor = 0, 0

	m = applyRestore(m, r)

	got, ok := selectedLinkIn(m.grouped.outGroups, m.grouped.outFlat, m.rCursor)
	if !ok {
		t.Fatalf("after restore the cursor is not on a link (idx %d)", m.rCursor)
	}
	if got.info.id.name != sel.info.id.name {
		t.Errorf("selection = %q, want %q", got.info.id.name, sel.info.id.name)
	}
}

func TestCaptureApplyRestore_KeepsCollapse(t *testing.T) {
	m := makeModel("root", mixedLinks(), nil)
	m.grouped.outGroups[0].collapsed = true
	collapsedType := m.grouped.outGroups[0].tp
	m.grouped.rebuildFlat()

	r := captureRestore(m)

	m.grouped = buildGroupedView(mixedLinks(), "") // rebuild drops collapse
	m = applyRestore(m, r)

	for _, g := range m.grouped.outGroups {
		if g.tp == collapsedType && !g.collapsed {
			t.Fatalf("group %q should still be collapsed after restore", collapsedType)
		}
	}
}

func TestApplyRestore_DeletedLinkFallsBackToItsGroupHeader(t *testing.T) {
	// The normal case right after deleting the selected link: it is gone, and
	// dropping the user at index 0 of an unrelated group would be disorienting.
	m := makeModel("root", threeLinks(), nil)
	m.rCursor = 2 // "l2"
	r := captureRestore(m)

	remaining := []displayLink{
		{info: makeLinkInfo("root", "l1", "c1", "contains"), isOut: true},
		{info: makeLinkInfo("root", "l3", "c3", "contains"), isOut: true},
	}
	m.grouped = buildGroupedView(remaining, "")
	m.rCursor = 0
	m = applyRestore(m, r)

	if m.rCursor >= len(m.grouped.outFlat) {
		t.Fatalf("cursor %d out of range (%d rows)", m.rCursor, len(m.grouped.outFlat))
	}
	item := m.grouped.outFlat[m.rCursor]
	if item.kind != flatTypeGroup {
		t.Errorf("cursor landed on kind %v, want the group header of the deleted link's type", item.kind)
	}
}

func TestApplyRestore_NilIsNoop(t *testing.T) {
	m := makeModel("root", threeLinks(), nil)
	m.rCursor = 2
	got := applyRestore(m, nil)
	if got.rCursor != 2 {
		t.Errorf("rCursor = %d, want it untouched (2)", got.rCursor)
	}
}

func TestCaptureRestore_PreservesFocus(t *testing.T) {
	m := makeModel("root", mixedLinks(), nil)
	m.focus = panelIn
	r := captureRestore(m)

	m.focus = panelOut
	m = applyRestore(m, r)

	if m.focus != panelIn {
		t.Errorf("focus = %v, want panelIn", m.focus)
	}
}
