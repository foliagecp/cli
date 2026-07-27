package main

import (
	"strings"
	"testing"

	"github.com/foliagecp/easyjson"
)

func typeAt(id string) []displayLink {
	return []displayLink{
		{info: makeLinkInfo(NatsHubDomain+"/types", stripDomain(id), id, "__type"), isOut: false},
	}
}

func objectAt(id, typeID string) []displayLink {
	return []displayLink{
		{info: makeLinkInfo(NatsHubDomain+"/objects", stripDomain(id), id, "__object"), isOut: false},
		{info: makeLinkInfo(id, "type", typeID, "__type"), isOut: true},
	}
}

// focusField moves the form cursor onto a named field.
func focusField(m tuiModel, key string) tuiModel {
	for i, fl := range m.form.fields {
		if fl.key == key {
			m.form.cur = i
			return m
		}
	}
	return m
}

// typeText types a string into the focused field, one key at a time.
func typeText(m tuiModel, s string) tuiModel {
	for _, r := range s {
		m = update(m, key(string(r)))
	}
	return m
}

// ── Anchor ────────────────────────────────────────────────────────────────────

func TestAnchor_SetToggleAndShow(t *testing.T) {
	m := makeModel("hub/a", nil, nil)

	m = update(m, key("a"))
	if m.anchor == nil || m.anchor.id != "hub/a" {
		t.Fatalf("a should anchor the current vertex, got %+v", m.anchor)
	}
	if !strings.Contains(m.renderBreadcrumbs(), "⚓") {
		t.Error("the anchor should be visible in the breadcrumb row")
	}
	if m.breadcrumbH() != 1 {
		t.Error("the breadcrumb row must be reserved when an anchor is set")
	}

	m = update(m, key("a"))
	if m.anchor != nil {
		t.Error("a on the same vertex should clear the anchor")
	}
}

func TestAnchor_ClearedByR(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("a"))
	m = update(m, key("R"))
	if m.anchor != nil {
		t.Error("R resets navigation state and should clear the anchor too")
	}
}

func TestAnchor_SurvivesNavigationAndFormCancel(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("a"))

	// The anchor is data, not a mode: ordinary keys keep working.
	m = update(m, key("v"))
	m = update(m, key("j"))
	if m.anchor == nil {
		t.Fatal("navigation must not clear the anchor")
	}

	m.currentID = "hub/b"
	m = update(m, key("L"))
	if m.form == nil {
		t.Fatal("L should open the link form")
	}
	m = update(m, keyEsc())
	if m.anchor == nil {
		t.Error("cancelling the form should keep the anchor — the usual reason to cancel is a typo")
	}
}

// ── Link creation ─────────────────────────────────────────────────────────────

func TestLink_LWithoutAnchorExplains(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))
	if m.form != nil {
		t.Error("L without an anchor should not open a form")
	}
	if !strings.Contains(m.queryResult, "anchor") {
		t.Errorf("toast = %q, want it to say an anchor is needed", m.queryResult)
	}
}

func TestLink_TierFollowsTheEndpoints(t *testing.T) {
	cases := []struct {
		name     string
		from, to vertexKind
		llMode   bool
		want     linkTier
	}{
		{"type to type is a schema declaration", vkType, vkType, false, tierTypesLink},
		{"object to object is an instance link", vkObject, vkObject, false, tierObjectsLink},
		{"type to object has no high-level meaning", vkType, vkObject, false, tierRawLink},
		{"a plain endpoint forces the raw API", vkObject, vkPlain, false, tierRawLink},
		{"the low-level toggle overrides everything", vkObject, vkObject, true, tierRawLink},
	}
	for _, c := range cases {
		if got := linkTierFor(c.from, c.to, c.llMode); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLink_TypesLinkFormAsksForTheObjectLinkType(t *testing.T) {
	m := makeModel("hub/rack", typeAt("hub/rack"), nil)
	m.anchor = &anchorState{id: "hub/srv", kind: vkType}

	m = update(m, key("L"))
	if m.form == nil {
		t.Fatal("L should open the link form")
	}
	if !strings.Contains(m.form.title, "types-link") {
		t.Errorf("title = %q, want it to name the API tier", m.form.title)
	}
	if _, ok := m.form.field("olt"); !ok {
		t.Error("a types-link declares the object-link type; the field must be present")
	}
}

func TestLink_ObjectsLinkTypeIsDerivedNotTyped(t *testing.T) {
	m := makeModel("hub/rack-A", objectAt("hub/rack-A", "hub/rack"), nil)
	m.anchor = &anchorState{id: "hub/srv-1", kind: vkObject, typeName: "srv"}

	m = update(m, key("L"))
	fl, ok := m.form.field("ltype")
	if !ok {
		t.Fatal("the derived link type should be shown")
	}
	if fl.kind != fieldStatic {
		t.Error("the object-link type comes from the schema and must not be editable")
	}
}

func TestLink_RawFormPrefillsNameAndType(t *testing.T) {
	links := []displayLink{
		{info: makeLinkInfo("hub/a", "existing", "hub/z", "rel"), isOut: true},
	}
	m := makeModel("hub/b", links, nil)
	m.anchor = &anchorState{id: "hub/a", kind: vkPlain}

	m = update(m, key("L"))
	if got := m.form.value("name"); got != "b" {
		t.Errorf("name prefill = %q, want the target id (the server's own default)", got)
	}
	if got := m.form.value("type"); got != "rel" {
		t.Errorf("type prefill = %q, want the type already used from this source", got)
	}
}

func TestLink_DirectionSwaps(t *testing.T) {
	m := makeModel("hub/b", nil, nil)
	m.anchor = &anchorState{id: "hub/a", kind: vkPlain}
	m = update(m, key("L"))

	m.form.cur = 0 // the endpoints row
	f, _ := m.form.handleKey("right")
	if f.ctx.fromID != "hub/b" || f.ctx.toID != "hub/a" {
		t.Errorf("→ should swap the direction, got %s → %s", f.ctx.fromID, f.ctx.toID)
	}
}

func TestLink_SubmitCallsTheRightAPI(t *testing.T) {
	var called string
	withOps(t, graphOps{
		linkCreate: func(from, to, name, tp string, tags []string, b easyjson.JSON, force bool) opResult {
			called = "raw:" + from + "->" + to + ":" + name + ":" + tp
			return opResult{status: opApplied}
		},
		typesLinkCreate: func(from, to, olt string, tags []string, b easyjson.JSON) opResult {
			called = "types:" + from + "->" + to + ":" + olt
			return opResult{status: opApplied}
		},
	})

	// Raw link between plain vertices.
	m := makeModel("hub/b", nil, nil)
	m.anchor = &anchorState{id: "hub/a", kind: vkPlain}
	m = update(m, key("L"))
	m = focusField(m, "type")
	m = typeText(m, "rel")
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)
	if !strings.HasPrefix(called, "raw:hub/a->hub/b") {
		t.Errorf("raw link call = %q", called)
	}

	// Types-link between two types.
	called = ""
	m = makeModel("hub/rack", typeAt("hub/rack"), nil)
	m.anchor = &anchorState{id: "hub/srv", kind: vkType}
	m = update(m, key("L"))
	_, cmd = updateCmd(m, tea_ctrlS())
	runCmd(cmd)
	if !strings.HasPrefix(called, "types:hub/srv->hub/rack") {
		t.Errorf("types-link call = %q", called)
	}
}

func TestLink_ForceFlagReachesTheOperation(t *testing.T) {
	var gotForce bool
	withOps(t, graphOps{
		linkCreate: func(_, _, _, _ string, _ []string, _ easyjson.JSON, force bool) opResult {
			gotForce = force
			return opResult{status: opApplied}
		},
	})

	m := makeModel("hub/b", nil, nil)
	m.anchor = &anchorState{id: "hub/a", kind: vkPlain}
	m = update(m, key("L"))

	m = focusField(m, "type")
	m = typeText(m, "rel")
	m = focusField(m, "force")
	m = update(m, key(" "))
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if !gotForce {
		t.Error("the force toggle did not reach the operation layer")
	}
}

// ── Create menu ───────────────────────────────────────────────────────────────

func TestCreateMenu_OffersObjectOnAType(t *testing.T) {
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))

	if m.form == nil || m.form.kind != formCreateMenu {
		t.Fatal("n should open the create menu")
	}
	out := m.renderFormCenter(80, 20)
	if !strings.Contains(out, "new object of srv") {
		t.Errorf("standing on a type, the menu should offer its objects:\n%s", out)
	}
}

func TestCreateMenu_LinkEntryExplainsTheMissingAnchor(t *testing.T) {
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))

	out := m.renderFormCenter(80, 20)
	if !strings.Contains(out, "press a to anchor a source vertex first") {
		t.Errorf("an entry needing an anchor should say so rather than vanish:\n%s", out)
	}

	// Choosing it anyway explains rather than silently doing nothing.
	m = update(m, key("l"))
	if m.form.kind != formCreateMenu {
		t.Error("the menu should stay open")
	}
	if m.form.err == "" {
		t.Error("choosing a disabled entry should explain why it is unavailable")
	}
}

func TestCreateMenu_OpensTheChosenForm(t *testing.T) {
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))
	m = update(m, key("t"))

	if m.form == nil || m.form.kind != formTypeCreate {
		t.Fatalf("t should open the type form, got %+v", m.form)
	}
}

func TestCreateMenu_EscCloses(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("n"))
	m = update(m, keyEsc())
	if m.form != nil {
		t.Error("esc should close the menu")
	}
}

// ── Object / type / vertex creation ───────────────────────────────────────────

func TestObjectCreate_TypeIsFixedWhenStandingOnIt(t *testing.T) {
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))
	m = update(m, key("o"))

	fl, ok := m.form.field("type")
	if !ok {
		t.Fatal("the object form needs a type field")
	}
	if fl.kind != fieldStatic {
		t.Error("standing on the type, it is settled and should not be editable")
	}
	if fl.static != "srv" {
		t.Errorf("type = %q, want srv", fl.static)
	}
}

func TestObjectCreate_SubmitsIDAndType(t *testing.T) {
	var gotID, gotType string
	withOps(t, graphOps{
		objectCreate: func(id, tp string, _ easyjson.JSON) opResult {
			gotID, gotType = id, tp
			return opResult{status: opApplied}
		},
	})

	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))
	m = update(m, key("o"))
	m = typeText(m, "srv-9")
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if gotID != "srv-9" || gotType != "srv" {
		t.Errorf("objectCreate(%q,%q), want (srv-9, srv)", gotID, gotType)
	}
}

func TestCreate_RejectsAnInvalidID(t *testing.T) {
	withOps(t, graphOps{}) // any call panics
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("t"))
	m = typeText(m, "bad.name") // a dot is the KV separator

	if m.form.ok() {
		t.Fatal("an id with a dot must not be submittable")
	}
	_, cmd := updateCmd(m, tea_ctrlS())
	if runCmd(cmd) != nil {
		t.Error("submission should be blocked while the id is invalid")
	}
	if !strings.Contains(m.renderFormCenter(80, 20), "dots are not allowed") {
		t.Error("the form should say what is wrong with the id")
	}
}

func TestTypeCreate_PreviewNamesTheHub(t *testing.T) {
	// Type operations are redirected to the hub wherever the user is browsing,
	// so the preview must not promise the local domain.
	m := makeModel("leaf/x", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("t"))

	fl, _ := m.form.field("name")
	if !strings.Contains(fl.hint, NatsHubDomain+"/") {
		t.Errorf("hint = %q, want it to name the hub domain", fl.hint)
	}
}

// ── Tags ──────────────────────────────────────────────────────────────────────

func TestTags_NeedsALinkUnderTheCursor(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("t"))
	if m.form != nil {
		t.Error("t with no link selected should not open a form")
	}
}

func TestTags_PrefilledAndSubmitted(t *testing.T) {
	var gotTags []string
	var gotReplace bool
	withOps(t, graphOps{
		linkUpdate: func(_, _ string, tags []string, _ easyjson.JSON, replace bool) opResult {
			gotTags, gotReplace = tags, replace
			return opResult{status: opApplied}
		},
	})

	links := threeLinks()
	links[0].info.tags = []string{"old"}
	m := makeModel("root", links, nil)
	m.rCursor = 1

	m = update(m, key("t"))
	if m.form == nil {
		t.Fatal("t should open the tag form")
	}
	if got := m.form.value("tags"); !strings.Contains(got, "old") {
		t.Errorf("tags prefill = %q, want the existing tags", got)
	}

	m = typeText(m, ",new")
	_, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if strings.Join(gotTags, ",") != "old,new" {
		t.Errorf("tags = %v, want [old new]", gotTags)
	}
	if gotReplace {
		t.Error("replace should default to off")
	}
}

func TestTags_ReplaceIsAdvertisedAsTheWayToClear(t *testing.T) {
	// Every write wrapper drops empty tags, so clearing them under a merge
	// silently does nothing. The form must say so.
	links := threeLinks()
	links[0].info.tags = []string{"old"}
	m := makeModel("root", links, nil)
	m.rCursor = 1
	m = update(m, key("t"))

	fl, ok := m.form.field("replace")
	if !ok {
		t.Fatal("the tag form needs a replace toggle")
	}
	if !strings.Contains(fl.hint, "clear") {
		t.Errorf("hint = %q, want it to explain that clearing needs replace", fl.hint)
	}
}

// ── Yank ──────────────────────────────────────────────────────────────────────

func TestYank_CapturesTheBody(t *testing.T) {
	m := makeModelWithBody(t, "hub/x", `{"a":1}`)
	m = update(m, key("y"))

	if !strings.Contains(m.bodyRegister, `"a"`) {
		t.Errorf("bodyRegister = %q, want the displayed body", m.bodyRegister)
	}
	if strings.Contains(m.bodyRegister, "\x1b") {
		t.Error("the yanked body must be plain text, not the coloured rendering")
	}
}
