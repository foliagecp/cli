package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// ── jsonEditor ────────────────────────────────────────────────────────────────

func bodyWith(pairs map[string]string) easyjson.JSON {
	j := easyjson.NewJSONObject()
	for k, v := range pairs {
		j.SetByPath(k, easyjson.NewJSON(v))
	}
	return j
}

func TestJSONEditor_StripsReservedPaths(t *testing.T) {
	body := easyjson.NewJSONObject()
	body.SetByPath("name", easyjson.NewJSON("srv"))
	body.SetByPath("triggers.create", easyjson.NewJSON("fn"))
	body.SetByPath("cache.parent_types", easyjson.NewJSONArray())

	ed := newJSONEditor(body, entType, 80, 20)

	text := ed.ta.Value()
	if strings.Contains(text, "triggers") {
		t.Error("triggers must not appear in the editable text")
	}
	if strings.Contains(text, "parent_types") {
		t.Error("the cache subtree must not appear in the editable text")
	}
	if !strings.Contains(text, "srv") {
		t.Error("user data should still be editable")
	}
	if names := ed.reservedNames(); len(names) != 2 {
		t.Errorf("reservedNames() = %v, want triggers and cache", names)
	}
}

func TestJSONEditor_ReplaceGraftsReservedBack(t *testing.T) {
	// Under REPLACE the whole body is overwritten, so machine-owned subtrees
	// the user never saw must be put back or they would be silently deleted.
	body := easyjson.NewJSONObject()
	body.SetByPath("name", easyjson.NewJSON("srv"))
	body.SetByPath("triggers.create", easyjson.NewJSON("fn"))

	ed := newJSONEditor(body, entType, 80, 20)
	ed.replace = true
	ed.ta.SetValue(`{"name":"srv2"}`)
	ed.validate()

	got, ok := ed.body()
	if !ok {
		t.Fatal("body() failed on valid JSON")
	}
	if got.GetByPath("triggers.create").AsStringDefault("") != "fn" {
		t.Errorf("triggers were dropped under REPLACE: %s", got.ToString())
	}
	if got.GetByPath("name").AsStringDefault("") != "srv2" {
		t.Errorf("the edit was lost: %s", got.ToString())
	}
}

func TestJSONEditor_MergeOmitsReserved(t *testing.T) {
	// Under MERGE the server keeps what it has; resending it only widens the
	// payload.
	body := easyjson.NewJSONObject()
	body.SetByPath("name", easyjson.NewJSON("srv"))
	body.SetByPath("triggers.create", easyjson.NewJSON("fn"))

	ed := newJSONEditor(body, entType, 80, 20)
	ed.replace = false
	ed.ta.SetValue(`{"name":"srv2"}`)
	ed.validate()

	got, _ := ed.body()
	if got.PathExists("triggers") {
		t.Errorf("MERGE should not resend reserved paths: %s", got.ToString())
	}
}

func TestJSONEditor_LargeBodyIsNotTruncated(t *testing.T) {
	// textarea.MaxHeight defaults to 99 and SetValue silently drops every line
	// past it. A 150-key body pretty-prints to ~152 lines, so a regression here
	// would quietly destroy user data on save.
	pairs := map[string]string{}
	for i := 0; i < 150; i++ {
		pairs[fmt.Sprintf("k%03d", i)] = "v"
	}
	ed := newJSONEditor(bodyWith(pairs), entVertex, 80, 20)

	got, ok := ed.body()
	if !ok {
		t.Fatalf("body() failed; text was truncated to:\n%s", ed.ta.Value())
	}
	for i := 0; i < 150; i++ {
		key := fmt.Sprintf("k%03d", i)
		if !got.PathExists(key) {
			t.Fatalf("key %s was lost — the textarea truncated the body", key)
		}
	}
}

func TestPrettyJSON_HasNoANSI(t *testing.T) {
	// JSONStrPrettyStringAnyway colourises through colorjson; feeding that into
	// the textarea would put escape codes into the body sent to the server.
	out := prettyJSON(bodyWith(map[string]string{"a": "b"}))
	if strings.Contains(out, "\x1b") {
		t.Errorf("prettyJSON emitted ANSI escapes: %q", out)
	}
}

func TestJSONEditor_ValidationReportsPosition(t *testing.T) {
	ed := newJSONEditor(easyjson.NewJSONObject(), entVertex, 80, 20)

	ed.ta.SetValue("{\n  \"a\": ,\n}")
	ed.validate()
	if ed.valid() {
		t.Fatal("malformed JSON should be invalid")
	}
	if !strings.Contains(ed.parseErr, "line 2") {
		t.Errorf("parseErr = %q, want it to name the line", ed.parseErr)
	}

	ed.ta.SetValue(`[1,2]`)
	ed.validate()
	if ed.valid() {
		t.Fatal("a non-object body should be invalid")
	}

	ed.ta.SetValue(`{"a":1}`)
	ed.validate()
	if !ed.valid() {
		t.Errorf("valid JSON reported invalid: %s", ed.parseErr)
	}
}

func TestReservedPaths_PerEntity(t *testing.T) {
	if got := reservedPaths(entTypesLink); len(got) != 1 || got[0] != "type" {
		t.Errorf("types-link reserved = %v, want [type] (it holds the object-link type)", got)
	}
	if got := reservedPaths(entLink); len(got) != 0 {
		t.Errorf("plain link reserved = %v, want none", got)
	}
	if got := reservedPaths(entType); len(got) != 4 {
		t.Errorf("type reserved = %v, want 4 entries", got)
	}
}

// ── formState ─────────────────────────────────────────────────────────────────

func bodyForm(t *testing.T) formState {
	t.Helper()
	f, ok := openBodyEditForm(makeModelWithBody(t, "hub/x", `{"a":1}`))
	if !ok {
		t.Fatal("openBodyEditForm returned !ok")
	}
	return f
}

func makeModelWithBody(t *testing.T, id, body string) tuiModel {
	t.Helper()
	j, ok := easyjson.JSONFromString(body)
	if !ok {
		t.Fatalf("bad fixture body: %s", body)
	}
	fvi := fullVertexInfo{id: id, body: j.GetPtr()}
	return makeModel(id, nil, &fvi)
}

func TestForm_EscClosesAndCtrlSSubmits(t *testing.T) {
	f := bodyForm(t)

	if _, action := f.handleKey("esc"); action != actClose {
		t.Errorf("esc → %v, want actClose", action)
	}
	next, action := f.handleKey("ctrl+s")
	if action != actSubmit {
		t.Fatalf("ctrl+s → %v, want actSubmit", action)
	}
	if !next.submitting {
		t.Error("submitting flag should be set")
	}
}

func TestForm_CtrlSBlockedWhileInvalid(t *testing.T) {
	f := bodyForm(t)
	f.fields[0].json.ta.SetValue("{oops")
	f.fields[0].json.validate()
	f = f.validate()

	if _, action := f.handleKey("ctrl+s"); action != actNone {
		t.Errorf("ctrl+s on invalid JSON → %v, want actNone", action)
	}
}

func TestForm_CtrlRTogglesReplace(t *testing.T) {
	f := bodyForm(t)
	if f.editor().replace {
		t.Fatal("edits should default to MERGE")
	}
	f, _ = f.handleKey("ctrl+r")
	if !f.editor().replace {
		t.Error("ctrl+r should switch to REPLACE")
	}
	f, _ = f.handleKey("ctrl+r")
	if f.editor().replace {
		t.Error("ctrl+r should toggle back to MERGE")
	}
}

func TestForm_CtrlERequestsEditor(t *testing.T) {
	f := bodyForm(t)
	if _, action := f.handleKey("ctrl+e"); action != actOpenEditor {
		t.Errorf("ctrl+e → %v, want actOpenEditor", action)
	}
}

func TestForm_SubmittingIgnoresKeysButAllowsCancel(t *testing.T) {
	f := bodyForm(t)
	f.submitting = true

	if _, action := f.handleKey("ctrl+s"); action != actNone {
		t.Error("keys should be ignored while a submission is in flight")
	}
	if _, action := f.handleKey("esc"); action != actClose {
		t.Error("esc should still cancel while submitting")
	}
}

func TestForm_HandleKeyDoesNotMutateReceiver(t *testing.T) {
	// The fields slice is copied so a caller holding the old state — a test
	// asserting a key did nothing, for one — does not see it change.
	f := bodyForm(t)
	f.fields = append(f.fields, formField{key: "e", kind: fieldEnum,
		options: []enumOption{{label: "a", value: "a"}, {label: "b", value: "b"}}})
	f.cur = 1

	next, _ := f.handleKey("right")
	if f.fields[1].optIdx != 0 {
		t.Error("the receiver's field was mutated")
	}
	if next.fields[1].optIdx != 1 {
		t.Errorf("the returned state did not advance: optIdx = %d", next.fields[1].optIdx)
	}
}

func TestSplitTags(t *testing.T) {
	cases := map[string][]string{
		"":        nil,
		"a":       {"a"},
		"a,b":     {"a", "b"},
		"a b":     {"a", "b"},
		" a , b ": {"a", "b"},
		"a,a":     {"a"},
	}
	for in, want := range cases {
		got := splitTags(in)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("splitTags(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestForm_EntityForVertex(t *testing.T) {
	if got := entityForVertex(vkType, false); got != entType {
		t.Errorf("type vertex → %v, want entType", got)
	}
	if got := entityForVertex(vkObject, false); got != entObject {
		t.Errorf("object vertex → %v, want entObject", got)
	}
	if got := entityForVertex(vkType, true); got != entVertex {
		t.Errorf("low-level mode should force entVertex, got %v", got)
	}
}

// ── Integration through Update ────────────────────────────────────────────────

func TestKeyI_OpensBodyEditor(t *testing.T) {
	m := makeModelWithBody(t, "hub/x", `{"a":1}`)
	m = update(m, key("i"))

	if m.form == nil {
		t.Fatal("i should open a form")
	}
	if m.form.chrome != chromeFull {
		t.Errorf("chrome = %v, want chromeFull", m.form.chrome)
	}
	if m.form.editor() == nil {
		t.Fatal("the form should carry a JSON editor")
	}
	if !strings.Contains(m.form.editor().ta.Value(), `"a"`) {
		t.Error("the editor should be pre-filled with the current body")
	}
}

func TestKeyI_RefusedWhileLoading(t *testing.T) {
	m := makeModelWithBody(t, "hub/x", `{"a":1}`)
	m.loading = true
	m = update(m, key("i"))

	if m.form != nil {
		t.Error("a form must not open over a body that is about to be replaced")
	}
}

func TestKeyX_TogglesLowLevelMode(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("x"))
	if !m.llMode {
		t.Fatal("x should turn the low-level mode on")
	}
	if !strings.Contains(m.renderCenterPanel(), "[LL]") {
		t.Error("the low-level mode must be visible in the header")
	}
	m = update(m, key("x"))
	if m.llMode {
		t.Error("x should toggle the low-level mode back off")
	}
}

func TestBodyEdit_SubmitsToTheRightAPI(t *testing.T) {
	cases := []struct {
		name    string
		links   []displayLink
		llMode  bool
		wantAPI string
	}{
		{
			name: "type vertex uses the CMDB type API",
			links: []displayLink{
				{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
			},
			wantAPI: "type",
		},
		{
			name: "object vertex uses the CMDB object API",
			links: []displayLink{
				{info: makeLinkInfo("hub/objects", "srv-1", "hub/srv-1", "__object"), isOut: false},
				{info: makeLinkInfo("hub/srv-1", "type", "hub/srv", "__type"), isOut: true},
			},
			wantAPI: "object",
		},
		{
			name:    "plain vertex uses the raw graph API",
			links:   nil,
			wantAPI: "vertex",
		},
		{
			name: "low-level mode forces the raw graph API on a type",
			links: []displayLink{
				{info: makeLinkInfo("hub/types", "srv", "hub/srv", "__type"), isOut: false},
			},
			llMode:  true,
			wantAPI: "vertex",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := ""
			withOps(t, graphOps{
				typeUpdate: func(string, easyjson.JSON, bool, bool) opResult {
					called = "type"
					return opResult{status: opApplied}
				},
				objectUpdate: func(string, easyjson.JSON, bool, string) opResult {
					called = "object"
					return opResult{status: opApplied}
				},
				vertexUpdate: func(string, easyjson.JSON, bool, bool) opResult {
					called = "vertex"
					return opResult{status: opApplied}
				},
			})

			j, _ := easyjson.JSONFromString(`{"a":1}`)
			fvi := fullVertexInfo{id: "hub/srv", body: j.GetPtr()}
			m := makeModel("hub/srv", c.links, &fvi)
			m.llMode = c.llMode

			m = update(m, key("i"))
			if m.form == nil {
				t.Fatal("i did not open the editor")
			}
			m, cmd := updateCmd(m, tea_ctrlS())
			runCmd(cmd)

			if called != c.wantAPI {
				t.Errorf("called the %q API, want %q", called, c.wantAPI)
			}
		})
	}
}

func TestBodyEdit_PassesReplaceFlag(t *testing.T) {
	var gotReplace bool
	withOps(t, graphOps{
		vertexUpdate: func(_ string, _ easyjson.JSON, replace, _ bool) opResult {
			gotReplace = replace
			return opResult{status: opApplied}
		},
	})

	m := makeModelWithBody(t, "hub/x", `{"a":1}`)
	m = update(m, key("i"))
	m = update(m, tea_ctrlR()) // MERGE → REPLACE
	m, cmd := updateCmd(m, tea_ctrlS())
	runCmd(cmd)

	if !gotReplace {
		t.Error("ctrl+r should have switched the submission to REPLACE")
	}
}

func TestMutationResult_InvalidatesAndToasts(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	m.cache["hub/x"] = cachedVertex{}
	m.cache["hub/y"] = cachedVertex{}

	msg := mutationResultMsg{
		op:         "vertex.update",
		target:     "hub/x",
		res:        opResult{status: opApplied},
		invalidate: []string{"hub/x"},
	}
	next, _ := m.Update(msg)
	m = next.(tuiModel)

	if _, ok := m.cache["hub/x"]; ok {
		t.Error("hub/x should have been evicted")
	}
	if _, ok := m.cache["hub/y"]; !ok {
		t.Error("hub/y should have survived")
	}
	if !strings.Contains(m.queryResult, "✓") {
		t.Errorf("toast = %q, want a success marker", m.queryResult)
	}
	if m.form != nil {
		t.Error("the form should be closed once the result lands")
	}
}

func TestMutationResult_NoopIsDistinctFromApplied(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	msg := mutationResultMsg{
		op:     "vertex.update",
		target: "hub/x",
		res:    opResult{status: opNoop, details: "body unchanged"},
	}
	next, _ := m.Update(msg)
	m = next.(tuiModel)

	if strings.Contains(m.queryResult, "✓") {
		t.Errorf("a no-op must not report success: %q", m.queryResult)
	}
	if !strings.Contains(m.queryResult, "∅") {
		t.Errorf("toast = %q, want the no-op marker", m.queryResult)
	}
}

func TestMutationResult_FailurePersistsInHeader(t *testing.T) {
	m := makeModel("hub/x", nil, nil)
	msg := mutationResultMsg{
		op:     "vertex.update",
		target: "hub/x",
		res:    opResult{status: opFailed, details: "boom"},
	}
	next, _ := m.Update(msg)
	m = next.(tuiModel)

	if m.errMsg != "boom" {
		t.Errorf("errMsg = %q, want the failure to persist in the header", m.errMsg)
	}
}

func TestEditorDone_AppliesTextAndSurvivesClosedForm(t *testing.T) {
	m := makeModelWithBody(t, "hub/x", `{"a":1}`)
	m = update(m, key("i"))

	next, _ := m.Update(editorDoneMsg{text: `{"b":2}`})
	m = next.(tuiModel)
	if got := m.form.editor().ta.Value(); got != `{"b":2}` {
		t.Errorf("editor text = %q, want the $EDITOR result", got)
	}

	// With no form open the text must be reported as discarded, not swallowed.
	m.form = nil
	next, _ = m.Update(editorDoneMsg{text: `{"c":3}`})
	m = next.(tuiModel)
	if !strings.Contains(m.queryResult, "discarded") {
		t.Errorf("toast = %q, want it to say the edit was discarded", m.queryResult)
	}
}

// Key helpers for control chords the shared key() helper cannot express.
func tea_ctrlS() tea.Msg { return tea.KeyMsg{Type: tea.KeyCtrlS} }
func tea_ctrlR() tea.Msg { return tea.KeyMsg{Type: tea.KeyCtrlR} }
