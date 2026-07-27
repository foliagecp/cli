package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Form rendering. Three tiers, because the TUI has no overlay primitive and the
// status bar is exactly one line — a second line there would silently corrupt
// every panel's computed height.

// editorSize returns the textarea dimensions inside the centre column: the
// panel minus the form's own chrome — title, the mode and validity lines, the
// hint line — and whatever rows the form's other fields take.
func (m tuiModel) editorSize() (int, int) { return m.editorSizeFor(0) }

func (m tuiModel) editorSizeFor(extraRows int) (int, int) {
	w := m.centerContentW() - 2
	if m.isNarrow() {
		w = m.width - 2
	}
	if w < 16 {
		w = 16
	}
	h := m.panelContentH() - 7 - extraRows
	if h < 3 {
		h = 3
	}
	return w, h
}

// renderFormCenter draws a form in the centre column — every form, including
// the body editor.
//
// There used to be a second, full-screen renderer for anything with a JSON
// editor, so creating a link and editing one happened in visibly different
// places for no reason the user could infer. The centre column is where the
// subject lives, and a form is the subject in edit mode, so that is where it
// belongs. The side panels stay visible on purpose: filling in a link form,
// the graph you navigated through to get here is still on screen.
func (m tuiModel) renderFormCenter(w, h int) string {
	f := m.form
	if f == nil {
		return ""
	}

	if f.confirm {
		return m.renderConfirm(f, w, h)
	}

	lines := []string{styleTitle.Render(truncateCells(f.title, w)), ""}
	// Facts the form is ABOUT, as opposed to what it will write: how the edge
	// is addressed, which cannot be changed here.
	for _, r := range f.contextRows {
		lines = append(lines, truncateCells(styleDim.Render("  "+r), w))
	}
	if len(f.contextRows) > 0 {
		lines = append(lines, "")
	}

	labelW := 0
	for _, fl := range f.fields {
		if lw := lipgloss.Width(fl.label); lw > labelW {
			labelW = lw
		}
	}

	for i, fl := range f.fields {
		if !f.visible(fl) {
			continue
		}
		focused := i == f.cur
		label := fl.label + strings.Repeat(" ", labelW-lipgloss.Width(fl.label))
		if focused {
			label = styleHintKey.Render("▸ " + label)
		} else {
			label = styleDim.Render("  " + label)
		}

		if fl.kind == fieldJSON && fl.json != nil {
			lines = append(lines, m.renderJSONField(fl, focused, w)...)
			continue
		}

		var val string
		switch fl.kind {
		case fieldEnum:
			val = renderEnum(fl, w-labelW-6)
		case fieldBool:
			if fl.boolVal {
				val = styleOk.Render("[x]")
			} else {
				val = styleDim.Render("[ ]")
			}
		case fieldStatic:
			val = styleDim.Render(truncateCells(fl.static, w-labelW-6))
		case fieldVertex:
			val = styleMetaVal.Render(stripDomain(f.ctx.fromID)) +
				styleOut.Render("  ──▶  ") +
				styleMetaVal.Render(stripDomain(f.ctx.toID)) +
				styleDim.Render("   (← → swaps)")
		case fieldTags:
			val = renderTagChips(fl.value)
			if focused {
				val += styleHintSep.Render("▏")
			}
		default:
			val = styleMetaVal.Render(truncateCells(fl.value, w-labelW-6))
			if focused {
				val += styleHintSep.Render("▏")
			}
		}

		row := label + "  " + val
		lines = append(lines, truncateCells(row, w))

		// An enum's hint belongs to the OPTION, not the field: the whole reason
		// to offer a choice is that the options mean different things.
		hint := fl.hint
		if fl.kind == fieldEnum && fl.optIdx < len(fl.options) && fl.options[fl.optIdx].hint != "" {
			hint = fl.options[fl.optIdx].hint
		}
		if fl.err != "" {
			lines = append(lines, truncateCells(styleErr.Render(strings.Repeat(" ", labelW+4)+"✗ "+fl.err), w))
		} else if hint != "" {
			lines = append(lines, truncateCells(styleDim.Render(strings.Repeat(" ", labelW+4)+hint), w))
		}
	}

	if f.err != "" {
		lines = append(lines, "", truncateCells(styleErr.Render("✗ "+f.err), w))
	}

	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}

func renderEnum(fl formField, w int) string {
	if len(fl.options) == 0 {
		return styleErr.Render("(no options)")
	}
	cur := fl.options[fl.optIdx]
	pos := ""
	if len(fl.options) > 1 {
		pos = styleDim.Render(fmt.Sprintf("  ← %d/%d →", fl.optIdx+1, len(fl.options)))
	}
	return styleMetaVal.Render(truncateCells(cur.label, w)) + pos
}

func renderTagChips(raw string) string {
	tags := splitTags(raw)
	if len(tags) == 0 {
		return styleDim.Render("(none)")
	}
	chips := make([]string, len(tags))
	for i, t := range tags {
		chips[i] = styleSearch.Render(" " + t + " ")
	}
	return strings.Join(chips, " ")
}

// ── Help ──────────────────────────────────────────────────────────────────────

// renderHelp draws the full keymap, in as many columns as fit and scrollable
// when it still does not.
//
// Two earlier attempts each fixed one half and broke the other: a hardcoded
// column width silently clipped the longest descriptions, and measuring the
// widest description made the columns so wide that two never fit. Both come
// from deciding the width first and the layout second. The description column
// is capped and long ones WRAP, so the width is a choice rather than a
// measurement — and the height is handled by scrolling instead of by hoping.
func (m tuiModel) renderHelp() string {
	const descCap = 54

	keyW := 0
	for _, b := range keymap {
		if w := lipgloss.Width(b.keys); w > keyW {
			keyW = w
		}
	}

	var blocks []string
	for _, g := range helpGroups {
		lines := []string{styleTypeHdr.Render(g)}
		for _, b := range keymap {
			if b.group != g {
				continue
			}
			pad := strings.Repeat(" ", keyW-lipgloss.Width(b.keys))
			indent := strings.Repeat(" ", keyW+4)
			for i, seg := range wrapWords(b.desc, descCap) {
				if i == 0 {
					lines = append(lines, "  "+styleHintKey.Render(b.keys)+pad+"  "+styleDim.Render(seg))
				} else {
					lines = append(lines, indent+styleDim.Render(seg))
				}
			}
		}
		if len(lines) == 1 {
			continue
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	colW := keyW + descCap + 6
	cols := (m.width - 2) / colW
	if cols < 1 {
		cols = 1
	}
	if cols > len(blocks) {
		cols = len(blocks)
	}

	body := strings.Join(blocks, "\n\n")
	if cols > 1 {
		body = joinColumns(blocks, cols, colW)
	}

	out := []string{styleTitle.Render("◈ Keys"), ""}
	out = append(out, strings.Split(body, "\n")...)
	out = append(out, "", styleDim.Render("Creating and linking also work from the shell: "+
		"foliage-cli graph … and foliage-cli cmdb … (an omitted id means the gwalk cursor)"))

	// Every line is clipped to the terminal width. JoinVertical pads all lines
	// to the widest one, so a single over-long line would otherwise push the
	// whole screen past the edge and wrap it into an unreadable mess.
	for i, line := range out {
		out[i] = truncateCells(line, m.width)
	}

	// Vertical: show the window the scroll offset selects, and say when there
	// is more. A keymap that runs off the bottom of the screen is a keymap
	// whose last entries do not exist.
	viewH := m.height - 1
	if viewH < 1 {
		viewH = 1
	}
	hint := "any key closes"
	if len(out) > viewH {
		off := m.helpOffset
		if max := len(out) - viewH; off > max {
			off = max
		}
		if off < 0 {
			off = 0
		}
		out = out[off : off+viewH]
		hint = fmt.Sprintf("jk:scroll   %d–%d of %d   any other key closes",
			off+1, off+viewH, off+viewH+(len(out)-viewH))
	}
	out = append(out, styleStatus.Width(m.width).Render(styleHintSep.Render(hint)))

	return lipgloss.JoinVertical(lipgloss.Left, out...)
}

// wrapWords breaks s at word boundaries into segments of at most w cells.
func wrapWords(s string, w int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	cur := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(cur)+1+lipgloss.Width(word) > w {
			out = append(out, cur)
			cur = word
			continue
		}
		cur += " " + word
	}
	return append(out, cur)
}

// renderFormStatus draws a one-line form in the status bar: confirmations and
// single-field prompts.

// joinColumns lays blocks out in n columns, filling each to roughly equal
// height rather than splitting the list in half — the groups differ enough in
// size that halving leaves one column twice as long as the other, which is
// what pushes the screen past the bottom.
func joinColumns(blocks []string, n, colW int) string {
	total := 0
	for _, b := range blocks {
		total += strings.Count(b, "\n") + 2 // the block plus its blank separator
	}
	target := total / n

	cols := make([]string, 0, n)
	cur := make([]string, 0, len(blocks))
	curH := 0
	for i, b := range blocks {
		h := strings.Count(b, "\n") + 2
		// Start a new column once this one is full, provided enough blocks
		// remain to fill the columns that are left.
		if curH > 0 && curH+h > target && len(cols) < n-1 && len(blocks)-i >= n-len(cols) {
			cols = append(cols, strings.Join(cur, "\n\n"))
			cur, curH = cur[:0:0], 0
		}
		cur = append(cur, b)
		curH += h
	}
	cols = append(cols, strings.Join(cur, "\n\n"))

	rendered := make([]string, len(cols))
	for i, c := range cols {
		if i == len(cols)-1 {
			rendered[i] = c
		} else {
			rendered[i] = lipgloss.NewStyle().Width(colW).Render(c)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

// renderJSONField draws the body editor inline in the form, with the mode line
// and the validity line that used to live in the full-screen chrome.
func (m tuiModel) renderJSONField(fl formField, focused bool, w int) []string {
	ed := fl.json

	// The consequence is spelled out rather than the flag name: "MERGE" tells
	// the user nothing, "cannot remove a key" tells them why their deletion
	// did not take effect.
	var mode string
	if ed.replace {
		mode = styleWarn.Render("REPLACE") + styleDim.Render("  replaces the whole body")
	} else {
		mode = styleOk.Render("MERGE") + styleDim.Render("  cannot remove a key")
	}

	head := styleDim.Render("  body  ")
	if focused {
		head = styleHintKey.Render("▸ body  ")
	}
	out := []string{truncateCells(head+mode, w)}
	out = append(out, strings.Split(ed.ta.View(), "\n")...)

	if ed.valid() {
		out = append(out, truncateCells(styleOk.Render("  ✓ valid JSON object"), w))
	} else {
		out = append(out, truncateCells(styleErr.Render("  ✗ "+ed.parseErr), w))
	}
	if names := ed.reservedNames(); len(names) > 0 {
		out = append(out, truncateCells(
			styleDim.Render("  preserved (not editable): "+strings.Join(names, " · ")), w))
	}
	return out
}

// renderConfirm draws a destructive confirmation in the centre column.
//
// It used to be one line in the status bar, and that line said "type the
// name". Which name? For a types-link the word is the OWNER TYPE's, because
// the deletion cascades into every object of it — so typing the LINK's name,
// the obvious reading, fails every time with no correction on screen. And the
// whole thing was squeezed onto one row with a long explanation in front of
// it, so the prompt was the first thing to be truncated away.
//
// So: the same place as every other form, the explanation wrapped rather than
// clipped, and the exact word to type shown next to the box you type it in.
func (m tuiModel) renderConfirm(f *formState, w, h int) string {
	lines := []string{styleWarn.Render("⚠  Confirm"), ""}
	for _, seg := range wrapWords(f.title, w-2) {
		lines = append(lines, truncateCells("  "+styleMetaVal.Render(seg), w))
	}
	lines = append(lines, "")

	if f.confirmWord == "" {
		lines = append(lines, truncateCells("  "+styleDim.Render("Delete it?"), w))
	} else {
		// Naming the word IS the safeguard. Hiding it would not make the
		// deletion safer, only harder to complete correctly.
		lines = append(lines, truncateCells("  "+styleDim.Render("Type ")+
			styleWarn.Render(f.confirmWord)+styleDim.Render(" to confirm:"), w))
		typed := f.confirmText
		if typed == "" {
			typed = styleDim.Render("…")
		} else if typed == f.confirmWord {
			typed = styleOk.Render(typed)
		} else {
			typed = styleErr.Render(typed)
		}
		lines = append(lines, truncateCells("    "+typed+styleHintSep.Render("▏"), w))
	}

	if f.err != "" {
		lines = append(lines, "", truncateCells("  "+styleErr.Render("✗ "+f.err), w))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}
