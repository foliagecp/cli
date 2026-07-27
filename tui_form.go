package main

import (
	"github.com/foliagecp/easyjson"
)

// The form layer.
//
// formState is a PURE state machine: no tea.Cmd, no dbClient, no I/O. Update
// translates the formAction it returns into commands. That split is what makes
// every flow testable with the existing harness and no network.

// ── Chrome: where a form draws ────────────────────────────────────────────────

type formChrome int

const (
	chromeStatus formChrome = iota // one line in the status bar
	chromeCenter                   // replaces the centre panel's body
	chromeFull                     // owns the whole screen
)

// ── Fields ────────────────────────────────────────────────────────────────────

type fieldKind int

const (
	fieldText   fieldKind = iota // free text
	fieldID                      // text constrained to the server's id charset
	fieldEnum                    // ←/→/Tab cycles a fixed option list
	fieldTags                    // comma/space separated, rendered as chips
	fieldBool                    // space toggles
	fieldJSON                    // hands off to the full-screen editor
	fieldVertex                  // link endpoint pair, ←/→ swaps direction
	fieldStatic                  // read-only derived value
)

type enumOption struct{ label, value, hint string }

type formField struct {
	key      string // stable lookup key used by the flows
	label    string
	kind     fieldKind
	required bool
	readOnly bool
	hint     string

	value   string       // fieldText / fieldID / fieldTags
	options []enumOption // fieldEnum
	optIdx  int
	boolVal bool
	static  string      // fieldStatic
	json    *jsonEditor // fieldJSON

	err string // set by validate(); "" == valid
}

// ── Context snapshot ──────────────────────────────────────────────────────────

// formCtx is captured when a form opens and never re-read from the model.
//
// Update's message switch runs BEFORE the dispatch chain, so a vertex load
// landing while a form is open rebuilds the groups and swaps m.fvi underneath
// it. A form that consulted m.currentID at submit time would then act on a
// vertex the user is no longer editing. Snapshotting removes the failure mode
// structurally rather than by timing.
type formCtx struct {
	fromID   string
	toID     string
	fromKind vertexKind
	toKind   vertexKind
	fromType string
	toType   string
	linkName string
	linkType string
	origBody easyjson.JSON
	entity   entityKind
	llMode   bool
	domain   string
}

// ── Form ──────────────────────────────────────────────────────────────────────

type formKind int

const (
	formBodyEdit formKind = iota
	formDeleteVertex
	formDeleteLink
)

type formState struct {
	kind   formKind
	title  string
	chrome formChrome
	fields []formField
	cur    int
	ctx    formCtx

	// confirmation sub-state (delete flows)
	confirm     bool
	confirmWord string // "" ⇒ single-key y/n; otherwise must be typed exactly
	confirmText string
	affected    int // -1 = unknown

	submitting bool
	err        string // submission failure
}

// formAction is what Update should do after handing a key to the form.
type formAction int

const (
	actNone formAction = iota
	actClose
	actSubmit
	actOpenEditor
)

// ── Accessors ─────────────────────────────────────────────────────────────────

func (f formState) field(key string) (formField, bool) {
	for _, fl := range f.fields {
		if fl.key == key {
			return fl, true
		}
	}
	return formField{}, false
}

func (f formState) value(key string) string {
	fl, _ := f.field(key)
	switch fl.kind {
	case fieldEnum:
		if fl.optIdx < len(fl.options) {
			return fl.options[fl.optIdx].value
		}
		return ""
	case fieldStatic:
		return fl.static
	default:
		return fl.value
	}
}

func (f formState) boolean(key string) bool {
	fl, _ := f.field(key)
	return fl.boolVal
}

func (f formState) body(key string) (easyjson.JSON, bool) {
	fl, ok := f.field(key)
	if !ok || fl.json == nil {
		return easyjson.NewJSONObject(), false
	}
	return fl.json.body()
}

func (f formState) focused() *formField {
	if f.cur < 0 || f.cur >= len(f.fields) {
		return nil
	}
	return &f.fields[f.cur]
}

// editor returns the focused JSON editor, if the focused field has one.
func (f formState) editor() *jsonEditor {
	if fl := f.focused(); fl != nil {
		return fl.json
	}
	return nil
}

// ok reports whether the form may be submitted.
func (f formState) ok() bool {
	if f.err != "" {
		return false
	}
	for _, fl := range f.fields {
		if fl.err != "" {
			return false
		}
		if fl.required && !fl.readOnly && f.value(fl.key) == "" && fl.kind != fieldJSON {
			return false
		}
		if fl.kind == fieldJSON && fl.json != nil && !fl.json.valid() {
			return false
		}
	}
	return true
}

// ── Validation ────────────────────────────────────────────────────────────────

func (f formState) validate() formState {
	for i := range f.fields {
		f.fields[i].err = validateField(f.fields[i])
	}
	return f
}

func validateField(fl formField) string {
	switch fl.kind {
	case fieldID:
		if fl.value == "" {
			if fl.required {
				return "required"
			}
			return ""
		}
		if err := validateID(fl.label, fl.value); err != nil {
			// validateID formats for the shell; strip the marker for the TUI.
			return trimLeadingMark(err.Error())
		}
	case fieldText:
		if fl.required && fl.value == "" {
			return "required"
		}
	case fieldTags:
		for _, t := range splitTags(fl.value) {
			if !validIDRe.MatchString(t) {
				return "tag " + t + " has characters outside a-z A-Z 0-9 / _ $ # @ % + = -"
			}
		}
	case fieldEnum:
		if fl.required && len(fl.options) == 0 {
			return "no options available"
		}
	case fieldJSON:
		if fl.json != nil {
			return fl.json.parseErr
		}
	}
	return ""
}

func trimLeadingMark(s string) string {
	if len(s) > 2 && s[0:len("✗ ")] == "✗ " {
		return s[len("✗ "):]
	}
	return s
}

// splitTags parses a tag field: comma or space separated, deduped.
func splitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	cur := ""
	flush := func() {
		if cur == "" || seen[cur] {
			cur = ""
			return
		}
		seen[cur] = true
		out = append(out, cur)
		cur = ""
	}
	for _, r := range s {
		if r == ',' || r == ' ' || r == '\t' {
			flush()
			continue
		}
		cur += string(r)
	}
	flush()
	return out
}

func (f formState) tags(key string) []string {
	fl, _ := f.field(key)
	return splitTags(fl.value)
}

// ── Key handling ──────────────────────────────────────────────────────────────

// handleKey advances the form. Pure: it returns the next state and what the
// caller should do, and never performs the action itself.
//
// The fields slice is copied up front. Without it the returned state would
// share its backing array with the receiver, so a caller holding the previous
// state — a test asserting that a key did nothing, say — would see it mutated
// too. The jsonEditor pointer inside a field is deliberately NOT copied: it
// owns live textarea state that must survive across keystrokes.
func (f formState) handleKey(k string) (formState, formAction) {
	f.fields = append([]formField(nil), f.fields...)

	if f.submitting {
		// Only cancelling is allowed while a submission is in flight; other
		// keys would queue edits against state that is about to change.
		if k == "esc" || k == "ctrl+c" {
			return f, actClose
		}
		return f, actNone
	}

	if f.confirm {
		return f.handleConfirmKey(k)
	}

	switch k {
	case "ctrl+c", "esc":
		return f, actClose

	case "ctrl+s":
		if f.ok() {
			f.submitting = true
			return f, actSubmit
		}
		return f, actNone

	case "ctrl+e":
		if f.editor() != nil {
			return f, actOpenEditor
		}
		return f, actNone

	case "ctrl+r":
		if ed := f.editor(); ed != nil {
			ed.replace = !ed.replace
		}
		return f, actNone

	case "tab":
		f.cur = f.nextField(+1)
		return f, actNone

	case "shift+tab":
		f.cur = f.nextField(-1)
		return f, actNone
	}

	fl := f.focused()
	if fl == nil {
		return f, actNone
	}

	switch fl.kind {
	case fieldEnum:
		switch k {
		case "left":
			if n := len(fl.options); n > 0 {
				fl.optIdx = (fl.optIdx - 1 + n) % n
			}
			return f.validate(), actNone
		case "right":
			if n := len(fl.options); n > 0 {
				fl.optIdx = (fl.optIdx + 1) % n
			}
			return f.validate(), actNone
		}
	case fieldBool:
		if k == " " || k == "space" || k == "left" || k == "right" {
			fl.boolVal = !fl.boolVal
			return f, actNone
		}
	case fieldVertex:
		if k == "left" || k == "right" {
			f.ctx.fromID, f.ctx.toID = f.ctx.toID, f.ctx.fromID
			f.ctx.fromKind, f.ctx.toKind = f.ctx.toKind, f.ctx.fromKind
			f.ctx.fromType, f.ctx.toType = f.ctx.toType, f.ctx.fromType
			return f.validate(), actNone
		}
	}

	// Enter submits on the last field, advances otherwise. Inside the JSON
	// editor it is a newline, so submitting there is ctrl+s only.
	if k == "enter" && fl.kind != fieldJSON {
		if f.cur == f.lastFieldIdx() {
			if f.ok() {
				f.submitting = true
				return f, actSubmit
			}
			return f, actNone
		}
		f.cur = f.nextField(+1)
		return f, actNone
	}

	return f, actNone
}

func (f formState) handleConfirmKey(k string) (formState, formAction) {
	switch k {
	case "ctrl+c", "esc":
		return f, actClose
	}
	if f.confirmWord == "" {
		switch k {
		case "y", "Y":
			f.submitting = true
			return f, actSubmit
		case "n", "N":
			return f, actClose
		}
		return f, actNone
	}
	switch k {
	case "enter":
		if f.confirmText == f.confirmWord {
			f.submitting = true
			return f, actSubmit
		}
		f.err = "type " + f.confirmWord + " exactly to confirm"
		return f, actNone
	case "backspace":
		if n := len(f.confirmText); n > 0 {
			f.confirmText = f.confirmText[:n-1]
			f.err = ""
		}
		return f, actNone
	}
	// Single printable runes build up the typed confirmation. Anything longer
	// is a chord (ctrl+…, arrows) and is ignored rather than inserted.
	if len([]rune(k)) == 1 {
		f.confirmText += k
		f.err = ""
	}
	return f, actNone
}

// nextField moves focus, skipping fields that cannot take it.
func (f formState) nextField(dir int) int {
	n := len(f.fields)
	if n == 0 {
		return 0
	}
	i := f.cur
	for range f.fields {
		i = (i + dir + n) % n
		if f.fields[i].focusable() {
			return i
		}
	}
	return f.cur
}

func (f formState) lastFieldIdx() int {
	for i := len(f.fields) - 1; i >= 0; i-- {
		if f.fields[i].focusable() {
			return i
		}
	}
	return 0
}

func (fl formField) focusable() bool {
	return !fl.readOnly && fl.kind != fieldStatic
}
