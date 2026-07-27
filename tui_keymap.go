package main

import (
	"sort"
	"strings"
)

// The keymap.
//
// One table. The help screen and the status hints are both rendered from it,
// so they cannot drift from each other — and a test checks it against the
// handlers, so neither can drift from what the keys actually do.
//
// There were four places before: the switch statements, a hand-written help
// table, a hand-written status line, and the README. Three of them lied. The
// help advertised `a` for a whole release after the anchor it named had been
// replaced; it advertised ctrl+t, which nothing handled, backed by a register
// nothing read; it claimed `[LL]` appears in the header when it appears beside
// the id in the centre panel, which in a narrow terminal is nowhere at all.
// `q` and ctrl+c were documented nowhere. None of that was carelessness — it
// is what four copies of one fact do on their own.

type binding struct {
	// keys is what the user presses, spelled the way they read it. Aliases
	// share one row ("j k  ↑ ↓") because they are one binding, not four.
	keys string
	desc string

	// mode the binding belongs to. Browse-mode keys are checked against the
	// handlers; the others document modes whose keys are inspected by hand.
	mode uiMode

	// group orders the help screen.
	group string

	// hint, when set, is the short form for the status bar. Only a handful fit
	// on one line, so most bindings live behind `?` alone.
	hint string

	// prio orders the hints, lowest first. It is also the order they are
	// DROPPED in as the terminal narrows — so it has to reflect what a user
	// would miss most, not what reads well in the table.
	prio int
}

var keymap = []binding{
	// ── Browse ────────────────────────────────────────────────────────────
	{keys: "j k  ↑ ↓", desc: "move the cursor", mode: modeBrowse, group: "Navigate", hint: "jk:nav", prio: 1},
	{keys: "h l  ← →", desc: "move between the columns: incoming · the vertex · outgoing", mode: modeBrowse, group: "Navigate", hint: "hl:column", prio: 2},
	{keys: "Enter", desc: "follow the selected link · expand or collapse a type group", mode: modeBrowse, group: "Navigate", hint: "Enter:go", prio: 1},
	{keys: "Tab", desc: "collapse or expand the type group", mode: modeBrowse, group: "Navigate", hint: "Tab:collapse", prio: 4},
	{keys: "b  Backspace", desc: "back to the previous vertex", mode: modeBrowse, group: "Navigate", hint: "b:back", prio: 2},
	{keys: "R", desc: "jump to root and reset history, search and a pending link", mode: modeBrowse, group: "Navigate"},
	{keys: "g G", desc: "scroll the body up / down", mode: modeBrowse, group: "Navigate"},
	{keys: "q", desc: "quit", mode: modeBrowse, group: "Navigate"},
	{keys: "ctrl+c", desc: "quit, from any mode", mode: modeBrowse, group: "Navigate"},

	{keys: "v", desc: "toggle raw JSON vs. key-value body", mode: modeBrowse, group: "View"},
	{keys: "c", desc: "copy the current vertex id to the clipboard", mode: modeBrowse, group: "View"},
	{keys: "/", desc: "JPGQL query from the current vertex", mode: modeBrowse, group: "View"},
	{keys: ":", desc: "go to a vertex by id · Tab cycles the built-in ones", mode: modeBrowse, group: "View", hint: ":goto", prio: 5},
	{keys: "f", desc: "filter the link lists · Esc restores the previous filter", mode: modeBrowse, group: "View"},
	{keys: "e", desc: "export the graph", mode: modeBrowse, group: "View"},
	{keys: "r", desc: "refresh the current vertex", mode: modeBrowse, group: "View"},
	{keys: "ctrl+r", desc: "refresh and clear the whole cache", mode: modeBrowse, group: "View"},
	{keys: "?", desc: "this screen", mode: modeBrowse, group: "View"},

	{keys: "n", desc: "new… — what can be created from where you are standing", mode: modeBrowse, group: "Create", hint: "n:new", prio: 3},
	{keys: "L", desc: "start a link here · press again at the target to commit", mode: modeBrowse, group: "Create", hint: "L:link", prio: 3},
	{keys: "esc", desc: "cancel a pending link", mode: modeBrowse, group: "Create"},

	{keys: "i", desc: "edit what the cursor is on — the selected link, or the vertex", mode: modeBrowse, group: "Modify", hint: "i:edit", prio: 2},
	{keys: "I", desc: "always the current vertex's body", mode: modeBrowse, group: "Modify"},
	{keys: "t", desc: "edit the selected link, starting on its tags", mode: modeBrowse, group: "Modify"},
	{keys: "x", desc: "switch the CRUD API between high-level and low-level", mode: modeBrowse, group: "Modify", hint: "x:api", prio: 6},
	{keys: "y", desc: "yank the body on screen, to paste with ctrl+t", mode: modeBrowse, group: "Modify"},

	{keys: "d", desc: "delete what the cursor is on — the selected link, or the vertex", mode: modeBrowse, group: "Delete", hint: "d:del", prio: 2},
	{keys: "D", desc: "always the current vertex", mode: modeBrowse, group: "Delete"},

	// ── Forms and the body editor ─────────────────────────────────────────
	{keys: "ctrl+s", desc: "apply / submit from any field", mode: modeForm, group: "Editing"},
	{keys: "ctrl+r", desc: "switch MERGE ⇄ REPLACE", mode: modeForm, group: "Editing"},
	{keys: "ctrl+e", desc: "open $EDITOR", mode: modeForm, group: "Editing"},
	{keys: "ctrl+t", desc: "paste the yanked body — offered only when there is one", mode: modeForm, group: "Editing"},
	{keys: "Tab  shift+Tab", desc: "next / previous field", mode: modeForm, group: "Editing"},
	{keys: "← →", desc: "cycle options · toggle a checkbox · swap the link direction", mode: modeForm, group: "Editing"},
	{keys: "space", desc: "toggle a checkbox", mode: modeForm, group: "Editing"},
	{keys: "Enter", desc: "next field, or submit on the last one", mode: modeForm, group: "Editing"},
	{keys: "y n", desc: "confirm / cancel a deletion", mode: modeForm, group: "Editing"},
	{keys: "Esc", desc: "cancel", mode: modeForm, group: "Editing"},

	// ── The other modes ───────────────────────────────────────────────────
	// One group per mode. They used to share a single block, so it listed
	// four unqualified Enter rows and four unqualified Esc rows and the reader
	// had to guess which belonged to what.
	{keys: "Tab", desc: "cycle root · types · objects · trash_can · group · nav", mode: modeGoto, group: "After : — go to"},
	{keys: "Enter", desc: "jump to the id", mode: modeGoto, group: "After : — go to"},
	{keys: "Esc", desc: "cancel", mode: modeGoto, group: "After : — go to"},

	{keys: "Enter", desc: "run it", mode: modeQuery, group: "After / — JPGQL"},
	{keys: "Esc", desc: "cancel", mode: modeQuery, group: "After / — JPGQL"},
	{keys: "j k  Enter", desc: "walk the results, then follow one", mode: modeResults, group: "After / — JPGQL"},
	{keys: "Esc  b", desc: "dismiss the results", mode: modeResults, group: "After / — JPGQL"},

	{keys: "Enter", desc: "keep the filter", mode: modeSearch, group: "After f — filter"},
	{keys: "Esc", desc: "restore the previous filter", mode: modeSearch, group: "After f — filter"},

	{keys: "← → Tab", desc: "choose the format, then a depth preset", mode: modeExport, group: "After e — export"},
	{keys: "Enter", desc: "next step, then run", mode: modeExport, group: "After e — export"},
	{keys: "Esc", desc: "back a step, then out", mode: modeExport, group: "After e — export"},
}

// helpGroups is the display order. A group missing from here would not render,
// so the test that walks the keymap also checks every group is listed.
var helpGroups = []string{
	"Navigate", "View", "Create", "Modify", "Delete", "Editing",
	"After : — go to", "After / — JPGQL", "After f — filter", "After e — export",
}

// browseKeys returns every individual key token bound in browse mode, expanded
// from the alias rows. This is what the handler-agreement test compares against.
func browseKeys() []string {
	var out []string
	for _, b := range keymap {
		if b.mode != modeBrowse {
			continue
		}
		out = append(out, strings.Fields(b.keys)...)
	}
	return out
}

// browseHintsFor is the short form for the status bar, in table order.
//
// Tab is announced only while a link panel has focus, because that is the only
// time it does anything — and a hint for a key that is currently inert is the
// same lie as a hint for a key that does not exist. Everything else applies to
// the subject, whatever the subject happens to be, so it is always shown.
func browseHintsFor(rowSelected bool) []string {
	picked := make([]binding, 0, 12)
	for _, b := range keymap {
		if b.hint == "" {
			continue
		}
		if b.hint == "Tab:collapse" && !rowSelected {
			continue
		}
		picked = append(picked, b)
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].prio < picked[j].prio })

	out := make([]string, len(picked))
	for i, b := range picked {
		out[i] = b.hint
	}
	return out
}

// statusHints is every short form, regardless of context — used by the tests
// that check the help screen covers the status bar.
func statusHints() []string {
	var out []string
	for _, b := range keymap {
		if b.hint != "" {
			out = append(out, b.hint)
		}
	}
	return out
}

// modeHints is the line a mode shows about ITSELF, at the right-hand end of
// the status bar.
//
// chromeCenter forms used to show none at all: the status bar fell through to
// the browse hints, so a form advertised `jk:nav  Enter:go  d:del` while every
// one of those keys was being typed into a text field. Tab in particular
// cycles in four different places and was announced in one of them.
func modeHints(k uiMode) string {
	switch k {
	case modeGoto:
		return "Tab:built-ins  Enter:go  Esc:cancel"
	case modeQuery:
		return "Enter:run  Esc:cancel"
	case modeSearch:
		return "Enter:keep  Esc:restore"
	}
	return ""
}

// formHints describes the keys a form responds to. Confirmations answer y/n or
// a typed word and have their own line; everything else is a field form.
func formHints(f *formState) string {
	if f == nil {
		return ""
	}
	switch {
	case f.kind == formCreateMenu:
		// Letter accelerators, not a field form — there is nothing to Tab
		// between, so nothing claims there is.
		return "a-z:choose  Esc:cancel"
	case f.confirm:
		return "" // renderFormStatus draws its own
	case f.jsonField() != nil:
		h := "ctrl+s:apply  ctrl+r:merge/replace  ctrl+e:$EDITOR  Tab:field  Esc:cancel"
		if f.template != "" {
			h = "ctrl+s:apply  ctrl+r:merge/replace  ctrl+t:paste  Tab:field  Esc:cancel"
		}
		return h
	default:
		return "Tab:next field  ←→:change  Enter:next/submit  ctrl+s:submit  Esc:cancel"
	}
}
