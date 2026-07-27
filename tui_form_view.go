package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Form rendering. Three tiers, because the TUI has no overlay primitive and the
// status bar is exactly one line — a second line there would silently corrupt
// every panel's computed height.

// editorSize returns the textarea dimensions for a full-screen form:
// the whole window minus title, footer strip, hint line and borders.
func (m tuiModel) editorSize() (int, int) {
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	h := m.height - 6
	if h < 3 {
		h = 3
	}
	return w, h
}

// renderFormFull draws a full-screen form — currently the JSON body editor.
func (m tuiModel) renderFormFull() string {
	f := m.form
	if f == nil {
		return ""
	}
	ed := f.editor()
	if ed == nil {
		return styleErr.Render("form has no editor")
	}

	title := styleTitle.Render("◈ " + truncateCells(f.title, m.width-2))

	// Mode line. The consequence is spelled out rather than the flag name:
	// "MERGE" tells the user nothing, "cannot remove a key" tells them why
	// their deletion did not take effect.
	var mode string
	if ed.replace {
		mode = styleWarn.Render("REPLACE") +
			styleDim.Render("  the whole body is replaced; preserved paths are re-attached")
	} else {
		mode = styleOk.Render("MERGE") +
			styleDim.Render("  deep merge; arrays are appended — cannot remove a key or an element")
	}

	// Validation line.
	var status string
	if ed.valid() {
		status = styleOk.Render("✓ valid JSON object")
	} else {
		status = styleErr.Render("✗ " + ed.parseErr)
	}

	// Preserved-paths strip, so nothing looks lost.
	preserved := ""
	if names := ed.reservedNames(); len(names) > 0 {
		preserved = styleDim.Render("preserved (not editable): " + strings.Join(names, " · "))
	}

	sep := styleHintSep.Render("  ")
	hints := strings.Join([]string{
		hint("ctrl+s", "apply"),
		hint("ctrl+r", "merge/replace"),
		hint("ctrl+e", "$EDITOR"),
		hint("Esc", "cancel"),
	}, sep)

	rows := []string{title, mode, ed.ta.View(), status}
	if preserved != "" {
		rows = append(rows, preserved)
	}
	if f.err != "" {
		rows = append(rows, styleErr.Render("✗ "+f.err))
	}
	rows = append(rows, styleStatus.Width(m.width).Render(hints))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderFormStatus draws a one-line form in the status bar: confirmations and
// single-field prompts.
func (m tuiModel) renderFormStatus() string {
	f := m.form
	if f == nil {
		return ""
	}
	sep := styleHintSep.Render("  ")

	if f.confirm {
		prompt := styleWarn.Render(f.title)
		if f.confirmWord != "" {
			return prompt + "  " + f.confirmText + styleHintSep.Render("▏") +
				sep + styleHintSep.Render("type the name, Enter:confirm  Esc:cancel")
		}
		return prompt + sep + styleHintSep.Render("y:confirm  n/Esc:cancel")
	}
	return styleTitle.Render(f.title) + sep + styleHintSep.Render("Esc:cancel")
}
