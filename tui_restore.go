package main

// View-state preservation across a reload.
//
// A mutation is followed by a refetch, and without help that refetch throws the
// user back to the top of the link list with every group re-expanded.

// ── View-state preservation ───────────────────────────────────────────────────

// viewRestore is a snapshot of what the user was looking at, taken before a
// reload and re-applied after the groups are rebuilt.
//
// Group indices and flat-list indices are meaningless across a rebuild (groups
// are re-derived and re-sorted from scratch), so everything here is keyed by
// stable identity: the type name for a group, and the (type, name) pair — plus
// the source vertex for incoming links — for a selection.
type viewRestore struct {
	collapsedOut map[string]bool // key: group type name
	collapsedIn  map[string]bool

	selOutType, selOutName string
	selInType, selInFrom   string
	selInName              string

	focus panelFocus
}

// captureRestore snapshots collapse state and the current selection in both
// panels. Pure.
func captureRestore(m tuiModel) *viewRestore {
	r := &viewRestore{
		collapsedOut: map[string]bool{},
		collapsedIn:  map[string]bool{},
		focus:        m.focus,
	}
	for _, g := range m.grouped.outGroups {
		if g.collapsed {
			r.collapsedOut[g.tp] = true
		}
	}
	for _, g := range m.grouped.inGroups {
		if g.collapsed {
			r.collapsedIn[g.tp] = true
		}
	}

	if dl, ok := selectedLinkIn(m.grouped.outGroups, m.grouped.outFlat, m.rCursor); ok {
		r.selOutType = groupTypeOf(dl)
		r.selOutName = dl.info.id.name
	}
	if dl, ok := selectedLinkIn(m.grouped.inGroups, m.grouped.inFlat, m.lCursor); ok {
		r.selInType = groupTypeOf(dl)
		r.selInName = dl.info.id.name
		r.selInFrom = dl.info.id.from
	}
	return r
}

// applyRestore re-applies a snapshot to freshly rebuilt groups. Pure.
//
// Collapse state is restored by type name. The selection is restored by
// locating the remembered link; when it is gone — the normal case right after
// deleting it — the cursor lands on that type's group header instead, and if
// the whole group vanished, on the first row.
func applyRestore(m tuiModel, r *viewRestore) tuiModel {
	if r == nil {
		return m
	}

	for i := range m.grouped.outGroups {
		if r.collapsedOut[m.grouped.outGroups[i].tp] {
			m.grouped.outGroups[i].collapsed = true
		}
	}
	for i := range m.grouped.inGroups {
		if r.collapsedIn[m.grouped.inGroups[i].tp] {
			m.grouped.inGroups[i].collapsed = true
		}
	}
	m.grouped.rebuildFlat()

	m.focus = r.focus
	m.rCursor = locateCursor(m.grouped.outGroups, m.grouped.outFlat, r.selOutType,
		func(dl displayLink) bool { return dl.info.id.name == r.selOutName })
	m.lCursor = locateCursor(m.grouped.inGroups, m.grouped.inFlat, r.selInType,
		func(dl displayLink) bool {
			return dl.info.id.name == r.selInName && dl.info.id.from == r.selInFrom
		})

	return m.clampScroll()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// groupTypeOf returns the group key a link is bucketed under, mirroring
// buildGroupedView's empty-type substitution.
func groupTypeOf(dl displayLink) string {
	if dl.info.tp == "" {
		return "(no type)"
	}
	return dl.info.tp
}

// selectedLinkIn returns the link at a cursor position, if that row is a link.
func selectedLinkIn(groups []linkGroup, flat []flatItem, cursor int) (displayLink, bool) {
	if cursor < 0 || cursor >= len(flat) {
		return displayLink{}, false
	}
	return linkForItemIn(groups, flat[cursor])
}

// locateCursor finds the flat index of the link matching `match` within the
// group named `tp`. Falls back to that group's header, then to 0.
func locateCursor(groups []linkGroup, flat []flatItem, tp string, match func(displayLink) bool) int {
	if tp == "" {
		return 0
	}
	headerIdx := -1
	for i, item := range flat {
		if item.groupIdx >= len(groups) || groups[item.groupIdx].tp != tp {
			continue
		}
		if item.kind == flatTypeGroup {
			headerIdx = i
			continue
		}
		if dl, ok := linkForItemIn(groups, item); ok && match(dl) {
			return i
		}
	}
	if headerIdx >= 0 {
		return headerIdx
	}
	return 0
}
