package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
)

// The subject is the one thing every action applies to.
//
// The TUI used to have four different answers to "what am I acting on". `i`
// always meant the vertex, `d` meant the link when the cursor was on one and
// the vertex otherwise, `t` only ever meant a link, and `y` only ever meant the
// vertex. Four rules for one question, none of them stated anywhere, and the
// consequence was the complaint: there was no obvious way to edit a link,
// because the key that edits things did not consider links to be things.
//
// One rule replaces them. The cursor selects a subject — a link row means that
// link, anything else means the vertex — the centre panel shows that subject in
// full, and i / d / y act on whatever is shown. That is also what makes link
// editing discoverable rather than a binding to memorise: you can see the
// link's tags and body, so pressing the edit key over them is the obvious move.

type subjectKind int

const (
	// subjNone is a link panel with nothing link-shaped under the cursor — an
	// empty panel, or a group header. There is deliberately NO fallback to the
	// vertex: the vertex lives in the centre column, and letting a side column
	// act on it would undo the whole point of putting the centre in the focus
	// cycle. Standing in the outgoing panel and editing the vertex is exactly
	// the confusion the columns exist to remove.
	subjNone subjectKind = iota
	subjVertex
	subjLink
)

type subject struct {
	kind subjectKind
	link displayLink // subjLink only
}

// subject is decided by which column has focus. On the centre column it is
// the vertex; in a link panel it is the selected link — and when that panel is
// empty or the cursor is on a group header, there is no link to be the
// subject, so it falls back to the vertex.
func (m tuiModel) subject() subject {
	if !m.linkPanelFocused() {
		return subject{kind: subjVertex}
	}
	if dl, ok := m.cursorLink(); ok {
		return subject{kind: subjLink, link: dl}
	}
	return subject{kind: subjNone}
}

// noSubjectHint says what to do about it. A group header is a row you can act
// on with Tab, just not with the subject keys.
func (m tuiModel) noSubjectHint() string {
	if _, onHeader := m.cursorGroup(); onHeader {
		return "a type group is not an entity — pick a link, or h/l to the centre column for the vertex"
	}
	return "no links here — h/l to the centre column for the vertex"
}

// subjectLabel names the subject for a form title or a toast.
func (m tuiModel) subjectLabel(s subject) string {
	if s.kind == subjLink {
		return stripDomain(s.link.info.id.from) + " ──▶ " + stripDomain(s.link.info.to)
	}
	return stripDomain(m.currentID)
}

// linkEndpoints returns an edge's owner and target. The API addresses a link
// from its owner, and for an INCOMING link the owner is the far vertex — which
// is the one thing about links the panels cannot show, because both panels draw
// the far endpoint in the same column.
func linkEndpoints(dl displayLink) (owner, target string) {
	return dl.info.id.from, dl.info.to
}

// tierOfSubjectLink classifies the selected edge without a round trip.
//
// The far endpoint's kind comes from the edge itself, which settles it for
// every edge the CMDB owns — those are exactly the ones where the tier
// changes what happens. A user-typed edge between two objects is the case it
// cannot settle, and there it degrades to the raw API: correct, just less
// clever. Whichever it lands on is spelled out in the panel's `via` row, so
// the user is never guessing which API the next keystroke will call.
func (m tuiModel) tierOfSubjectLink(dl displayLink) linkTier {
	nearKind, _ := m.vertexKind()
	farKind, settled := inferFarKind(dl)
	if !settled {
		// Resolved by the detail read; until it lands the tier is provisional
		// and the panel says "reading the link…" anyway.
		if d, ok := m.linkDetails[keyOf(dl)]; ok && d.loaded {
			farKind = d.farKind
		}
	}
	return tierOfExistingLink(dl, nearKind, farKind, m.llMode)
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// renderLinkSubject draws the selected edge in the centre panel: everything the
// API needs to address it, plus the two things that were previously invisible
// anywhere in the interface — its tags and its body.
func (m tuiModel) renderLinkSubject(dl displayLink, w, h int) string {
	owner, target := linkEndpoints(dl)
	tier := m.tierOfSubjectLink(dl)

	label := func(k, v string) string {
		key := styleMetaKey.Render(padRight(k, 9))
		return "  " + key + truncateCells(v, maxInt(w-12, 4))
	}

	lines := []string{
		"  " + styleMetaVal.Render(truncateCells(stripDomain(owner), maxInt(w/2-4, 4))) +
			styleOut.Render(" ──▶ ") +
			styleMetaVal.Render(truncateCells(stripDomain(target), maxInt(w/2-4, 4))),
		"",
		label("name", dl.info.id.name),
		label("type", orDash(dl.info.tp)),
		label("via", tier.noun()),
	}

	if from, to, rel, ok := parseSuperLinkType(dl.info.tp); ok {
		lines = append(lines,
			label("as", from+" ──▶ "+to),
			label("rel", rel),
		)
	}

	detail, loaded := m.linkDetailFor(dl)
	switch {
	case !loaded:
		lines = append(lines, "", "  "+styleLoading.Render("reading the link…"))
	case detail.err != nil:
		lines = append(lines, "", "  "+styleErr.Render("✗ "+truncateCells(detail.err.Error(), maxInt(w-4, 8))))
	default:
		lines = append(lines, label("tags", renderTagList(detail.tags, w-12)))
		lines = append(lines, "", styleDim.Render("  body"))
		lines = append(lines, indentLines(renderBodyKV(detail.body.GetPtr(), w-4), "  ")...)
	}

	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func renderTagList(tags []string, w int) string {
	if len(tags) == 0 {
		return styleDim.Render("(none)")
	}
	return truncateCells(strings.Join(tags, ", "), maxInt(w, 4))
}

func indentLines(s, prefix string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = prefix + l
	}
	return out
}

func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// bodyOfSubject returns the body a yank or an edit should act on, and whether
// it is available yet.
func (m tuiModel) bodyOfSubject(s subject) (easyjson.JSON, bool) {
	if s.kind == subjLink {
		d, loaded := m.linkDetailFor(s.link)
		if !loaded || d.err != nil {
			return easyjson.NewJSONObject(), false
		}
		return d.body, true
	}
	if m.fvi == nil || m.fvi.body == nil {
		return easyjson.NewJSONObject(), false
	}
	return *m.fvi.body, true
}
