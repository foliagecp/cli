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

// ── Pending link (Start → walk → Commit) ─────────────────────────────────────

func TestLink_StartShowsAPendingBanner(t *testing.T) {
	m := makeModel("hub/a", nil, nil)

	m = update(m, key("L"))
	if m.linking == nil || m.linking.fromID != "hub/a" {
		t.Fatalf("L should start a link from the current vertex, got %+v", m.linking)
	}

	banner := m.renderBreadcrumbs()
	for _, want := range []string{"LINK PENDING", "a", "commit", "cancel"} {
		if !strings.Contains(banner, want) {
			t.Errorf("banner should mention %q — it is what tells the user a link is pending:\n%s", want, banner)
		}
	}
	if m.breadcrumbH() != 1 {
		t.Error("the banner row must be reserved while a link is pending")
	}
}

func TestLink_EscCancels(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))
	m = update(m, keyEsc())

	if m.linking != nil {
		t.Error("Esc should cancel a pending link")
	}
	if !strings.Contains(m.queryResult, "cancel") {
		t.Errorf("toast = %q, want it to confirm the cancellation", m.queryResult)
	}
}

func TestLink_SurvivesNavigation(t *testing.T) {
	// "Walk anywhere" is the whole point: a pending link must not turn the
	// browser into a modal prompt.
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))

	m = update(m, key("v"))
	m = update(m, key("j"))
	m = update(m, key("h"))
	if m.linking == nil {
		t.Fatal("navigating must not clear the pending link")
	}
}

func TestLink_SecondPressCommits(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))

	m.currentID = "hub/b" // walked to the target
	m = update(m, key("L"))

	if m.form == nil || m.form.kind != formLinkCreate {
		t.Fatalf("the second L should open the link form, got %+v", m.form)
	}
	if m.form.ctx.fromID != "hub/a" || m.form.ctx.toID != "hub/b" {
		t.Errorf("endpoints = %s → %s, want hub/a → hub/b", m.form.ctx.fromID, m.form.ctx.toID)
	}
}

func TestLink_CommitOnTheSourceIsRefused(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))
	m = update(m, key("L")) // still standing on the source

	if m.form != nil {
		t.Error("a vertex cannot be linked to itself through this flow")
	}
	if m.queryResult == "" {
		t.Error("the refusal should be explained")
	}
}

func TestLink_ClearedByR(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	m = update(m, key("L"))
	m = update(m, key("R"))
	if m.linking != nil {
		t.Error("R resets navigation state and should drop a pending link")
	}
}

// ── Link creation ─────────────────────────────────────────────────────────────

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
	m.linking = &pendingLink{fromID: "hub/srv", kind: vkType}

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
	m.linking = &pendingLink{fromID: "hub/srv-1", kind: vkObject, typeName: "srv"}

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
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}

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
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
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
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
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
	m.linking = &pendingLink{fromID: "hub/srv", kind: vkType}
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
	m.linking = &pendingLink{fromID: "hub/a", kind: vkPlain}
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

// ── Create menu: you create a thing where that thing lives ───────────────────

func TestCreateMenu_TypeOnlyFromTheTypesRoot(t *testing.T) {
	// Away from the types root the entry is still shown, but it says where
	// types live rather than silently vanishing.
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))
	out := m.renderFormCenter(100, 20)
	if !strings.Contains(out, "types are created from") {
		t.Errorf("the menu should say where types live:\n%s", out)
	}

	// On the types root it is available outright.
	m = makeModel(NatsHubDomain+"/types", nil, nil)
	m = update(m, key("n"))
	for _, e := range createMenuFor(m) {
		if e.key == "t" && !e.available() {
			t.Error("standing on the types root, creating a type must be available")
		}
	}
}

func TestCreateMenu_ChoosingAGatedEntryNavigatesToItsHome(t *testing.T) {
	// Refusing would teach nothing; going there makes the rule concrete.
	m := makeModel("hub/srv-1", objectAt("hub/srv-1", "hub/srv"), nil)
	m = update(m, key("n"))
	m, cmd := updateCmd(m, key("t"))

	if m.form != nil {
		t.Error("a gated entry should close the menu, not open a form")
	}
	if cmd == nil {
		t.Fatal("choosing a gated entry should navigate to where it is allowed")
	}
	if !strings.Contains(m.queryResult, "types are created from") {
		t.Errorf("toast = %q, want the rule stated", m.queryResult)
	}
}

func TestCreateMenu_ObjectOnlyFromItsType(t *testing.T) {
	// On the type: available.
	m := makeModel("hub/srv", typeAt("hub/srv"), nil)
	for _, e := range createMenuFor(m) {
		if e.key == "o" && !e.available() {
			t.Error("standing on a type, creating its object must be available")
		}
	}
	if !strings.Contains(m.renderFormCenterVia(t), "object of srv") {
		t.Error("the menu should name the type whose object it creates")
	}

	// On an object: gated, and it points at the object's own type.
	m = makeModel("hub/srv-1", objectAt("hub/srv-1", "hub/srv"), nil)
	for _, e := range createMenuFor(m) {
		if e.key == "o" {
			if e.available() {
				t.Error("objects are created from their type, not from a sibling")
			}
			if e.unavailableAt != "srv" {
				t.Errorf("gated entry points at %q, want the object's type", e.unavailableAt)
			}
		}
	}
}

func TestCreateMenu_LinkIsAlwaysAvailable(t *testing.T) {
	// A link lives on its endpoints, so it can be started from anywhere.
	m := makeModel("hub/anything", nil, nil)
	for _, e := range createMenuFor(m) {
		if e.key == "l" && !e.available() {
			t.Error("starting a link should never be gated — an endpoint is always where you stand")
		}
	}
}

func TestCreateMenu_RawVertexSaysItWillBeLinked(t *testing.T) {
	m := makeModel("hub/a", nil, nil)
	out := ""
	for _, e := range createMenuFor(m) {
		if e.key == "v" {
			out = e.label
		}
	}
	if !strings.Contains(out, "linked from a") {
		t.Errorf("the raw-vertex entry must state that it will be attached, got %q", out)
	}
}

func TestCreateMenu_OpensTheChosenForm(t *testing.T) {
	m := makeModel(NatsHubDomain+"/types", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("t"))
	if m.form == nil || m.form.kind != formTypeCreate {
		t.Fatalf("t on the types root should open the type form, got %+v", m.form)
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
	m := makeModel(NatsHubDomain+"/types", nil, nil)
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
	m := makeModel(NatsHubDomain+"/types", nil, nil)
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

// renderFormCenterVia is a small helper: open the menu and render it.
func (m tuiModel) renderFormCenterVia(t *testing.T) string {
	t.Helper()
	m2 := update(m, key("n"))
	if m2.form == nil {
		t.Fatal("n did not open the menu")
	}
	return m2.renderFormCenter(100, 20)
}

// ── The two promises of the create flow ───────────────────────────────────────

func TestVertexCreate_AlwaysLinksItSoItStaysReachable(t *testing.T) {
	// A raw vertex nothing points at is invisible to a graph browser the moment
	// you navigate away. The link is therefore mandatory, not a toggle.
	var createdID, linkFrom, linkTo, linkName, linkType string
	withOps(t, graphOps{
		vertexCreate: func(id string, _ easyjson.JSON) opResult {
			createdID = id
			return opResult{status: opApplied}
		},
		linkCreate: func(from, to, name, tp string, _ []string, _ easyjson.JSON, _ bool) opResult {
			linkFrom, linkTo, linkName, linkType = from, to, name, tp
			return opResult{status: opApplied}
		},
	})

	m := makeModel("hub/parent", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("v"))
	if m.form == nil {
		t.Fatal("v should open the raw-vertex form")
	}
	for _, k := range []string{"linkname", "linktype"} {
		if fl, ok := m.form.field(k); !ok || !fl.required {
			t.Errorf("%q must be a required field — the link is what keeps the vertex reachable", k)
		}
	}

	m = focusField(m, "id")
	m = typeText(m, "child")
	m = focusField(m, "linkname")
	m = typeText(m, "kid")
	m = focusField(m, "linktype")
	m = typeText(m, "rel")

	_, cmd := updateCmd(m, tea_ctrlS())
	msg := runCmd(cmd).(mutationResultMsg)

	if createdID != "child" {
		t.Errorf("vertexCreate(%q), want child", createdID)
	}
	if linkFrom != "hub/parent" || linkTo != "child" {
		t.Errorf("link %s → %s, want hub/parent → child", linkFrom, linkTo)
	}
	if linkName != "kid" || linkType != "rel" {
		t.Errorf("link name/type = %q/%q, want kid/rel", linkName, linkType)
	}
	if msg.navTo != "child" {
		t.Errorf("navTo = %q — after creating something you should be standing on it", msg.navTo)
	}
}

func TestVertexCreate_ReportsAFailedLinkAsAFailure(t *testing.T) {
	// The vertex exists but nothing reaches it. Reporting success would leave
	// the user believing they have something they cannot find.
	withOps(t, graphOps{
		vertexCreate: func(string, easyjson.JSON) opResult { return opResult{status: opApplied} },
		linkCreate: func(_, _, _, _ string, _ []string, _ easyjson.JSON, _ bool) opResult {
			return opResult{status: opFailed, details: "already exists"}
		},
	})

	m := makeModel("hub/parent", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("v"))
	m = focusField(m, "id")
	m = typeText(m, "child")
	m = focusField(m, "linkname")
	m = typeText(m, "kid")
	m = focusField(m, "linktype")
	m = typeText(m, "rel")

	_, cmd := updateCmd(m, tea_ctrlS())
	msg := runCmd(cmd).(mutationResultMsg)

	if msg.res.status != opFailed {
		t.Fatal("a vertex that could not be linked must not be reported as created")
	}
	if !strings.Contains(msg.res.details, "linking it failed") {
		t.Errorf("details = %q, want it to say the vertex exists but is unlinked", msg.res.details)
	}
	if msg.navTo != "" {
		t.Error("do not navigate to a vertex whose creation half-failed")
	}
}

func TestObjectAndTypeCreate_LandOnWhatWasCreated(t *testing.T) {
	withOps(t, graphOps{
		typeCreate:   func(string, easyjson.JSON) opResult { return opResult{status: opApplied} },
		objectCreate: func(string, string, easyjson.JSON) opResult { return opResult{status: opApplied} },
	})

	m := makeModel(NatsHubDomain+"/types", nil, nil)
	m = update(m, key("n"))
	m = update(m, key("t"))
	m = typeText(m, "srv")
	_, cmd := updateCmd(m, tea_ctrlS())
	if got := runCmd(cmd).(mutationResultMsg).navTo; got != "srv" {
		t.Errorf("after creating a type navTo = %q, want srv", got)
	}

	m = makeModel("hub/srv", typeAt("hub/srv"), nil)
	m = update(m, key("n"))
	m = update(m, key("o"))
	m = focusField(m, "id")
	m = typeText(m, "srv-1")
	_, cmd = updateCmd(m, tea_ctrlS())
	if got := runCmd(cmd).(mutationResultMsg).navTo; got != "srv-1" {
		t.Errorf("after creating an object navTo = %q, want srv-1", got)
	}
}

func TestSubType_DeclaredFromTheParentType(t *testing.T) {
	var base, child string
	withOps(t, graphOps{
		subTypeSet: func(b, c string) opResult { base, child = b, c; return opResult{status: opApplied} },
	})

	m := makeModel("hub/hw", typeAt("hub/hw"), nil)
	m = update(m, key("n"))
	m = update(m, key("s"))
	if m.form == nil || m.form.kind != formSubTypeSet {
		t.Fatalf("s on a type should open the sub-type form, got %+v", m.form)
	}
	m = typeText(m, "srv")
	_, cmd := updateCmd(m, tea_ctrlS())
	msg := runCmd(cmd).(mutationResultMsg)

	if base != "hub/hw" || child != "srv" {
		t.Errorf("subTypeSet(%q,%q), want (hub/hw, srv)", base, child)
	}
	if !msg.clearAll {
		t.Error("declaring a sub-type rewrites inherited state on descendants; the cache cannot be trusted")
	}
}
