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

func TestDelete_DOnGroupHeaderTargetsTheVertex(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	m.rCursor = 0 // the group header, not a link

	m = update(m, key("d"))

	if m.form == nil || m.form.kind != formDeleteVertex {
		t.Fatalf("d off a link row should target the vertex, got %+v", m.form)
	}
}

func TestDelete_ShiftDAlwaysTargetsTheVertex(t *testing.T) {
	m := makeModel("hub/x", threeLinks(), nil)
	m.rCursor = 1 // sitting on a link

	m = update(m, key("D"))

	if m.form == nil || m.form.kind != formDeleteVertex {
		t.Fatalf("D should target the vertex even from a link row, got %+v", m.form)
	}
}

// ── Tiers ─────────────────────────────────────────────────────────────────────

func TestDelete_BuiltInIsRefusedWithoutAForm(t *testing.T) {
	m := makeModel("hub/types", nil, nil)
	m = update(m, key("D"))

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
	m = update(m, key("D"))
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
	m = update(m, key("D"))
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
	m = update(m, key("D"))
	if m.form == nil {
		t.Fatal("D should open a confirmation")
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
	m = update(m, key("D"))
	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	_, cmd = updateCmd(m, keyEnter())
	runCmd(cmd)
	if !called {
		t.Error("typing the exact name should confirm the delete")
	}
}

func TestDelete_TypeClearsTheWholeCache(t *testing.T) {
	withOps(t, graphOps{
		typeDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/srv", typeVertexLinks(), nil)
	m.cache["hub/unrelated"] = cachedVertex{}
	m = update(m, key("D"))
	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	_, cmd := updateCmd(m, keyEnter())
	msg := runCmd(cmd)

	mr, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %T", msg)
	}
	if !mr.clearAll {
		t.Error("a type delete cascades into unbounded object deletes; the cache must be cleared wholesale")
	}
}

// ── Consequences of a successful delete ───────────────────────────────────────

func TestDelete_VertexInvalidatesNeighbours(t *testing.T) {
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", threeLinks(), nil)
	m = update(m, key("D"))
	_, cmd := updateCmd(m, key("y"))
	msg := runCmd(cmd).(mutationResultMsg)

	joined := strings.Join(msg.invalidate, ",")
	for _, want := range []string{"hub/x", "c1", "c2", "c3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("invalidate = %v, want it to include %q", msg.invalidate, want)
		}
	}
}

func TestDelete_NavigatesAwayFromTheDeletedVertex(t *testing.T) {
	// gWalkTo persists the current id on every load. Staying on a deleted
	// vertex would leave a dead id on disk and break the NEXT launch.
	withOps(t, graphOps{
		vertexDelete: func(string) opResult { return opResult{status: opApplied} },
	})

	m := makeModel("hub/x", nil, nil)
	m.history = []string{"hub/parent"}

	m = update(m, key("D"))
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

	m = update(m, key("D"))
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
	m = update(m, key("D"))
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

	m = update(m, key("D"))
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
	m = update(m, key("D"))
	if m.form != nil {
		t.Error("a delete must not be staged against a vertex that is still loading")
	}
}

func TestDelete_ConfirmationRendersInTheStatusBar(t *testing.T) {
	m := makeModel("hub/srv-1", objectVertexLinks(), nil)
	m = update(m, key("D"))

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
	m = update(m, key("D"))
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
	m = update(m, key("D"))
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
