package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// The JSON body editor: a textarea over an entity's current body, with live
// validation, protection for machine-owned subtrees, and an escape hatch to
// $EDITOR for bodies too large to comfortably edit inline.

// entityKind selects which body paths are machine-owned.
type entityKind int

const (
	entVertex entityKind = iota
	entType
	entObject
	entLink
	entTypesLink
)

// reservedPaths lists subtrees the server owns and rewrites on its own. They
// are lifted out of the text the user edits and put back on submit, so an edit
// can neither clobber them nor make them look editable.
//
// `cache` is reserved as a whole subtree, not leaf by leaf: the polymorphism
// recompute removes and rewrites the entire thing, so anything a user put
// inside would vanish without explanation.
func reservedPaths(e entityKind) []string {
	switch e {
	case entType:
		return []string{"triggers", "meta_triggers", "cache", "human_readable_name_field"}
	case entVertex, entObject:
		return []string{"triggers", "meta_triggers", "cache"}
	case entTypesLink:
		// body.type carries the object-link type that instances inherit; the
		// server writes it when the types-link is created.
		return []string{"type"}
	default:
		return nil
	}
}

func entityLabel(e entityKind) string {
	switch e {
	case entType:
		return "type"
	case entObject:
		return "object"
	case entLink:
		return "link"
	case entTypesLink:
		return "types-link"
	default:
		return "vertex"
	}
}

// ── Editor ────────────────────────────────────────────────────────────────────

type jsonEditor struct {
	ta       textarea.Model
	original easyjson.JSON
	reserved map[string]easyjson.JSON // stripped subtrees, re-grafted on REPLACE
	replace  bool                     // false = MERGE (deep merge), true = REPLACE
	parseErr string                   // "" == the text is a valid JSON object
	entity   entityKind
}

func newJSONEditor(body easyjson.JSON, e entityKind, w, h int) *jsonEditor {
	ed := &jsonEditor{
		original: body,
		reserved: map[string]easyjson.JSON{},
		entity:   e,
	}

	edit := body.Clone()
	for _, p := range reservedPaths(e) {
		if edit.PathExists(p) {
			ed.reserved[p] = edit.GetByPath(p)
			edit.RemoveByPath(p)
		}
	}

	ta := textarea.New()
	// MaxHeight defaults to 99 and SetValue silently DROPS every line past it —
	// a body of 120 lines would come back truncated with no warning. 0 disables
	// the cap. CharLimit already defaults to unlimited, but is pinned here so a
	// future default change cannot quietly start truncating either.
	ta.MaxHeight = 0
	ta.CharLimit = 0
	ta.ShowLineNumbers = true
	ta.Prompt = ""
	ta.SetWidth(w)
	ta.SetHeight(h)
	ta.SetValue(prettyJSON(edit))
	ta.CursorStart()
	ta.Focus()
	ed.ta = ta

	ed.validate()
	return ed
}

// prettyJSON renders a body for editing.
//
// Deliberately NOT JSONStrPrettyStringAnyway: that one colourises through
// colorjson, and feeding ANSI escapes into a textarea would put them in the
// text the user edits and then in the body sent to the server.
func prettyJSON(j easyjson.JSON) string {
	b, err := json.MarshalIndent(j.Value, "", "  ")
	if err != nil {
		return j.ToString()
	}
	return string(b)
}

func (ed *jsonEditor) validate() {
	text := ed.ta.Value()
	var probe any
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		// encoding/json is used rather than easyjson.JSONFromString because it
		// reports WHERE the text broke; easyjson only returns a bool, which
		// makes a typo in a large body genuinely hard to find.
		if se, ok := err.(*json.SyntaxError); ok {
			line, col := offsetToLineCol(text, int(se.Offset))
			ed.parseErr = fmt.Sprintf("line %d, col %d: %s", line, col, se.Error())
		} else {
			ed.parseErr = err.Error()
		}
		return
	}
	if _, isObj := probe.(map[string]any); !isObj {
		ed.parseErr = "body must be a JSON object"
		return
	}
	ed.parseErr = ""
}

func offsetToLineCol(s string, off int) (int, int) {
	if off > len(s) {
		off = len(s)
	}
	line, col := 1, 1
	for i := 0; i < off; i++ {
		if s[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

func (ed *jsonEditor) valid() bool { return ed.parseErr == "" }

func (ed *jsonEditor) dirty() bool {
	return ed.ta.Value() != prettyJSON(strippedClone(ed.original, ed.entity))
}

func strippedClone(j easyjson.JSON, e entityKind) easyjson.JSON {
	c := j.Clone()
	for _, p := range reservedPaths(e) {
		if c.PathExists(p) {
			c.RemoveByPath(p)
		}
	}
	return c
}

// body returns what should be sent to the server.
//
// Under REPLACE the reserved subtrees are grafted back, because the whole body
// is about to be overwritten and dropping them would delete machine state the
// user never saw. Under MERGE they are left out: the server keeps whatever it
// already has, and sending them back would only widen the payload.
func (ed *jsonEditor) body() (easyjson.JSON, bool) {
	j, ok := easyjson.JSONFromString(ed.ta.Value())
	if !ok || !j.IsObject() {
		return easyjson.NewJSONObject(), false
	}
	if ed.replace {
		for p, v := range ed.reserved {
			j.SetByPath(p, v)
		}
	}
	return j, true
}

// setSize re-dimensions an open editor. Called on a terminal resize — without
// it the textarea kept whatever size it had when it opened, so resizing while
// editing left the text laid out for the old width.
func (ed *jsonEditor) setSize(w, h int) {
	ed.ta.SetWidth(w)
	ed.ta.SetHeight(h)
}

// reservedNames lists what is being preserved, for the editor footer.
func (ed *jsonEditor) reservedNames() []string {
	if len(ed.reserved) == 0 {
		return nil
	}
	out := make([]string, 0, len(ed.reserved))
	for _, p := range reservedPaths(ed.entity) { // stable order
		if _, ok := ed.reserved[p]; ok {
			out = append(out, p)
		}
	}
	return out
}

// ── $EDITOR handoff ───────────────────────────────────────────────────────────

type editorDoneMsg struct {
	text string
	err  error
}

// openEditorCmd suspends the TUI and runs $EDITOR on the body.
//
// Stdin/Stdout/Stderr are left nil on purpose: tea.ExecProcess wires the real
// terminal, which is what lets a full-screen editor like vim work.
func openEditorCmd(text string) tea.Cmd {
	f, err := os.CreateTemp("", "foliage-body-*.json")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	name := f.Name()
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_ = f.Close()

	editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi")
	// Fields, not a bare name, so "code -w" or "nvim -u NONE" work.
	parts := strings.Fields(editor)
	c := exec.Command(parts[0], append(parts[1:], name)...)

	return tea.ExecProcess(c, func(runErr error) tea.Msg {
		b, readErr := os.ReadFile(name)
		_ = os.Remove(name)
		return editorDoneMsg{text: string(b), err: cmp.Or(runErr, readErr)}
	})
}
