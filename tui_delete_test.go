package main

import (
	"strings"
	"testing"

	"github.com/foliagecp/easyjson"
)

func typeVertexLinks() []displayLink {
	return []displayLink{
		{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
	}
}

func objectVertexLinks() []displayLink {
	return []displayLink{
		{info: makeLinkInfo("hub/objects", "srv-1", "hub/srv-1", "__object"), isOut: false},
		{info: makeLinkInfo("hub/srv-1", "type", "hub/srv", "__type"), isOut: true},
	}
}

// ── Which entity `d` targets ──────────────────────────────────────────────────

func TestDelete_DOnLinkRowTargetsTheLink(t *testing.T) {
	m := makeModel("root", threeLinks(), nil)
	m = update(m, key("x")) // a raw link needs the low-level API
	m.focus = panelOut
	m.rCursor = 1 // first link row, below the group header

	m = update(m, key("d"))

	if m.form == nil {
		t.Fatal("d should open a confirmation")
	}
	if m.form.kind != formDeleteLink {
		t.Fatalf("kind = %v, want formDeleteLink", m.form.kind)
	}
	if m.form.ctx.linkName != "l1" {
		t.Errorf("link name = %q, want l1", m.form.ctx.linkName)
	}
}

// TestDelete_ASideColumnNeverReachesTheVertex. Standing in the outgoing panel
// on a group header, `d` used to fall back to deleting the VERTEX — which
// undoes the whole point of putting the centre column in the focus cycle.
func TestDelete_ASideColumnNeverReachesTheVertex(t *testing.T) {
	withOps(t, graphOps{}) // any call panics
	m := makeModel("hub/x", threeLinks(), nil)
	m.focus = panelOut
	m.rCursor = 0 // the group header, not a link

	m = update(m, key("d"))

	if m.form != nil {
		t.Fatalf("a side column must not act on the vertex, got %+v", m.form)
	}
	if !strings.Contains(stripANSI(m.queryResult), "centre column") {
		t.Errorf("the refusal should point at the centre column, got %q", stripANSI(m.queryResult))
	}
}

func TestDelete_FromTheCentreColumnTargetsTheVertex(t *testing.T) {
	m := makeModel("hub/srv-1", objectVertexLinks(), nil) // focus starts on the centre

	m = update(m, key("d"))

	if m.form == nil || m.form.kind != formDeleteVertex {
		t.Fatalf("the centre column is where the vertex lives, got %+v", m.form)
	}
}

// ── Tiers ─────────────────────────────────────────────────────────────────────

func TestDelete_BuiltInIsRefusedWithoutAForm(t *testing.T) {
	m := makeModel("hub/types", nil, nil)
	m = update(m, key("d"))

	if m.form != nil {
		t.Error("a built-in vertex must not even open a confirmation")
	}
	if !strings.Contains(m.queryResult, "built-in") {
		t.Errorf("toast = %q, want it to explain the refusal", m.queryResult)
	}
}

func TestDelete_OrdinaryTierConfirmsWithY(t *testing.T) {
	called := ""
	withOps(t, graphOps{
		objectDelete: func(id string) opResult { called = id; return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m = update(m, key("d"))
	if m.form == nil || m.form.confirmWord != "" {
		t.Fatal("an object delete should be a single-key confirmation")
	}

	m, cmd := updateCmd(m, key("y"))
	msg := runCmd(cmd)
	if called != "hub/srv-1" {
		t.Fatalf("objectDelete(%q), want hub/srv-1", called)
	}
	if _, ok := msg.(mutationResultMsg); !ok {
		t.Fatalf("expected a mutationResultMsg, got %T", msg)
	}
}

func TestDelete_OrdinaryTierCancelsWithN(t *testing.T) {
	withOps(t, graphOps{}) // any call panics
	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m = update(m, key("d"))
	m = update(m, key("n"))

	if m.form != nil {
		t.Error("n should close the confirmation")
	}
}

func TestDelete_TypeTierRequiresTheNameTyped(t *testing.T) {
	called := false
	withOps(t, graphOps{
		typeDelete: func(string) opResult { called = true; return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv", typeVertexLinks(), nil)
	m = update(m, key("d"))
	if m.form == nil {
		t.Fatal("d should open a confirmation")
	}
	if m.form.confirmWord != "srv" {
		t.Fatalf("confirmWord = %q, want the bare type name", m.form.confirmWord)
	}
	if !strings.Contains(m.form.title, "every object") {
		t.Errorf("title = %q, want it to state the cascade", m.form.title)
	}

	// Enter with nothing typed must not delete.
	m, cmd := updateCmd(m, keyEnter())
	runCmd(cmd)
	if called {
		t.Fatal("a cascading delete ran without the name being typed")
	}
	if m.form == nil {
		t.Fatal("the form should stay open after a failed confirmation")
	}

	// Wrong name must not delete either.
	for _, r := range "srvX" {
		m = update(m, key(string(r)))
	}
	m, cmd = updateCmd(m, keyEnter())
	runCmd(cmd)
	if called {
		t.Fatal("a cascading delete ran with the wrong name typed")
	}

	// The right name goes through.
	m = makeModel("hub/srv", typeVertexLinks(), nil)
	m = update(m, key("d"))
	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	_, cmd = updateCmd(m, keyEnter())
	runCmd(cmd)
	if !called {
		t.Error("typing the exact name should confirm the delete")
	}
}

// ── Consequences of a successful delete ───────────────────────────────────────

func TestDelete_NavigatesAwayFromTheDeletedVertex(t *testing.T) {
	// gWalkTo persists the current id on every load. Staying on a deleted
	// vertex would leave a dead id on disk and break the NEXT launch.
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", nil, nil)
	m.history = []string{"hub/parent"}

	m = update(m, key("x"))
	m = update(m, key("d"))
	m, cmd := updateCmd(m, key("y"))
	if m.pendingNavAfterDelete != "hub/parent" {
		t.Fatalf("pendingNavAfterDelete = %q, want the history entry", m.pendingNavAfterDelete)
	}

	msg := runCmd(cmd)
	next, _ := m.Update(msg)
	m = next.(tuiModel)

	if !m.loading {
		t.Error("a load should have been kicked off toward a live vertex")
	}
	if m.pendingNavAfterDelete != "" {
		t.Error("the pending navigation should be consumed")
	}
}

func TestDelete_FallsBackToRootWithNoHistory(t *testing.T) {
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", nil, nil)
	m.history = nil

	m = update(m, key("x"))
	m = update(m, key("d"))
	m, _ = updateCmd(m, key("y"))

	if m.pendingNavAfterDelete != hubID("root") {
		t.Errorf("pendingNavAfterDelete = %q, want %q", m.pendingNavAfterDelete, hubID("root"))
	}
}

func TestDelete_FailureStaysPut(t *testing.T) {
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opFailed, details: "in use"} },
	})

	m := makeModel("hub/x", nil, nil)
	m.history = []string{"hub/parent"}
	m = update(m, key("x"))
	m = update(m, key("d"))
	m, cmd := updateCmd(m, key("y"))
	next, _ := m.Update(runCmd(cmd))
	m = next.(tuiModel)

	if m.loading {
		t.Error("a failed delete must not navigate away")
	}
	if m.errMsg != "in use" {
		t.Errorf("errMsg = %q, want the server's reason", m.errMsg)
	}
}

func TestDelete_ClearsAnAnchorOnTheDeletedVertex(t *testing.T) {
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", nil, nil)
	m.linking = &pendingLink{fromID: "hub/x"}

	m = update(m, key("x"))
	m = update(m, key("d"))
	m, cmd := updateCmd(m, key("y"))
	next, _ := m.Update(runCmd(cmd))
	m = next.(tuiModel)

	if m.linking != nil {
		t.Error("an anchor on the deleted vertex should be cleared")
	}
}

func TestDelete_LinkUsesTheOwningVertex(t *testing.T) {
	// For an incoming link the owner is the OTHER vertex; linkId.from already
	// holds it, and deleting against the displayed vertex would be wrong.
	var gotFrom, gotName string
	withOps(t, graphOps{
		linkDelete: func(from, name string) opResult {
			gotFrom, gotName = from, name
			return opResult{status: opApplied}
		},
	})

	m := makeModel("root", mixedLinks(), nil)
	m = update(m, key("x")) // a raw link is a low-level entity
	m.focus = panelIn
	m.lCursor = 1 // the incoming link from hub/parent

	m = update(m, key("d"))
	if m.form == nil {
		t.Fatal("d should open a confirmation")
	}
	_, cmd := updateCmd(m, key("y"))
	runCmd(cmd)

	if gotFrom != "parent" {
		t.Errorf("linkDelete from = %q, want the owning vertex (parent)", gotFrom)
	}
	if gotName != "p_link" {
		t.Errorf("linkDelete name = %q, want p_link", gotName)
	}
}

func TestDelete_RefusedWhileLoading(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m.loading = true
	m = update(m, key("d"))
	if m.form != nil {
		t.Error("a delete must not be staged against a vertex that is still loading")
	}
}

func TestDelete_ConfirmationRendersInTheStatusBar(t *testing.T) {
	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m = update(m, key("d"))

	out := m.renderStatus()
	if !strings.Contains(out, "hub/srv-1") {
		t.Errorf("status = %q, want the target named", out)
	}
	if !strings.Contains(out, "y:confirm") {
		t.Errorf("status = %q, want the confirmation keys", out)
	}
}

func TestDelete_EscapeClosesWithoutCalling(t *testing.T) {
	withOps(t, graphOps{}) // any call panics
	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m = update(m, key("d"))
	m = update(m, keyEsc())
	if m.form != nil {
		t.Error("esc should close the confirmation")
	}
}

// Guard the low-level override: with `x` on, a type must be deleted through the
// raw graph API, not the CMDB one.
func TestDelete_LowLevelModeUsesTheRawAPI(t *testing.T) {
	called := ""
	withOps(t, graphOps{
		typeDelete:   func(string) opResult { called = "type"; return opResult{status: opApplied} },
		vertexDelete: func(string) opResult { called = "vertex"; return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv", typeVertexLinks(), nil)
	m.llMode = true
	m = update(m, key("d"))
	if m.form.confirmWord != "" {
		t.Error("a raw vertex delete should not demand a typed name")
	}
	_, cmd := updateCmd(m, key("y"))
	runCmd(cmd)

	if called != "vertex" {
		t.Errorf("called the %q API, want vertex", called)
	}
}

var _ = easyjson.NewJSONObject

// ── Where a delete leaves you, and what it invalidates ────────────────────────

// TestDelete_ObjectGoesUpToItsType. Staying on the deleted object shows its
// body as though nothing happened — and on a runtime with a trash can that is
// literally true, because the object is parked rather than erased. The type is
// where the deletion is visible: the object is no longer in its list.
func TestDelete_ObjectGoesUpToItsType(t *testing.T) {
	withOps(t, graphOps{
		objectDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m.history = []string{"hub/somewhere-unrelated"}
	m = update(m, key("d"))
	m, _ = updateCmd(m, key("y"))

	if m.pendingNavAfterDelete != "hub/srv" {
		t.Errorf("after deleting an object you land on %q, want its type", m.pendingNavAfterDelete)
	}
	if len(m.history) != 1 {
		t.Error("going to the entity's home is not a step back — the history stands")
	}
}

func TestDelete_TypeGoesUpToTheTypesRoot(t *testing.T) {
	withOps(t, graphOps{
		typeDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv", typeVertexLinks(), nil)
	m = update(m, key("d"))
	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	m, _ = updateCmd(m, keyEnter())

	if m.pendingNavAfterDelete != hubID("types") {
		t.Errorf("after deleting a type you land on %q, want the types root", m.pendingNavAfterDelete)
	}
}

func TestDelete_PlainVertexStillFallsBackToTheHistory(t *testing.T) {
	// A plain vertex has no home, so there is nothing better than where the
	// user came from.
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", nil, nil)
	m.history = []string{"hub/parent"}
	m = update(m, key("x"))
	m = update(m, key("d"))
	m, _ = updateCmd(m, key("y"))

	if m.pendingNavAfterDelete != "hub/parent" {
		t.Errorf("pendingNavAfterDelete = %q, want the history entry", m.pendingNavAfterDelete)
	}
}

// TestNavigation_AlwaysFetches is the replacement for a whole family of
// cache-invalidation tests, and the answer to "I delete an object, walk to the
// trash can, and it is not there until I restart the CLI".
//
// It was there. The browser was replaying a link list captured before the
// object was parked. Chasing that with invalidation lists is unwinnable — the
// CLI cannot know every vertex a server-side operation touches (the trash can
// is one; restoring FROM it is the next), and it cannot know about the other
// people writing to the same graph at all.
//
// So nothing is cached and every navigation is a real read.
func TestNavigation_AlwaysFetches(t *testing.T) {
	m := makeModel("hub/srv-1", objectVertexLinks(), nil)

	for _, dest := range []string{hubID("trash_can"), "hub/srv", hubID("root")} {
		next, cmd := m.navigateTo(dest)
		if cmd == nil {
			t.Fatalf("navigating to %s should load it", dest)
		}
		if _, isFetch := cmd().(vertexInfoMsg); !isFetch {
			t.Errorf("%s was not fetched — a browser that shows a snapshot is "+
				"showing something that may no longer be true", dest)
		}
		if !next.loading {
			t.Errorf("%s: a real fetch should show as loading", dest)
		}
	}
}

// TestReload_IsTheOneThingNavigationCannotDo. Standing still while somebody
// else changes the graph is the one case a fetch-on-navigate model does not
// cover, which is exactly what `r` is for.
func TestReload_IsTheOneThingNavigationCannotDo(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	gen := m.loadGen

	m, cmd := updateCmd(m, key("r"))
	if cmd == nil || m.loadGen != gen+1 || !m.loading {
		t.Fatal("r should reload where you are standing")
	}
	if _, isFetch := runCmd(cmd).(vertexInfoMsg); !isFetch {
		t.Error("r should issue a real read")
	}
}
