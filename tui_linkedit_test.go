package main

import (
	"strings"
	"testing"

	"github.com/foliagecp/easyjson"
)

// Editing an existing link. Everything here is downstream of one rule: the
// form is built from what the SERVER says the edge holds, never from the
// model's link list, which never carries tags or a body.

// linkWithDetail puts the cursor on one link and pre-loads its details, the
// state the debounce reaches after the user rests on a row.
func linkWithDetail(t *testing.T, dl displayLink, tags []string, body easyjson.JSON) tuiModel {
	t.Helper()
	m := makeModel("hub/a", []displayLink{dl}, nil)
	m.focus = panelOut
	m.rCursor = 1 // 0 is the group header
	// These fixtures use a raw link between plain vertices, which the
	// high-level API cannot address at all — so the low-level one is armed.
	m.llMode = true
	m = m.applyLinkDetail(linkDetailMsg{
		key:    keyOf(dl),
		detail: linkDetail{loaded: true, tags: tags, body: body},
	})
	return m
}

func rawLink() displayLink {
	return displayLink{info: makeLinkInfo("hub/a", "l1", "hub/b", "rel"), isOut: true}
}

func numBody(k string, v int) easyjson.JSON {
	b := easyjson.NewJSONObject()
	b.SetByPath(k, easyjson.NewJSON(v))
	return b
}

// ── The subject ───────────────────────────────────────────────────────────────

func TestSubject_IsTheLinkUnderTheCursorAndTheVertexOtherwise(t *testing.T) {
	m := makeModel("hub/a", threeLinks(), nil)

	if got := m.subject().kind; got != subjVertex {
		t.Error("on the centre column the subject is the vertex")
	}
	m.focus = panelOut
	m.rCursor = 0 // group header — nothing link-shaped is selected
	if got := m.subject().kind; got != subjNone {
		t.Error("a group header is not an entity, and a side column cannot reach the vertex")
	}
	m.rCursor = 1
	if got := m.subject().kind; got != subjLink {
		t.Error("on a link row the subject is that link")
	}
}

func TestSubject_CentrePanelShowsTheSelectedLink(t *testing.T) {
	// The complaint was that a link's contents are nowhere on screen. They are
	// now the centre panel whenever one is selected — which is also what makes
	// `i` over it an obvious move rather than a binding to memorise.
	m := linkWithDetail(t, rawLink(), []string{"prod"}, numBody("weight", 7))

	out := m.renderCenterPanel()
	for _, want := range []string{"l1", "rel", "prod", "weight"} {
		if !strings.Contains(stripANSI(out), want) {
			t.Errorf("the centre panel does not show %q:\n%s", want, stripANSI(out))
		}
	}
}

func TestSubject_SaysItIsStillReadingRatherThanShowingBlanks(t *testing.T) {
	m := makeModel("hub/a", []displayLink{rawLink()}, nil)
	m.focus = panelOut
	m.rCursor = 1

	out := stripANSI(m.renderCenterPanel())
	if !strings.Contains(out, "reading the link") {
		t.Errorf("an unread link must say so, not render as empty:\n%s", out)
	}
}

// ── i / I / t ─────────────────────────────────────────────────────────────────

func TestLinkEdit_IOnALinkRowEditsTheLink(t *testing.T) {
	m := linkWithDetail(t, rawLink(), []string{"prod"}, numBody("w", 1))
	m = update(m, key("i"))

	if m.form == nil || m.form.kind != formLinkEdit {
		t.Fatalf("i over a link should open the link editor, got %v", m.form)
	}
}

func TestLinkEdit_TheVertexIsReachedByMovingToTheCentreColumn(t *testing.T) {
	fvi := makeVertexInfo("hub/a", nil, nil)
	m := linkWithDetail(t, rawLink(), nil, easyjson.NewJSONObject())
	m.fvi = &fvi

	// From the link panel, i edits the link.
	if got := update(m, key("i")); got.form == nil || got.form.kind != formLinkEdit {
		t.Fatalf("i over a link should edit the link, got %v", got.form)
	}
	// One press to the centre column, and the same key edits the vertex.
	m = update(m, key("h"))
	m = update(m, key("i"))
	if m.form == nil || m.form.kind != formBodyEdit {
		t.Fatalf("from the centre column i should edit the vertex, got %v", m.form)
	}
}

func TestLinkEdit_TLandsOnTagsAndILandsOnTheBody(t *testing.T) {
	m := linkWithDetail(t, rawLink(), []string{"prod"}, numBody("w", 1))

	viaT := update(m, key("t"))
	if viaT.form == nil || viaT.form.fields[viaT.form.cur].key != "tags" {
		t.Error("t must land on the tags row")
	}
	viaI := update(m, key("i"))
	if viaI.form == nil || viaI.form.fields[viaI.form.cur].key != "body" {
		t.Error("i must land on the body")
	}
}

// ── The data-loss fixes ───────────────────────────────────────────────────────

// TestLinkEdit_PrefillsFromTheServerNotTheModel is the regression test for the
// silent tag loss: the old form seeded its field from dl.info.tags, which the
// fast path leaves nil, so it opened blank over links that had tags.
func TestLinkEdit_PrefillsFromTheServerNotTheModel(t *testing.T) {
	m := linkWithDetail(t, rawLink(), []string{"prod", "eu"}, numBody("weight", 7))
	m = update(m, key("i"))

	if m.form == nil {
		t.Fatal("the editor should have opened")
	}
	if got := m.form.value("tags"); !strings.Contains(got, "prod") || !strings.Contains(got, "eu") {
		t.Errorf("tags = %q, want the tags the server reported", got)
	}
	if ed := m.form.jsonField(); ed == nil || !strings.Contains(ed.ta.Value(), "weight") {
		t.Error("the body must be the one that was read, not an empty object")
	}
}

// TestLinkEdit_ReplaceCarriesTheFetchedBody is the regression test for the
// second half of that bug: the tags form hardcoded an empty body, so ticking
// `replace` to clear tags also wiped a body no screen had ever shown.
func TestLinkEdit_ReplaceCarriesTheFetchedBody(t *testing.T) {
	var gotBody easyjson.JSON
	var gotReplace bool
	withOps(t, graphOps{
		linkUpdate: func(_, _ string, _ []string, body easyjson.JSON, replace bool) opResult {
			gotBody, gotReplace = body, replace
			return opResult{status: opApplied}
		},
	})

	m := linkWithDetail(t, rawLink(), []string{"prod"}, numBody("weight", 7))
	m = update(m, key("i"))
	m = update(m, tea_ctrlR()) // MERGE → REPLACE
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if !gotReplace {
		t.Fatal("ctrl+r should have switched to REPLACE")
	}
	if gotBody.GetByPath("weight").AsNumericDefault(0) != 7 {
		t.Errorf("REPLACE sent %s — it must carry the body the user was shown, "+
			"or it silently destroys it", gotBody.ToString())
	}
}

func TestLinkEdit_OneReplaceFlagBecauseTheServerHasOne(t *testing.T) {
	// The server applies `replace` to the body AND the tag index with a single
	// payload field. A separate checkbox next to the editor's own mode would be
	// two controls for one flag.
	m := linkWithDetail(t, rawLink(), []string{"prod"}, easyjson.NewJSONObject())
	m = update(m, key("i"))

	if _, ok := m.form.field("replace"); ok {
		t.Error("the editor's MERGE/REPLACE mode is the only replace control")
	}
}

func TestLinkEdit_TagsAndBodySubmitTogether(t *testing.T) {
	var gotTags []string
	var gotBody easyjson.JSON
	withOps(t, graphOps{
		linkUpdate: func(_, _ string, tags []string, body easyjson.JSON, _ bool) opResult {
			gotTags, gotBody = tags, body
			return opResult{status: opApplied}
		},
	})

	m := linkWithDetail(t, rawLink(), []string{"old"}, numBody("w", 1))
	m = update(m, key("t"))
	m = typeText(m, ",new")
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if strings.Join(gotTags, ",") != "old,new" {
		t.Errorf("tags = %v, want [old new]", gotTags)
	}
	if gotBody.GetByPath("w").AsNumericDefault(0) != 1 {
		t.Error("the body must travel with the tags — one call, one replace flag")
	}
}

// ── Rendering ─────────────────────────────────────────────────────────────────

func TestLinkEdit_RendersWithFocusOnTags(t *testing.T) {
	// renderFormFull used to look up the editor via the FOCUSED field, so a
	// form whose focus is on tags would paint "form has no editor".
	m := linkWithDetail(t, rawLink(), []string{"prod"}, numBody("w", 1))
	m = update(m, key("t"))

	out := stripANSI(m.renderFormFull())
	if strings.Contains(out, "form has no editor") {
		t.Fatalf("the editor must be found regardless of focus:\n%s", out)
	}
	if !strings.Contains(out, "prod") {
		t.Error("the tags row should be visible while editing a link")
	}
}

func TestLinkEdit_ShowsWhatItCannotChange(t *testing.T) {
	m := linkWithDetail(t, rawLink(), nil, easyjson.NewJSONObject())
	m = update(m, key("i"))

	out := stripANSI(m.renderFormFull())
	for _, want := range []string{"endpoints", "name", "l1", "via"} {
		if !strings.Contains(out, want) {
			t.Errorf("the form must state %q — it is how the edge is addressed:\n%s", want, out)
		}
	}
}

// ── Editor chords work from any field ─────────────────────────────────────────

func TestLinkEdit_CtrlRWorksFromTheTagsRow(t *testing.T) {
	m := linkWithDetail(t, rawLink(), []string{"prod"}, easyjson.NewJSONObject())
	m = update(m, key("t")) // focus on tags
	m = update(m, tea_ctrlR())

	if ed := m.form.jsonField(); ed == nil || !ed.replace {
		t.Error("ctrl+r must toggle the mode from any field, not only from the body")
	}
}

// ── Yank follows the subject ──────────────────────────────────────────────────

func TestYank_TakesTheLinkBodyWhenALinkIsSelected(t *testing.T) {
	m := linkWithDetail(t, rawLink(), nil, numBody("weight", 7))
	m = update(m, key("y"))

	if !strings.Contains(m.bodyRegister, "weight") {
		t.Errorf("yank register = %q, want the selected link's body", m.bodyRegister)
	}
}

// ── Deleting an edge routes through the API that owns it ──────────────────────

// typeLinks puts the model on a type vertex, so the near kind is vkType.
func onType(id string, extra ...displayLink) tuiModel {
	links := append([]displayLink{
		{info: makeLinkInfo(hubID("types"), stripDomain(id), id, ltInstanceOf), isOut: false},
	}, extra...)
	m := makeModel(id, links, nil)
	m.focus = panelOut
	return m
}

// TestDeleteLink_TypesLinkCascades is the fix for a real corruption: the TUI
// removed a schema link with the raw API, so the declaration vanished while
// every instance kept an edge the schema no longer permitted. The shell's
// `cmdb typeslink delete` has always cascaded — the same action meant two
// different things depending on where you ran it.
func TestDeleteLink_TypesLinkCascadesBehindATypedName(t *testing.T) {
	var gotFrom, gotTo string
	withOps(t, graphOps{
		typesLinkDelete: func(from, to string) opResult {
			gotFrom, gotTo = from, to
			return opResult{status: opApplied}
		},
	})

	schema := displayLink{info: makeLinkInfo("hub/srv", "rack", "hub/rack", ltInstanceOf), isOut: true}
	m := onType("hub/srv", schema)
	m.rCursor = 1 // the schema edge, under its group header

	m = update(m, key("d"))
	if m.form == nil {
		t.Fatal("d should stage the delete")
	}
	if m.form.confirmWord != "srv" {
		t.Errorf("confirmWord = %q — a cascade must be confirmed by name, not y/n", m.form.confirmWord)
	}
	if !strings.Contains(m.form.title, "every object") {
		t.Errorf("title = %q, want it to state the blast radius", m.form.title)
	}

	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	_, cmd := updateCmd(m, keyEnter())
	runCmd(cmd)

	if gotFrom != "hub/srv" || gotTo != "hub/rack" {
		t.Errorf("typesLinkDelete(%q,%q), want (hub/srv, hub/rack)", gotFrom, gotTo)
	}
}

func TestDeleteLink_SubTypeEdgeRemovesTheRelation(t *testing.T) {
	// The n → s menu can declare a sub-type; removing one had no home in the
	// TUI at all. Its home is the edge you can see.
	var gotBase, gotChild string
	withOps(t, graphOps{
		subTypeRemove: func(b, c string) opResult {
			gotBase, gotChild = b, c
			return opResult{status: opApplied}
		},
	})

	sub := displayLink{info: makeLinkInfo("hub/hw", "child_srv", "hub/srv", ltSubType), isOut: true}
	m := onType("hub/hw", sub)
	m.rCursor = 1

	m = update(m, key("d"))
	if m.form == nil || m.form.confirmWord != "srv" {
		t.Fatalf("removing a sub-type must be confirmed by name, got %v", m.form)
	}
	for _, r := range "srv" {
		m = update(m, key(string(r)))
	}
	_, cmd := updateCmd(m, keyEnter())
	runCmd(cmd)

	if gotBase != "hub/hw" || gotChild != "hub/srv" {
		t.Errorf("subTypeRemove(%q,%q), want (hub/hw, hub/srv)", gotBase, gotChild)
	}
}

func TestDeleteLink_OrdinaryEdgeStaysASingleKeyConfirmation(t *testing.T) {
	var called bool
	withOps(t, graphOps{
		linkDelete: func(string, string) opResult { called = true; return opResult{status: opApplied} },
	})

	m := makeModel("hub/a", []displayLink{rawLink()}, nil)
	m = update(m, key("x")) // a raw link is a low-level entity
	m.focus = panelOut
	m.rCursor = 1

	m = update(m, key("d"))
	if m.form == nil || m.form.confirmWord != "" {
		t.Fatal("an ordinary link deletes on y/n — escalation is for cascades")
	}
	_, cmd := updateCmd(m, key("y"))
	runCmd(cmd)
	if !called {
		t.Error("an ordinary edge goes through the raw API")
	}
}

func TestTier_StructuralEdgesNeedNoSecondRoundTrip(t *testing.T) {
	// The tier must not depend on whether the user happened to walk to the far
	// endpoint, or a cascade would fire or not depending on where they had
	// been. The edge type settles it for every structural edge.
	schema := displayLink{info: makeLinkInfo("hub/srv", "rack", "hub/rack", ltInstanceOf), isOut: true}
	m := onType("hub/srv", schema)
	if got := m.tierOfSubjectLink(schema); got != tierTypesLink {
		t.Errorf("tier = %v, want tierTypesLink derived from the edge alone", got)
	}
}

// ── Claimed super-types on a new object-link ──────────────────────────────────

// objectAtWithType stands on an object of the named type.
func objectPair(t *testing.T) tuiModel {
	t.Helper()
	m := makeModel("hub/rack-a", []displayLink{
		{info: makeLinkInfo("hub/rack-a", instanceOfLinkName, "hub/rack", ltInstanceOf), isOut: true},
	}, nil)
	m.linking = &pendingLink{fromID: "hub/srv-1", kind: vkObject, typeName: "srv"}
	return m
}

func TestLinkCreate_ClaimsDefaultToTheRealTypesAndStayOrdinary(t *testing.T) {
	var calledPlain bool
	withOps(t, graphOps{
		objectsLinkCreate: func(string, string, string, []string, easyjson.JSON) opResult {
			calledPlain = true
			return opResult{status: opApplied}
		},
	})

	m := objectPair(t)
	m = update(m, key("L"))
	if m.form == nil {
		t.Fatal("committing a pending link should open the form")
	}
	if got := m.form.value("fromclaim"); got != "srv" {
		t.Errorf("from-claim prefill = %q, want the object's real type", got)
	}
	if got := m.form.value("toclaim"); got != "rack" {
		t.Errorf("to-claim prefill = %q, want the target's real type", got)
	}

	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)
	if !calledPlain {
		t.Error("unchanged claims mean an ordinary objects-link")
	}
}

func TestLinkCreate_ChangingAClaimLinksUnderTheSuperType(t *testing.T) {
	var gotFromClaim, gotToClaim string
	withOps(t, graphOps{
		superLinkCreate: func(_, _, fc, tc, _ string, _ []string, _ easyjson.JSON) opResult {
			gotFromClaim, gotToClaim = fc, tc
			return opResult{status: opApplied}
		},
	})

	m := objectPair(t)
	m = update(m, key("L"))
	m = focusField(m, "fromclaim")
	for range "srv" {
		m = update(m, keyBackspace())
	}
	m = typeText(m, "machine")

	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if gotFromClaim != "machine" || gotToClaim != "rack" {
		t.Errorf("superLinkCreate claims = (%q,%q), want (machine, rack)", gotFromClaim, gotToClaim)
	}
}

func TestLinkCreate_CarriesABody(t *testing.T) {
	// Every create path used to send an empty object unconditionally, so a link
	// could not be given a body at all without dropping to the shell.
	var gotBody easyjson.JSON
	withOps(t, graphOps{
		linkCreate: func(_, _, _, _ string, _ []string, body easyjson.JSON, _ bool) opResult {
			gotBody = body
			return opResult{status: opApplied}
		},
	})

	m := makeModel("hub/b", nil, nil)
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	m = update(m, key("L"))
	m = focusField(m, "type")
	m = typeText(m, "rel")
	m = focusField(m, "body")
	m = typeText(m, `{"weight":3}`)

	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if gotBody.GetByPath("weight").AsNumericDefault(0) != 3 {
		t.Errorf("body = %s, want the one that was typed", gotBody.ToString())
	}
}

func TestLinkCreate_RejectsABodyThatIsNotAnObject(t *testing.T) {
	withOps(t, graphOps{})

	m := makeModel("hub/b", nil, nil)
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
	m = update(m, key("L"))
	m = focusField(m, "type")
	m = typeText(m, "rel")
	m = focusField(m, "body")
	m = typeText(m, "not json")

	_, cmd := updateCmd(m, tea_ctrlS())
	msg := runCmd(cmd).(mutationResultMsg)
	if msg.res.status != opFailed {
		t.Error("an unparseable body must fail loudly, not be silently dropped")
	}
}

// ── Templates ─────────────────────────────────────────────────────────────────

// TestTemplate_YankThenPaste closes a loop that was open for the whole life of
// the feature: `y` wrote to a register nothing read, and the help advertised
// ctrl+t, which nothing handled.
func TestTemplate_YankThenPaste(t *testing.T) {
	m := linkWithDetail(t, rawLink(), nil, numBody("weight", 7))
	m = update(m, key("y"))
	if m.bodyRegister == "" {
		t.Fatal("y should have filled the register")
	}

	fvi := makeVertexInfo("hub/a", nil, nil)
	m.fvi = &fvi
	m = update(m, key("h")) // to the centre column
	m = update(m, key("i")) // edit the vertex body
	if m.form == nil {
		t.Fatal("i on the centre column should open the body editor")
	}
	if strings.Contains(m.form.jsonField().ta.Value(), "weight") {
		t.Fatal("fixture: the vertex body should not already contain the yank")
	}

	m = update(m, tea_ctrlT())
	if !strings.Contains(m.form.jsonField().ta.Value(), "weight") {
		t.Error("ctrl+t should paste the yanked body — it has been advertised and absent")
	}
}

func TestTemplate_NotAdvertisedWithNothingToPaste(t *testing.T) {
	fvi := makeVertexInfo("hub/a", nil, nil)
	m := makeModel("hub/a", nil, &fvi)
	m = update(m, key("i"))

	if strings.Contains(stripANSI(m.renderFormFull()), "ctrl+t") {
		t.Error("a key that would do nothing must not be offered")
	}
}
