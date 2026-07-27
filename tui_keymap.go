package main

import "strings"

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
}

var keymap = []binding{
	// ── Browse ────────────────────────────────────────────────────────────
	{keys: "j k  ↑ ↓", desc: "move the cursor", mode: modeBrowse, group: "Navigate", hint: "jk:nav"},
	{keys: "h l  ← →", desc: "switch between the incoming and outgoing panel", mode: modeBrowse, group: "Navigate"},
	{keys: "Enter", desc: "follow the selected link · expand or collapse a type group", mode: modeBrowse, group: "Navigate", hint: "Enter:go"},
	{keys: "Tab", desc: "collapse or expand the type group", mode: modeBrowse, group: "Navigate"},
	{keys: "b  Backspace", desc: "back to the previous vertex", mode: modeBrowse, group: "Navigate", hint: "b:back"},
	{keys: "R", desc: "jump to root and reset history, search and a pending link", mode: modeBrowse, group: "Navigate"},
	{keys: "g G", desc: "scroll the body up / down", mode: modeBrowse, group: "Navigate"},
	{keys: "q", desc: "quit", mode: modeBrowse, group: "Navigate"},
	{keys: "ctrl+c", desc: "quit, from any mode", mode: modeBrowse, group: "Navigate"},

	{keys: "v", desc: "toggle raw JSON vs. key-value body", mode: modeBrowse, group: "View"},
	{keys: "c", desc: "copy the current vertex id to the clipboard", mode: modeBrowse, group: "View"},
	{keys: "/", desc: "JPGQL query from the current vertex", mode: modeBrowse, group: "View"},
	{keys: "f", desc: "filter the link lists · Esc restores the previous filter", mode: modeBrowse, group: "View"},
	{keys: "e", desc: "export the graph", mode: modeBrowse, group: "View"},
	{keys: "r", desc: "refresh the current vertex", mode: modeBrowse, group: "View"},
	{keys: "ctrl+r", desc: "refresh and clear the whole cache", mode: modeBrowse, group: "View"},
	{keys: "?", desc: "this screen", mode: modeBrowse, group: "View"},

	{keys: "n", desc: "new… — what can be created from where you are standing", mode: modeBrowse, group: "Create", hint: "n:new"},
	{keys: "L", desc: "start a link here · press again at the target to commit", mode: modeBrowse, group: "Create", hint: "L:link"},
	{keys: "esc", desc: "cancel a pending link", mode: modeBrowse, group: "Create"},

	{keys: "i", desc: "edit what the cursor is on — the selected link, or the vertex", mode: modeBrowse, group: "Modify", hint: "i:edit"},
	{keys: "I", desc: "always the current vertex's body", mode: modeBrowse, group: "Modify"},
	{keys: "t", desc: "edit the selected link, starting on its tags", mode: modeBrowse, group: "Modify"},
	{keys: "x", desc: "toggle the low-level API — marked [LL] beside the id", mode: modeBrowse, group: "Modify"},
	{keys: "y", desc: "yank the body on screen, to paste with ctrl+t", mode: modeBrowse, group: "Modify"},

	{keys: "d", desc: "delete what the cursor is on — the selected link, or the vertex", mode: modeBrowse, group: "Delete", hint: "d:del"},
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
	{keys: "Enter", desc: "run the query", mode: modeQuery, group: "Query · filter · export"},
	{keys: "Esc", desc: "cancel the query", mode: modeQuery, group: "Query · filter · export"},
	{keys: "Enter", desc: "keep the filter", mode: modeSearch, group: "Query · filter · export"},
	{keys: "Esc", desc: "restore the previous filter", mode: modeSearch, group: "Query · filter · export"},
	{keys: "← → Tab", desc: "choose the export format, then a depth preset", mode: modeExport, group: "Query · filter · export"},
	{keys: "Esc", desc: "back a step, then out", mode: modeExport, group: "Query · filter · export"},
	{keys: "j k  Enter", desc: "walk the query results", mode: modeResults, group: "Query · filter · export"},
	{keys: "Esc  b", desc: "dismiss the results", mode: modeResults, group: "Query · filter · export"},
}

// helpGroups is the display order. A group missing from here would not render,
// so the test that walks the keymap also checks every group is listed.
var helpGroups = []string{"Navigate", "View", "Create", "Modify", "Delete", "Editing", "Query · filter · export"}

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

// statusHints is the short form for the status bar, in table order.
func statusHints() []string {
	var out []string
	for _, b := range keymap {
		if b.hint != "" {
			out = append(out, b.hint)
		}
	}
	return out
}
