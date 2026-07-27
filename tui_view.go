package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
)

// ── Body rendering ────────────────────────────────────────────────────────────

func (m tuiModel) bodyContent() string {
	if m.fvi == nil {
		if m.errMsg != "" {
			return styleErr.Render("⚠ " + m.errMsg)
		}
		return styleDim.Render("(loading…)")
	}

	var content string
	if m.rawBody {
		if m.fvi.body == nil || !m.fvi.body.IsNonEmptyObject() {
			content = styleDim.Render("  (empty body)")
		} else {
			content = JSONStrPrettyStringAnyway(m.fvi.body, 0, 2)
		}
	} else {
		content = renderBodyKV(m.fvi.body, m.vpWidth())
	}
	return highlightMatches(content, m.searchQuery)
}

// renderBodyKV renders a JSON object as aligned key: value lines.
func renderBodyKV(body *easyjson.JSON, w int) string {
	if body == nil || !body.IsNonEmptyObject() {
		return styleDim.Render("  (empty)")
	}

	obj, ok := body.Value.(map[string]interface{})
	if !ok {
		return JSONStrPrettyStringAnyway(body, 0, 2)
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	maxKeyLen := 0
	for _, k := range keys {
		if kw := lipgloss.Width(k); kw > maxKeyLen {
			maxKeyLen = kw
		}
	}
	if maxKeyLen > 20 {
		maxKeyLen = 20
	}

	valW := w - maxKeyLen - 4
	if valW < 6 {
		valW = 6
	}

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		keyStr := truncateCells(k, maxKeyLen) + ":"
		pad := maxKeyLen + 1 - lipgloss.Width(keyStr)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(styleMetaKey.Render("  " + keyStr + strings.Repeat(" ", pad)))
		sb.WriteString("  ")
		sb.WriteString(renderJSONValue(obj[k], valW))
	}
	return sb.String()
}

func renderJSONValue(v interface{}, maxW int) string {
	if maxW < 4 {
		maxW = 4
	}
	switch val := v.(type) {
	case string:
		return styleMetaVal.Render(truncateCells(val, maxW))
	case float64:
		if val == math.Trunc(val) && math.Abs(val) < 1e18 {
			return styleMetaVal.Render(fmt.Sprintf("%d", int64(val)))
		}
		return styleMetaVal.Render(fmt.Sprintf("%g", val))
	case bool:
		if val {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("true")
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("false")
	case nil:
		return styleDim.Render("null")
	case []interface{}:
		if len(val) == 0 {
			return styleDim.Render("[]")
		}
		strs := make([]string, len(val))
		allStr := true
		for i, item := range val {
			if s, ok := item.(string); ok {
				strs[i] = s
			} else {
				allStr = false
				break
			}
		}
		if allStr {
			joined := "[" + strings.Join(strs, ", ") + "]"
			if lipgloss.Width(joined) <= maxW {
				return styleMetaVal.Render(joined)
			}
		}
		b, _ := json.Marshal(val)
		return styleDim.Render(truncateCells(string(b), maxW))
	default:
		b, _ := json.Marshal(v)
		return styleDim.Render(truncateCells(string(b), maxW))
	}
}

// ── Three-column panels ───────────────────────────────────────────────────────

// renderSidePanel renders either the incoming (isOut=false) or outgoing (isOut=true) side panel.
func (m tuiModel) renderSidePanel(isOut bool) string {
	cw := m.sideContentW()
	ch := m.panelContentH()
	active := (isOut && m.focus == panelOut) || (!isOut && m.focus == panelIn)

	var groups []linkGroup
	var flat []flatItem
	var cursor, offset int
	var dirIcon, dirLabel string
	var headerStyle lipgloss.Style

	if isOut {
		groups = m.grouped.outGroups
		flat = m.grouped.outFlat
		cursor = m.rCursor
		offset = m.rOffset
		dirIcon, dirLabel = "→", "OUTGOING"
		headerStyle = styleOut.Bold(true)
	} else {
		groups = m.grouped.inGroups
		flat = m.grouped.inFlat
		cursor = m.lCursor
		offset = m.lOffset
		dirIcon, dirLabel = "←", "INCOMING"
		headerStyle = styleIn.Bold(true)
	}

	total := 0
	for _, g := range groups {
		total += len(g.links)
	}

	title := headerStyle.Render(dirIcon+" "+dirLabel) + " " + styleDim.Render(fmt.Sprintf("(%d)", total))

	listH := ch - 1
	if listH < 1 {
		listH = 1
	}

	var content string
	if m.loading {
		content = styleLoading.Render("loading…")
	} else if len(groups) == 0 {
		if m.searchQuery != "" {
			content = styleDim.Render("no matches")
		} else {
			content = styleDim.Render("(none)")
		}
	} else {
		content = m.renderPanelLinks(groups, flat, cursor, offset, active, isOut, cw, listH)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)

	ps := stylePanel.Width(cw).Height(ch)
	if active {
		ps = stylePanelActive.Width(cw).Height(ch)
	}
	return ps.Render(inner)
}

// renderPanelLinks renders the link list for one side panel.
func (m tuiModel) renderPanelLinks(groups []linkGroup, flat []flatItem, cursor, offset int, active, isOut bool, w, h int) string {
	lines := make([]string, 0, h)

	for i := offset; i < len(flat) && len(lines) < h; i++ {
		item := flat[i]
		if item.groupIdx >= len(groups) {
			continue
		}
		g := groups[item.groupIdx]
		selected := active && i == cursor

		switch item.kind {
		case flatTypeGroup:
			collapseIcon := "▾"
			if g.collapsed {
				collapseIcon = "▸"
			}
			countStr := fmt.Sprintf("%d", len(g.links))
			// prefix visual width: "  " + icon + " " = 4; right-align count at edge
			tpMax := w - 5 - len(countStr) // 4 prefix + 1 min-fill + count
			if tpMax < 1 {
				tpMax = 1
			}
			tp := truncateCells(g.tp, tpMax)
			tpW := lipgloss.Width(tp)
			fillW := w - 4 - tpW - len(countStr)
			if fillW < 1 {
				fillW = 1
			}
			fill := strings.Repeat(" ", fillW)
			if selected {
				plain := "  " + collapseIcon + " " + tp + fill + countStr
				lines = append(lines, styleSelected.Width(w).Render(plain))
			} else {
				lines = append(lines,
					"  "+styleDim.Render(collapseIcon)+" "+styleTypeHdr.Render(tp)+
						fill+styleDim.Render(countStr),
				)
			}

		case flatLink:
			if item.linkIdx >= len(g.links) {
				continue
			}
			dl := g.links[item.linkIdx]

			// "    name → tgt" — visual overhead = "    "4 + " "1 + "→"1 + " "1 = 7
			avail := w - 7
			if avail < 2 {
				avail = 2
			}
			nameMax := avail * 6 / 10 // ~60% for name
			if nameMax < 1 {
				nameMax = 1
			}
			targetMax := avail - nameMax
			if targetMax < 1 {
				targetMax = 1
			}

			name := dl.label()
			tgt := stripDomain(dl.target())
			rawName := truncateCells(name, nameMax)
			// pad to nameMax so the arrow aligns at a fixed column across all rows
			namePad := strings.Repeat(" ", nameMax-lipgloss.Width(rawName))

			if selected {
				dispName := highlightMatches(rawName, m.searchQuery) + namePad
				dispTarget := highlightMatches(truncateCells(tgt, targetMax), m.searchQuery)
				plain := "  ► " + dispName + " → " + dispTarget
				lines = append(lines, styleSelected.Width(w).Render(plain))
			} else {
				var arrowStr string
				if isOut {
					arrowStr = styleOut.Render("→")
				} else {
					arrowStr = styleIn.Render("←")
				}
				dispName := highlightMatches(rawName, m.searchQuery) + namePad
				dispTarget := highlightMatches(truncateCells(tgt, targetMax), m.searchQuery)
				lines = append(lines, "    "+dispName+" "+arrowStr+" "+dispTarget)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// ── Center panel ──────────────────────────────────────────────────────────────

func (m tuiModel) renderCenterPanel() string {
	cw := m.centerContentW()
	ch := m.panelContentH()

	rawMark := ""
	if m.rawBody {
		rawMark = styleDim.Render(" [raw]")
	}
	llMark := ""
	if m.llMode {
		llMark = styleWarn.Render(" [LL]")
	}
	title := styleTitle.Render("◈ "+truncateCells(m.currentID, cw/2)) + m.vertexKindBadge() + rawMark + llMark

	var body string
	switch {
	case m.form != nil && m.form.chrome == chromeCenter:
		body = m.renderFormCenter(cw, m.vpHeight())
	case len(m.queryResults) > 0:
		body = m.renderQueryResults(cw, m.vpHeight())
	default:
		body = m.bodyVP.View()
	}

	divider := styleDim.Render(strings.Repeat("─", cw))
	typeMap := m.renderTypeMap(cw, m.typeMapH())

	inner := lipgloss.JoinVertical(lipgloss.Left, title, body, divider, typeMap)
	return stylePanel.Width(cw).Height(ch).Render(inner)
}

// renderTypeMap renders a 4-column type-flow map showing two hops in each direction:
//
//	[depth-2 in]  [depth-1 in] ← │ → [depth-1 out]  [depth-2 out]
//
// Depth-2 columns appear only when the async fetch has completed.
func (m tuiModel) renderTypeMap(w, h int) string {
	if h <= 0 || w < 6 {
		return ""
	}

	in1 := m.grouped.inGroups
	out1 := m.grouped.outGroups
	in2 := m.inTypes2
	out2 := m.outTypes2

	rows := len(in1)
	for _, n := range []int{len(out1), len(in2), len(out2)} {
		if n > rows {
			rows = n
		}
	}
	if rows == 0 {
		if m.loading {
			return ""
		}
		return styleDim.Render("(no links)")
	}

	// Total layout: [lhW] │ [rhW]  where lhW + 1 + rhW = w.
	lhW := w / 2
	rhW := w - lhW - 1

	// When depth-2 data is present, split each half ~40 / 60.
	hasD2 := len(in2) > 0 || len(out2) > 0
	var d2inW, d1inW, d1outW, d2outW int
	if hasD2 {
		d2inW = lhW * 4 / 10
		if d2inW < 3 {
			d2inW = 3
		}
		d1inW = lhW - d2inW
		d2outW = rhW * 4 / 10
		if d2outW < 3 {
			d2outW = 3
		}
		d1outW = rhW - d2outW
	} else {
		d1inW = lhW
		d1outW = rhW
	}

	// cellPad right-pads or left-pads a styled string to a fixed visual width.
	padL := func(s string, w int) string {
		p := w - lipgloss.Width(s)
		if p <= 0 {
			return s
		}
		return strings.Repeat(" ", p) + s
	}
	padR := func(s string, w int) string {
		p := w - lipgloss.Width(s)
		if p <= 0 {
			return s
		}
		return s + strings.Repeat(" ", p)
	}

	lines := make([]string, 0, h)
	for i := 0; i < rows && len(lines) < h; i++ {
		// ── depth-1 incoming (right-aligned, coloured ←) ──────────────────────
		var d1in string
		if i < len(in1) {
			tpW := d1inW - 2
			if tpW < 1 {
				tpW = 1
			}
			tp := truncateCells(in1[i].tp, tpW)
			d1in = padL(styleTypeHdr.Render(tp)+" "+styleIn.Render("←"), d1inW)
		} else {
			d1in = strings.Repeat(" ", d1inW)
		}

		// ── depth-2 incoming (right-aligned, dim) ─────────────────────────────
		var d2in string
		if hasD2 {
			if i < len(in2) {
				tpW := d2inW - 2
				if tpW < 1 {
					tpW = 1
				}
				tp := truncateCells(in2[i], tpW)
				d2in = padL(styleDim.Render(tp)+" "+styleDim.Render("←"), d2inW)
			} else {
				d2in = strings.Repeat(" ", d2inW)
			}
		}

		// ── depth-1 outgoing (left-aligned, coloured →) ───────────────────────
		var d1out string
		if i < len(out1) {
			tpW := d1outW - 2
			if tpW < 1 {
				tpW = 1
			}
			tp := truncateCells(out1[i].tp, tpW)
			d1out = padR(styleOut.Render("→")+" "+styleTypeHdr.Render(tp), d1outW)
		} else {
			d1out = strings.Repeat(" ", d1outW)
		}

		// ── depth-2 outgoing (left-aligned, dim) ──────────────────────────────
		var d2out string
		if hasD2 && i < len(out2) {
			tpW := d2outW - 2
			if tpW < 1 {
				tpW = 1
			}
			tp := truncateCells(out2[i], tpW)
			d2out = styleDim.Render("→") + " " + styleDim.Render(tp)
		}

		lines = append(lines, d2in+d1in+styleDim.Render("│")+d1out+d2out)
	}

	return strings.Join(lines, "\n")
}

// ── Query results (shown in center panel body area) ───────────────────────────

func (m tuiModel) renderQueryResults(w, h int) string {
	lines := make([]string, 0, h)
	for i := m.qOffset; i < len(m.queryResults) && len(lines) < h; i++ {
		id := m.queryResults[i]
		if i == m.qCursor {
			lines = append(lines, styleSelected.Width(w).Render("▶ "+truncateCells(id, w-2)))
		} else {
			lines = append(lines, "  "+truncateCells(id, w-2))
		}
	}
	return strings.Join(lines, "\n")
}

// ── Body panel badge ──────────────────────────────────────────────────────────

// vertexKindBadge renders the classification computed by vertexKind
// (tui_model.go). Behaviour is unchanged from when the classification lived
// here inline.
func (m tuiModel) vertexKindBadge() string {
	switch k, typeName := m.vertexKind(); k {
	case vkType:
		return " " + styleMetaKey.Render("[type]")
	case vkObject:
		return " " + styleMetaKey.Render("["+typeName+"]")
	}
	return ""
}

// ── Narrow layout (single-column fallback) ────────────────────────────────────

func (m tuiModel) viewNarrow() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	listH := m.height - 1 - m.breadcrumbH() - 2
	if listH < 1 {
		listH = 1
	}
	divider := styleDim.Render(strings.Repeat("─", w))

	var links string
	if m.loading {
		links = styleLoading.Render("loading…")
	} else if len(m.links) == 0 {
		links = styleDim.Render("(no links)")
	} else {
		links = m.renderPanelLinks(
			m.grouped.outGroups, m.grouped.outFlat,
			m.rCursor, m.rOffset, m.focus == panelOut, true, w, listH/2,
		)
		links += "\n" + m.renderPanelLinks(
			m.grouped.inGroups, m.grouped.inFlat,
			m.lCursor, m.lOffset, m.focus == panelIn, false, w, listH/2,
		)
	}

	rows := []string{m.renderHeader(), divider, links}
	if bc := m.renderBreadcrumbs(); bc != "" {
		rows = append(rows, bc)
	}
	rows = append(rows, m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m tuiModel) View() string {
	if !m.ready || m.width == 0 {
		return "Starting…"
	}
	if m.helpOpen {
		return m.renderHelp()
	}
	// A full-screen form owns the whole window; it needs the room, and the
	// narrow fallback has no centre panel to host anything smaller.
	if m.form != nil && m.form.chrome == chromeFull {
		return m.renderFormFull()
	}
	if m.isNarrow() {
		return m.viewNarrow()
	}

	panels := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderSidePanel(false), // left: incoming
		m.renderCenterPanel(),
		m.renderSidePanel(true), // right: outgoing
	)

	rows := []string{m.renderHeader(), panels}
	if bc := m.renderBreadcrumbs(); bc != "" {
		rows = append(rows, bc)
	}
	rows = append(rows, m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// ── Search highlight ──────────────────────────────────────────────────────────

func highlightMatches(text, query string) string {
	if query == "" {
		return text
	}
	lq := strings.ToLower(query)
	lt := strings.ToLower(text)
	var b strings.Builder
	i := 0
	for i < len(lt) {
		idx := strings.Index(lt[i:], lq)
		if idx < 0 {
			b.WriteString(text[i:])
			break
		}
		b.WriteString(text[i : i+idx])
		b.WriteString(styleSearch.Render(text[i+idx : i+idx+len(lq)]))
		i += idx + len(lq)
	}
	return b.String()
}

// ── Header, breadcrumbs, status ───────────────────────────────────────────────

func stripDomain(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func (m tuiModel) breadcrumbs() string {
	if len(m.history) == 0 || m.width <= 0 {
		return ""
	}
	const sep = " ← "
	labels := make([]string, len(m.history))
	for i, id := range m.history {
		labels[len(m.history)-1-i] = stripDomain(id)
	}
	sepW := lipgloss.Width(sep)
	used := sepW
	count := 0
	for _, lbl := range labels {
		need := lipgloss.Width(lbl)
		if count > 0 {
			need += sepW
		}
		if used+need > m.width {
			break
		}
		used += need
		count++
	}
	if count == 0 {
		return ""
	}
	return styleDim.Render(sep + strings.Join(labels[:count], sep))
}

func (m tuiModel) renderHeader() string {
	load := ""
	if m.loading {
		if m.linksTotal > 0 {
			load = " " + styleLoading.Render(fmt.Sprintf("loading %d links…", m.linksTotal))
		} else {
			load = " " + styleLoading.Render("loading…")
		}
	}
	// Cap the vertex ID so the header never wraps to a second line.
	idMax := m.width - 2 - lipgloss.Width(load) // 2 = "◈ " prefix
	if idMax < 4 {
		idMax = 4
	}
	header := styleHeader.Render("◈ "+truncateCells(m.currentID, idMax)) + load
	if m.errMsg != "" {
		errMax := m.width - lipgloss.Width(header) - 4 // 4 = "  ⚠ "
		if errMax > 4 {
			header += "  " + styleErr.Render("⚠ "+truncateCells(m.errMsg, errMax))
		}
	}
	return lipgloss.NewStyle().Width(m.width).Render(header)
}

func (m tuiModel) renderBreadcrumbs() string {
	bc := m.breadcrumbs()
	if m.linking == nil {
		if bc == "" {
			return ""
		}
		return lipgloss.NewStyle().Width(m.width).Render(bc)
	}

	// A pending link is the single most important thing on screen while it
	// lasts: it changes what the next keypress means. It gets the whole row,
	// states both endpoints, and names its two exits — the history crumbs are
	// decoration by comparison and give way first.
	from := stripDomain(m.linking.fromID)
	to := stripDomain(m.currentID)
	target := styleDim.Render("(walk to the target)")
	if m.currentID != m.linking.fromID {
		target = styleMetaVal.Render(to)
	}

	banner := styleWarn.Render(" ◆ LINK PENDING ") + "  " +
		styleMetaVal.Render(from) + styleOut.Render("  ──▶  ") + target +
		styleHintSep.Render("   ") + hint("L", "commit") + styleHintSep.Render("  ") + hint("Esc", "cancel")

	return lipgloss.NewStyle().Width(m.width).Render(truncateCells(banner, m.width))
}

func hint(key, desc string) string {
	return styleHintKey.Render(key) + styleHintSep.Render(":"+desc)
}

func (m tuiModel) renderStatus() string {
	var s string
	sep := styleHintSep.Render("  ")
	switch {
	case m.form != nil && m.form.chrome == chromeStatus:
		s = m.renderFormStatus()
	case m.queryMode:
		s = "Query: " + m.queryInput.View() + styleHintSep.Render("  Esc:cancel")
	case m.exportMode && !m.exportDepStep:
		parts := make([]string, len(exportFmts))
		for i, f := range exportFmts {
			if i == m.exportFmtIdx {
				parts[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("[" + f.label + "]")
			} else {
				parts[i] = styleDim.Render(f.label)
			}
		}
		s = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("Export") +
			"  format: " + strings.Join(parts, "  ") +
			styleHintSep.Render("  ←→ Tab:change  Enter:→depth  Esc:cancel")
	case m.exportMode && m.exportDepStep:
		depthParts := make([]string, len(exportDepthPresets))
		for i, p := range exportDepthPresets {
			lbl := p
			if lbl == "" {
				lbl = "all"
			}
			if i == m.exportDepthIdx {
				depthParts[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("[" + lbl + "]")
			} else {
				depthParts[i] = styleDim.Render(lbl)
			}
		}
		s = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("Export") +
			"  " + exportFmts[m.exportFmtIdx].label +
			"  depth: " + m.exportInput.View() +
			"  " + strings.Join(depthParts, " ") +
			styleHintSep.Render("  Tab:preset  Enter:export  Esc:back")
	case m.searchMode:
		s = "Search: " + m.searchInput.View() + styleHintSep.Render("  Enter:keep  Esc:clear")
	case len(m.queryResults) > 0:
		s = strings.Join([]string{
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("Esc/b", "close"),
			hint("q", "quit"),
		}, sep)
	case m.queryResult != "":
		s = "↳ " + m.queryResult
	case m.searchQuery != "":
		s = styleSearch.Render(" /"+m.searchQuery+" ") + sep + strings.Join([]string{
			hint("f", "edit"),
			hint("Esc", "clear"),
			hint("jk", "navigate"),
			hint("Enter/Tab", "go/collapse"),
			hint("q", "quit"),
		}, sep)
	case m.errMsg != "":
		s = hint("r", "retry") + "  " + hint("R", "→ root") + "  " + hint("q", "quit")
	default:
		// Deliberately short. There are ~25 bindings now and they cannot all
		// fit on one line at 80 columns; the full list lives behind `?`.
		// These are the ones used constantly.
		parts := []string{
			hint("jk", "nav"),
			hint("Enter", "go"),
			hint("b", "back"),
			hint("n", "new"),
			hint("i", "edit"),
			hint("d", "del"),
		}
		if m.linking != nil {
			parts = append(parts, hint("L", "commit link"))
		} else {
			parts = append(parts, hint("L", "link"))
		}
		parts = append(parts, hint("?", "help"))
		s = strings.Join(parts, sep)
	}
	return styleStatus.Width(m.width).Render(s)
}

func truncateCells(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > max-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString("…")
	return b.String()
}
