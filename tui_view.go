package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── View ──────────────────────────────────────────────────────────────────────

func (m tuiModel) bodyContent() string {
	if m.fvi == nil {
		if m.errMsg != "" {
			return styleErr.Render("⚠ " + m.errMsg)
		}
		return styleDim.Render("(loading…)")
	}

	var bodyStr string
	if m.fvi.body == nil || !m.fvi.body.IsNonEmptyObject() {
		bodyStr = styleDim.Render("(empty body)")
	} else {
		bodyStr = JSONStrPrettyString(m.fvi.body, 0, 2)
	}

	var result string
	if !m.showAll || len(m.links) == 0 {
		result = bodyStr
	} else {
		var sb strings.Builder
		sb.WriteString(bodyStr)

		outCount, inCount := m.outCount(), m.inCount()
		outIdx, inIdx := 0, 0
		for _, dl := range m.links {
			sb.WriteString("\n\n")
			if dl.isOut {
				outIdx++
				sb.WriteString(styleOut.Render(fmt.Sprintf("→ [%d/%d] %s → %s", outIdx, outCount, dl.label(), dl.target())))
			} else {
				inIdx++
				sb.WriteString(styleIn.Render(fmt.Sprintf("← [%d/%d] %s ← %s", inIdx, inCount, dl.label(), dl.target())))
			}
			if dl.info.tp != "" {
				sb.WriteString("\n  " + styleMetaKey.Render("type:") + " " + styleMetaVal.Render(dl.info.tp))
			}
			if len(dl.info.tags) > 0 {
				sb.WriteString("\n  " + styleTagKey.Render("tags:") + " " + styleMetaVal.Render(strings.Join(dl.info.tags, " ")))
			}
			if dl.info.body != nil && dl.info.body.IsNonEmptyObject() {
				sb.WriteString("\n  " + JSONStrPrettyString(dl.info.body, 2, 2))
			}
		}
		result = sb.String()
	}
	return highlightMatches(result, m.searchQuery)
}

// vertexKindBadge returns a styled badge for the current vertex:
//   - "[type]"          if it has a link to the "types" vertex
//   - "[<__type>]"      if it has a link to the "objects" vertex (shows its type)
//   - ""                otherwise
func (m tuiModel) vertexKindBadge() string {
	isObject, isType := false, false
	typeName := ""
	typesID := NatsHubDomain + "/types"
	objectsID := NatsHubDomain + "/objects"
	for _, dl := range m.links {
		target := dl.target()
		if target == typesID {
			isType = true
		}
		if target == objectsID {
			isObject = true
		}
		if dl.isOut && dl.info.tp == "__type" {
			typeName = target
		}
	}
	if isType {
		return " " + styleMetaKey.Render("[type]")
	}
	if isObject && typeName != "" {
		return " " + styleMetaKey.Render("["+typeName+"]")
	}
	return ""
}

func (m tuiModel) renderBodyPanel() string {
	innerW := m.bodyOuterW() - 2
	innerH := m.panelContentH()

	allMark := ""
	if m.showAll {
		allMark = styleDim.Render(" [+all]")
	}
	title := styleTitle.Render("◈ "+m.currentID) + m.vertexKindBadge() + allMark
	content := m.bodyVP.View()

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return stylePanel.
		Width(innerW).
		Height(innerH + 1). // +1 for title line
		Render(inner)
}

func (m tuiModel) renderLinksPanel() string {
	innerW := m.linksOuterW() - 2
	innerH := m.panelContentH()
	listH := innerH - 1 // -1 for title

	var title, content string

	if len(m.queryResults) > 0 {
		title = styleTitle.Render(fmt.Sprintf("Query results (%d)", len(m.queryResults)))
		content = m.renderQueryResults(innerW-2, listH)
	} else {
		title = styleTitle.Render(
			fmt.Sprintf("Links  %s %s",
				styleOut.Render(fmt.Sprintf("↓out:%d", m.outCount())),
				styleIn.Render(fmt.Sprintf("↑in:%d", m.inCount())),
			),
		)
		if m.loading {
			if m.linksTotal > 0 {
				content = styleLoading.Render(fmt.Sprintf("loading %d links…", m.linksTotal))
			} else {
				content = styleLoading.Render("loading…")
			}
		} else if len(m.links) == 0 {
			content = styleDim.Render("(no links)")
		} else {
			content = m.renderLinkList(innerW-2, listH)
		}
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content)
	return stylePanel.
		Width(innerW).
		Height(innerH + 1).
		Render(inner)
}

func (m tuiModel) renderQueryResults(w, h int) string {
	lines := make([]string, 0, h)
	for i := m.qOffset; i < len(m.queryResults) && len(lines) < h; i++ {
		id := m.queryResults[i]
		if i == m.qCursor {
			lines = append(lines, styleSelected.Width(w).Render("▶ "+id))
		} else {
			lines = append(lines, "  "+id)
		}
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) renderLinkList(w, h int) string {
	lines := make([]string, 0, h)

	for i := m.linkOff; i < len(m.links) && len(lines) < h; i++ {
		dl := m.links[i]

		var dir string
		if dl.isOut {
			dir = styleOut.Render("→")
		} else {
			dir = styleIn.Render("←")
		}

		name := dl.label()
		target := dl.target()
		tp := ""
		if dl.info.tp != "" {
			tp = styleDim.Render(" [" + dl.info.tp + "]")
		}

		// Truncate target if needed
		maxT := w - len(name) - 8
		if maxT < 6 {
			maxT = 6
		}
		if len(target) > maxT {
			target = target[:maxT-1] + "…"
		}

		if i == m.cursor {
			dirPlain := "→"
			if !dl.isOut {
				dirPlain = "←"
			}
			tpPlain := ""
			if dl.info.tp != "" {
				tpPlain = " [" + dl.info.tp + "]"
			}
			hName := highlightMatches(fmt.Sprintf("%-16s", name), m.searchQuery)
			hTarget := highlightMatches(target, m.searchQuery)
			lines = append(lines, styleSelected.Width(w).Render(
				fmt.Sprintf("▶ %s %s %s%s", dirPlain, hName, hTarget, tpPlain),
			))
		} else {
			hName := highlightMatches(fmt.Sprintf("%-16s", name), m.searchQuery)
			hTarget := highlightMatches(target, m.searchQuery)
			lines = append(lines, fmt.Sprintf("  %s %s %s%s", dir, hName, hTarget, tp))
		}
	}

	return strings.Join(lines, "\n")
}

func (m tuiModel) outCount() int {
	if m.fvi == nil {
		return 0
	}
	return len(m.fvi.outLinks)
}

func (m tuiModel) inCount() int {
	if m.fvi == nil {
		return 0
	}
	return len(m.fvi.inLinks)
}

// stripDomain removes the "domain/" prefix from a vertex ID.
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
	// Build crumb labels most-recent-first (no domain).
	labels := make([]string, len(m.history))
	for i, id := range m.history {
		labels[len(m.history)-1-i] = stripDomain(id)
	}
	// Fit as many crumbs as possible within m.width, starting from most recent.
	used := len(sep) // leading " ← "
	count := 0
	for _, lbl := range labels {
		need := len(lbl)
		if count > 0 {
			need += len(sep)
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
	errPart := ""
	if m.errMsg != "" {
		errPart = "  " + styleErr.Render("⚠ "+m.errMsg)
	}
	header := styleHeader.Render("◈ "+m.currentID) + load + errPart
	return lipgloss.NewStyle().Width(m.width).Render(header)
}

func (m tuiModel) renderBreadcrumbs() string {
	bc := m.breadcrumbs()
	if bc == "" {
		return ""
	}
	return lipgloss.NewStyle().Width(m.width).Render(bc)
}

// hint renders a key:description pair for the status bar.
func hint(key, desc string) string {
	return styleHintKey.Render(key) + styleHintSep.Render(":"+desc)
}

func (m tuiModel) renderStatus() string {
	var s string
	switch {
	case m.queryMode:
		s = "Query: " + m.queryInput.View() + styleHintSep.Render("  Esc:cancel")
	case m.searchMode:
		s = "Search: " + m.searchInput.View() + styleHintSep.Render("  Enter:keep  Esc:clear")
	case len(m.queryResults) > 0:
		sep := styleHintSep.Render("  ")
		s = strings.Join([]string{
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("Esc/b", "close"),
			hint("q", "quit"),
		}, sep)
	case m.queryResult != "":
		s = "↳ " + m.queryResult
	case m.searchQuery != "":
		sep := styleHintSep.Render("  ")
		s = styleSearch.Render(" / "+m.searchQuery+" ") + sep + strings.Join([]string{
			hint("f", "edit"),
			hint("Esc", "clear"),
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("q", "quit"),
		}, sep)
	case m.errMsg != "":
		s = hint("r", "retry") + "  " + hint("q", "quit")
	default:
		sep := styleHintSep.Render("  ")
		s = strings.Join([]string{
			hint("jk", "navigate"),
			hint("Enter", "go"),
			hint("b", "back"),
			hint("a", "all"),
			hint("c", "copy id"),
			hint("/", "query"),
			hint("f", "search"),
			hint("r", "refresh"),
			hint("ctrl+r", "clear cache"),
			hint("g/G", "body ↑↓ half"),
			hint("q", "quit"),
		}, sep)
	}
	return styleStatus.Width(m.width).Render(s)
}

// viewNarrow renders a compact single-column layout for terminals narrower than narrowThreshold.
func (m tuiModel) viewNarrow() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	listH := m.height - 1 - m.breadcrumbH() - 2 // -header -breadcrumbs -status -divider
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
		links = m.renderLinkList(w, listH)
	}

	rows := []string{m.renderHeader(), divider, links}
	if bc := m.renderBreadcrumbs(); bc != "" {
		rows = append(rows, bc)
	}
	rows = append(rows, m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m tuiModel) View() string {
	if !m.ready || m.width == 0 {
		return "Starting…"
	}

	if m.isNarrow() {
		return m.viewNarrow()
	}

	panels := lipgloss.JoinHorizontal(lipgloss.Top, m.renderBodyPanel(), m.renderLinksPanel())
	rows := []string{m.renderHeader(), panels}
	if bc := m.renderBreadcrumbs(); bc != "" {
		rows = append(rows, bc)
	}
	rows = append(rows, m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
