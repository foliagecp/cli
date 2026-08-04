package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// L is the link key, and what it does follows the subject — the same rule as
// every other key. Three states, three actions, each named in the status bar.

func linkModel(t *testing.T) tuiModel {
	t.Helper()
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := makeModel("hub/x", threeLinks(), &fvi)
	m.width = 140
	return m
}

func TestL_StartsALinkFromTheCentreColumn(t *testing.T) {
	m := linkModel(t)
	if m.linkKeyAction() != "start link" {
		t.Fatalf("action = %q, want start link", m.linkKeyAction())
	}
	m = update(m, key("L"))
	if m.linking == nil || m.linking.fromID != "hub/x" {
		t.Fatal("L on the vertex should start a link from it")
	}
	if !strings.Contains(stripANSI(m.renderStatus()), "commit link") {
		t.Error("with a link pending the hint must say what L will do next")
	}
}

// TestL_EditsTheSelectedLink is the missing half: selecting a link and pressing
// the link key did nothing at all, which is why L felt like it belonged to
// creation only.
func TestL_EditsTheSelectedLink(t *testing.T) {
	m := makeModel("hub/a", []displayLink{rawLink()}, nil)
	m.llMode = true // a raw link between plain vertices
	m.focus = panelOut
	m.rCursor = 1
	m = m.applyLinkDetail(linkDetailMsg{
		key:    keyOf(rawLink()),
		detail: linkDetail{loaded: true, body: easyjson.NewJSONObject()},
	})

	if m.linkKeyAction() != "edit link" {
		t.Fatalf("action = %q, want edit link", m.linkKeyAction())
	}
	m = update(m, key("L"))
	if m.form == nil || m.form.kind != formLinkEdit {
		t.Fatalf("L over a link should edit it, got %v", m.form)
	}
	if m.linking != nil {
		t.Error("editing a link must not start a new one")
	}
}

// TestL_CommitWinsOverEditWhileSomethingIsPending. The banner has been
// promising a commit across every keystroke since it appeared; the cursor
// happening to rest on a link row must not change what the screen said.
func TestL_CommitWinsOverEditWhileSomethingIsPending(t *testing.T) {
	m := makeModel("hub/b", threeLinks(), nil)
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	m.focus = panelOut
	m.rCursor = 1

	if m.linkKeyAction() != "commit link" {
		t.Fatalf("action = %q, want commit link", m.linkKeyAction())
	}
	m = update(m, key("L"))
	if m.form == nil || m.form.kind != formLinkCreate {
		t.Fatalf("L should commit the pending link, got %v", m.form)
	}
}

// TestL_CommitTakesTheBannerDown. A successful commit used to clear nothing,
// so the banner sat there promising a commit that had already happened — and
// every later L meant "commit again" instead of whatever it should have meant.
func TestL_CommitTakesTheBannerDown(t *testing.T) {
	withOps(t, graphOps{
		linkCreate: func(_, _, _, _ string, _ []string, _ easyjson.JSON, _ bool) opResult {
			return opResult{status: opApplied}
		},
	})

	m := makeModel("hub/b", nil, nil)
	m = update(m, key("x")) // a raw link needs the low-level API
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	m = update(m, key("L"))
	m = focusField(m, "type")
	m = typeText(m, "rel")
	_, cmd := updateCmd(m, tea_ctrlS())

	m = update(m, runCmd(cmd))
	if m.linking != nil {
		t.Error("the banner must come down once the link exists")
	}
	if strings.Contains(stripANSI(m.renderStatus()), "LINK PENDING") {
		t.Error("the pending banner is still on screen")
	}
}

func TestL_AFailedCommitKeepsThePendingLink(t *testing.T) {
	withOps(t, graphOps{
		linkCreate: func(_, _, _, _ string, _ []string, _ easyjson.JSON, _ bool) opResult {
			return opResult{status: opFailed, details: "already exists"}
		},
	})

	m := makeModel("hub/b", nil, nil)
	m = update(m, key("x"))
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	m = update(m, key("L"))
	m = focusField(m, "type")
	m = typeText(m, "rel")
	_, cmd := updateCmd(m, tea_ctrlS())

	m = update(m, runCmd(cmd))
	if m.linking == nil {
		t.Error("a failed commit should leave the pending link so it can be retried")
	}
}

// ── The tier a link is edited through ─────────────────────────────────────────

// objectsLinkBetween builds the state the browser is in when standing on an
// object with a schema-typed edge to another object: the edge type is the
// SCHEMA's own, so nothing about the edge reveals what is on the other end.
func objectsLinkBetween(t *testing.T) (tuiModel, displayLink) {
	t.Helper()
	edge := displayLink{info: makeLinkInfo("hub/srv-1", "rack-a", "hub/rack-a", "mounted_in"), isOut: true}
	m := makeModel("hub/srv-1", []displayLink{
		{info: makeLinkInfo("hub/srv-1", instanceOfLinkName, "hub/srv", ltInstanceOf), isOut: true},
		edge,
	}, nil)
	m.focus = panelOut
	return m, edge
}

// TestTier_AnObjectsLinkIsEditableInHighLevelMode is the regression test for
// "I cannot edit links". An objects-link carries the schema's link type, not a
// structural one, so the edge alone cannot say what it connects — and guessing
// "plain" sent every one of them to the raw API, which high-level mode then
// refused. The only editable links left were the structural ones.
func TestTier_AnObjectsLinkIsEditableInHighLevelMode(t *testing.T) {
	m, edge := objectsLinkBetween(t)

	// Before the far endpoint is known the tier is provisional…
	m = m.applyLinkDetail(linkDetailMsg{
		key:    keyOf(edge),
		detail: linkDetail{loaded: true, body: easyjson.NewJSONObject(), farKind: vkObject},
	})
	if got := m.tierOfSubjectLink(edge); got != tierObjectsLink {
		t.Fatalf("tier = %v, want tierObjectsLink once the far end is known", got)
	}
	if refusal := crudRefusesLink(false, tierObjectsLink); refusal != "" {
		t.Errorf("the high-level API owns objects-links: %q", refusal)
	}
}

func TestTier_HighLevelStillRefusesEdgesItDoesNotOwn(t *testing.T) {
	for _, c := range []struct {
		name string
		tier linkTier
		want bool // refused?
	}{
		{"raw link between plain vertices", tierRawLink, true},
		{"types-link", tierTypesLink, false},
		{"objects-link", tierObjectsLink, false},
		{"claimed-type object link", tierSuperLink, false},
		{"sub-type edge", tierSubType, false},
	} {
		refusal := crudRefusesLink(false, c.tier)
		if (refusal != "") != c.want {
			t.Errorf("%s: refusal = %q, want refused=%v", c.name, refusal, c.want)
		}
	}
	// With the low-level API armed nothing is refused — that is the point of it.
	for _, tr := range []linkTier{tierRawLink, tierTypesLink, tierObjectsLink} {
		if refusal := crudRefusesLink(true, tr); refusal != "" {
			t.Errorf("low-level mode refused %v: %q", tr, refusal)
		}
	}
}

// TestTier_ResolvesTheFarEndpointWhenTheEdgeCannot pins the extra read: it
// happens for a user-typed edge, and not for a structural one.
func TestTier_ResolvesTheFarEndpointWhenTheEdgeCannot(t *testing.T) {
	structural := displayLink{info: makeLinkInfo("hub/srv", "rack", "hub/rack", ltInstanceOf), isOut: true}
	if _, settled := inferFarKind(structural); !settled {
		t.Error("a structural edge says what is on the other end")
	}
	userTyped := displayLink{info: makeLinkInfo("hub/srv-1", "rack-a", "hub/rack-a", "mounted_in"), isOut: true}
	if _, settled := inferFarKind(userTyped); settled {
		t.Error("a user-typed edge cannot say what it connects — it must be read")
	}
}

// ── Relating two types ────────────────────────────────────────────────────────

// twoTypes stands on a type with another one pending, which is the state L
// commits from.
func twoTypes(t *testing.T) tuiModel {
	t.Helper()
	m := makeModel("hub/srv", []displayLink{
		{info: makeLinkInfo(hubID("types"), "srv", "hub/srv", ltInstanceOf), isOut: false},
	}, nil)
	m.linking = &pendingLink{fromID: "hub/hw", kind: vkType}
	return update(m, key("L"))
}

// TestTypeLink_AsksWhichRelation. Two types can be related in two entirely
// different ways — a types-link is a SCHEMA declaration that permits
// object-links between instances, a sub-type is INHERITANCE — and the form
// used to silently assume the first. Nothing about walking from one type to
// another says which was meant.
func TestTypeLink_AsksWhichRelation(t *testing.T) {
	m := twoTypes(t)
	if m.form == nil {
		t.Fatal("L should open the link form between two types")
	}
	fl, ok := m.form.field("relation")
	if !ok {
		t.Fatal("the form must ask which relation this is")
	}
	labels := ""
	for _, o := range fl.options {
		labels += o.label + " "
	}
	if !strings.Contains(labels, "types-link") || !strings.Contains(labels, "sub-type") {
		t.Errorf("options = %q, want both relations offered", labels)
	}
}

func TestTypeLink_SubTypeSubmitsInheritance(t *testing.T) {
	var base, child string
	withOps(t, graphOps{
		subTypeSet: func(b, c string) opResult { base, child = b, c; return opResult{status: opApplied} },
	})

	m := twoTypes(t)
	m = focusField(m, "relation")
	m = update(m, tea.KeyMsg{Type: tea.KeyRight}) // types-link → sub-type
	if got := m.form.value("relation"); got != relSubType {
		t.Fatalf("relation = %q, want %q", got, relSubType)
	}
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if base != "hub/hw" || child != "hub/srv" {
		t.Errorf("subTypeSet(%q,%q), want (hub/hw, hub/srv)", base, child)
	}
}

func TestTypeLink_TypesLinkStillSubmitsTheSchema(t *testing.T) {
	var gotOLT string
	withOps(t, graphOps{
		typesLinkCreate: func(_, _, olt string, _ []string, _ easyjson.JSON) opResult {
			gotOLT = olt
			return opResult{status: opApplied}
		},
	})

	m := twoTypes(t)
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if gotOLT != "srv" {
		t.Errorf("objectLinkType = %q, want the prefilled target name", gotOLT)
	}
}

// TestTypeLink_HidesWhatTheChosenRelationIgnores. A sub-type carries no
// object-link type, no tags and no body — showing them would invite input the
// operation has nowhere to put.
func TestTypeLink_HidesWhatTheChosenRelationIgnores(t *testing.T) {
	m := twoTypes(t)
	f := *m.form
	for _, key := range []string{"olt", "tags", "body"} {
		fl, ok := f.field(key)
		if !ok {
			t.Fatalf("%s should exist for a types-link", key)
		}
		if !f.visible(fl) {
			t.Errorf("%s should be shown while the relation is a types-link", key)
		}
	}

	m = focusField(m, "relation")
	m = update(m, tea.KeyMsg{Type: tea.KeyRight})
	f = *m.form
	for _, key := range []string{"olt", "tags", "body"} {
		fl, _ := f.field(key)
		if f.visible(fl) {
			t.Errorf("%s applies to a types-link, not to a sub-type", key)
		}
	}
	// And the form is submittable even though the hidden `olt` is required.
	if !f.ok() {
		t.Error("a required field the current choice hides must not block submission")
	}
}
