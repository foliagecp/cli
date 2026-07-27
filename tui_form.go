package main

import (
	"strings"

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
	chromeCenter                   // the centre column — every form with fields
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

	// showIfKey/showIfVal make a field conditional: it is rendered, focusable
	// and validated only while another field holds that value. A form that
	// shows a field the current choice makes meaningless is asking the user to
	// fill in something it intends to ignore.
	showIfKey string
	showIfVal string

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
	tier     linkTier
	origBody easyjson.JSON
	entity   entityKind
	llMode   bool

	// domain the created entity lands in, so a vertex created beside a
	// leaf-domain one does not silently become a hub sibling.
	domain string
}

// ── Form ──────────────────────────────────────────────────────────────────────

type formKind int

const (
	formBodyEdit formKind = iota
	formDeleteVertex
	formDeleteLink
	formLinkCreate
	formCreateMenu
	formVertexCreate
	formTypeCreate
	formObjectCreate
	formSubTypeSet
	formLinkEdit
)

type formState struct {
	kind   formKind
	title  string
	chrome formChrome
	fields []formField
	cur    int
	ctx    formCtx

	// contextRows are read-only facts rendered above the fields: what the form
	// is about, as opposed to what it will write.
	contextRows []string

	// template is the yanked body ctrl+t pastes, captured when the form opens.
	template string

	// confirmation sub-state (delete flows)
	confirm     bool
	confirmWord string // "" ⇒ single-key y/n; otherwise must be typed exactly
	confirmText string

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
// editor returns the FOCUSED field's editor. Use it only for routing
// keystrokes into a textarea — a form-wide question wants jsonField.
func (f formState) editor() *jsonEditor {
	if fl := f.focused(); fl != nil {
		return fl.json
	}
	return nil
}

// nonJSONRows counts the rows the form's other content takes, so the editor
// can be sized to what is left.
func (f formState) nonJSONRows() int {
	n := len(f.contextRows)
	for _, fl := range f.fields {
		if fl.kind == fieldJSON {
			continue
		}
		n++
		if fl.err != "" || fl.hint != "" {
			n++
		}
	}
	return n
}

// jsonField returns the form's JSON editor regardless of what has focus. A form
// has at most one.
//
// Keying "does this form have an editor" off the FOCUSED field was harmless
// while the only editor form was a single JSON field. The moment a sibling
// field can take focus — tags next to a link body — ctrl+r and ctrl+e become
// silent no-ops, the full-screen renderer claims the form has no editor at all,
// and an $EDITOR round-trip that lands while focus is elsewhere throws the
// user's text away with "editor result discarded".
func (f formState) jsonField() *jsonEditor {
	for i := range f.fields {
		if f.fields[i].json != nil {
			return f.fields[i].json
		}
	}
	return nil
}

// ok reports whether the form may be submitted.
// visible reports whether a conditional field currently applies.
func (f formState) visible(fl formField) bool {
	return fl.showIfKey == "" || f.value(fl.showIfKey) == fl.showIfVal
}

func (f formState) ok() bool {
	if f.err != "" {
		return false
	}
	for _, fl := range f.fields {
		if !f.visible(fl) {
			continue
		}
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
		if !f.visible(f.fields[i]) {
			f.fields[i].err = ""
			continue
		}
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
		// Short messages on purpose: a form row is one line and gets clipped
		// to the panel width, so the shell's fuller wording would lose its
		// tail exactly where the actionable part is.
		if strings.Contains(fl.value, ".") {
			return "dots are not allowed (the KV key separator)"
		}
		if strings.Count(fl.value, "/") > 1 {
			return "at most one / — it separates the domain"
		}
		if !validIDRe.MatchString(fl.value) {
			return "allowed: a-z A-Z 0-9 / _ $ # @ % + = -"
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
		if k == "esc" {
			return f, actClose
		}
		return f, actNone
	}

	if f.confirm {
		return f.handleConfirmKey(k)
	}

	switch k {
	case "esc":
		return f, actClose

	case "ctrl+s":
		if f.ok() {
			f.submitting = true
			return f, actSubmit
		}
		return f, actNone

	case "ctrl+e":
		if f.jsonField() != nil {
			return f, actOpenEditor
		}
		return f, actNone

	case "ctrl+r":
		if ed := f.jsonField(); ed != nil {
			ed.replace = !ed.replace
		}
		return f, actNone

	case "ctrl+t":
		// Paste the yanked body. `y` has always written to a register and the
		// help has always advertised this key, but nothing read the register
		// and nothing handled the key — the feature was documented and absent
		// for its whole existence.
		if ed := f.jsonField(); ed != nil && f.template != "" {
			ed.ta.SetValue(f.template)
			ed.validate()
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

	// Typing into a text-ish field. The JSON field has its own widget and is
	// fed by updateForm instead.
	switch fl.kind {
	case fieldText, fieldID, fieldTags:
		switch {
		case k == "backspace":
			if r := []rune(fl.value); len(r) > 0 {
				fl.value = string(r[:len(r)-1])
			}
			return f.validate(), actNone
		case len([]rune(k)) == 1:
			// Single printable rune. Longer strings are chords (ctrl+…,
			// arrows, "shift+tab") and must not be inserted as text.
			fl.value += k
			return f.validate(), actNone
		}
	}

	return f, actNone
}

func (f formState) handleConfirmKey(k string) (formState, formAction) {
	switch k {
	case "esc":
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
		if f.fields[i].focusable() && f.visible(f.fields[i]) {
			return i
		}
	}
	return f.cur
}

func (f formState) lastFieldIdx() int {
	for i := len(f.fields) - 1; i >= 0; i-- {
		if !f.visible(f.fields[i]) {
			continue
		}
		if f.fields[i].focusable() {
			return i
		}
	}
	return 0
}

func (fl formField) focusable() bool {
	return !fl.readOnly && fl.kind != fieldStatic
}
