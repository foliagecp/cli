package main

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ── Update ────────────────────────────────────────────────────────────────────

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.bodyVP = viewport.New(m.vpWidth(), m.vpHeight())
			m.bodyVP.SetContent(m.bodyContent())
			m.ready = true
		} else {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
		}
		return m, nil

	case depth2TypesMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		m.outTypes2 = msg.outTypes
		m.inTypes2 = msg.inTypes
		return m, nil

	case vertexLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		m.loading = false
		m.linksTotal = 0
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		if msg.partialErr != nil {
			m.errMsg = msg.partialErr.Error()
		}
		m.currentID = msg.id
		m.fvi = msg.fvi
		m.links = msg.links
		m.outTypes2 = nil
		m.inTypes2 = nil
		m.grouped = buildGroupedView(msg.links, m.searchQuery)
		if m.restore != nil {
			// Refresh triggered by the user's own edit (or by `r`): put them
			// back where they were rather than at the top of the list.
			m = applyRestore(m, m.restore)
			m.restore = nil
		} else {
			m.rCursor = 0
			m.lCursor = 0
			m.rOffset = 0
			m.lOffset = 0
		}
		m = m.refreshBody()
		if m.ready {
			m.bodyVP.GotoTop()
		}
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, fetchDepth2TypesCmd(m.loadGen, m.links)

	case vertexInfoMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		if msg.err != nil {
			m.loading = false
			m.linksTotal = 0
			m.errMsg = msg.err.Error()
			m = m.refreshBody()
			return m, nil
		}
		m.errMsg = ""
		m.currentID = msg.id
		m.fvi = msg.fvi
		m.links = nil
		m.outTypes2 = nil
		m.inTypes2 = nil
		m.grouped = groupedView{}
		m.linksTotal = len(msg.fvi.outLinks) + len(msg.fvi.inLinks)
		m.rCursor = 0
		m.lCursor = 0
		m.rOffset = 0
		m.lOffset = 0
		m = m.refreshBody()
		if m.ready {
			m.bodyVP.GotoTop()
		}
		if m.linksTotal == 0 {
			m.loading = false
			m.cache[msg.id] = cachedVertex{fvi: m.fvi, links: nil}
			if err := gWalkTo(msg.id); err != nil {
				m.errMsg = "persist failed: " + err.Error()
			}
			return m, nil
		}
		return m, fetchLinksCmd(msg.id, msg.gen, msg.fvi)

	case linksLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		m.loading = false
		m.linksTotal = 0
		m.links = msg.links
		m.grouped = buildGroupedView(msg.links, m.searchQuery)
		m.rCursor = 0
		m.lCursor = 0
		m.rOffset = 0
		m.lOffset = 0
		if msg.partialErr != nil {
			m.errMsg = msg.partialErr.Error()
		}
		if m.fvi != nil && msg.partialErr == nil {
			m.cache[msg.id] = cachedVertex{fvi: m.fvi, links: msg.links}
		}
		m = m.refreshBody()
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, fetchDepth2TypesCmd(m.loadGen, m.links)

	case queryResultMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		m.queryMode = false
		m.queryInput.Blur()
		m.queryInput.SetValue("")
		m.queryResults = nil
		if msg.err != nil {
			m.queryResult = styleErr.Render("error: " + msg.err.Error())
		} else if len(msg.results) == 0 {
			m.queryResult = styleDim.Render("(no results)")
		} else {
			m.queryResult = ""
			m.queryResults = msg.results
			m.qCursor = 0
			m.qOffset = 0
		}
		return m, nil

	case exportResultMsg:
		// Export result is always applied: the file was written regardless of
		// whether the user navigated away while waiting.
		if msg.err != nil {
			m.queryResult = styleErr.Render("export failed: " + msg.err.Error())
		} else {
			m.queryResult = styleDim.Render("✓ saved → " + msg.file)
		}
		return m, nil

	case mutationResultMsg:
		// Never gen-guarded: the write already happened on the server, so
		// dropping this would hide the outcome AND the cache invalidation.
		return m.applyMutationResult(msg)

	case editorDoneMsg:
		// Likewise unguarded — the user's edit exists and must not vanish.
		if msg.err != nil {
			m.queryResult = styleErr.Render("editor: " + msg.err.Error())
			return m, nil
		}
		ed := (*jsonEditor)(nil)
		if m.form != nil {
			ed = m.form.editor()
		}
		if ed == nil {
			// The form was closed while the editor ran. Say so rather than
			// swallowing the text — a silently discarded edit is worse than
			// a discarded edit the user knows about.
			m.queryResult = styleDim.Render("editor result discarded (form closed)")
			return m, nil
		}
		ed.ta.SetValue(msg.text)
		ed.validate()
		f := m.form.validate()
		m.form = &f
		return m, nil
	}

	// A form is modal: it is checked before every other mode.
	if m.form != nil {
		return m.updateForm(msg)
	}
	if m.queryMode {
		return m.updateQuery(msg)
	}
	if m.searchMode {
		return m.updateSearch(msg)
	}
	if m.exportMode {
		return m.updateExport(msg)
	}
	return m.updateNav(msg)
}

// ── Normal navigation ─────────────────────────────────────────────────────────

func (m tuiModel) updateNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.queryResults) > 0 {
		return m.updateNavQueryResults(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "j", "down":
			flat := m.activeFlat()
			if len(flat) > 0 {
				m = m.setActiveCursor(nextSelectable(flat, m.activeCursorVal(), +1))
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil

		case "k", "up":
			flat := m.activeFlat()
			if len(flat) > 0 {
				m = m.setActiveCursor(nextSelectable(flat, m.activeCursorVal(), -1))
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil

		case "enter":
			if m.loading {
				return m, nil
			}
			flat := m.activeFlat()
			cursor := m.activeCursorVal()
			if cursor >= len(flat) {
				return m, nil
			}
			item := flat[cursor]
			switch item.kind {
			case flatLink:
				dl, ok := linkForItemIn(m.activeGroups(), item)
				if !ok {
					return m, nil
				}
				target := dl.target()
				if target == "" {
					return m, nil
				}
				m.history = append(m.history, m.currentID)
				m.queryResult = ""
				m.loadGen++
				if cv, ok := m.cache[target]; ok {
					return m, cacheHitCmd(target, m.loadGen, cv)
				}
				m.loading = true
				m.linksTotal = 0
				return m, fetchVertexCmd(target, m.loadGen)
			case flatTypeGroup:
				m = m.toggleCollapse(item)
			}
			return m, nil

		case "tab":
			flat := m.activeFlat()
			cursor := m.activeCursorVal()
			if cursor >= len(flat) {
				return m, nil
			}
			item := flat[cursor]
			switch item.kind {
			case flatTypeGroup:
				m = m.toggleCollapse(item)
			case flatLink:
				// Find the nearest typeGroup above with the same groupIdx.
				for i := cursor - 1; i >= 0; i-- {
					if flat[i].kind == flatTypeGroup && flat[i].groupIdx == item.groupIdx {
						m = m.toggleCollapse(flat[i])
						break
					}
				}
			}
			return m, nil

		case "h", "left":
			m.focus = panelIn
			return m, nil

		case "l", "right":
			m.focus = panelOut
			return m, nil

		case "b", "backspace":
			if len(m.history) > 0 {
				prev := m.history[len(m.history)-1]
				m.history = m.history[:len(m.history)-1]
				m.queryResult = ""
				m.loadGen++
				if cv, ok := m.cache[prev]; ok {
					return m, cacheHitCmd(prev, m.loadGen, cv)
				}
				m.loading = true
				m.linksTotal = 0
				return m, fetchVertexCmd(prev, m.loadGen)
			}
			return m, nil

		case "v":
			m.rawBody = !m.rawBody
			m = m.refreshBody()
			if m.ready {
				m.bodyVP.GotoTop()
			}
			return m, nil

		case "i":
			// Edit the current vertex's body. Refused while a load is in
			// flight: the form snapshots the body at open time, and there is
			// no point snapshotting one that is about to be replaced.
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			f, ok := openBodyEditForm(m)
			if !ok {
				m.queryResult = styleDim.Render("nothing to edit here")
				return m, nil
			}
			m.form = &f
			m.queryResult = ""
			return m, textarea.Blink

		case "d", "D":
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			// `d` deletes what the cursor is on — a link row means that link;
			// anything else means the vertex. `D` always means the vertex, for
			// when the cursor happens to sit on a link.
			if msg.String() == "d" {
				if dl, ok := m.cursorLink(); ok {
					f := openDeleteLinkForm(m, dl)
					m.form = &f
					m.queryResult = ""
					return m, nil
				}
			}
			f, refusal := openDeleteVertexForm(m)
			if refusal != "" {
				m.queryResult = styleErr.Render(refusal)
				return m, nil
			}
			m.form = &f
			m.queryResult = ""
			return m, nil

		case "x":
			// Force the low-level API. Session state, shown in the header at
			// all times so it can never be on by surprise.
			m.llMode = !m.llMode
			if m.llMode {
				m.queryResult = styleDim.Render("low-level API on — typed operations bypassed")
			} else {
				m.queryResult = styleDim.Render("low-level API off")
			}
			return m, nil

		case "/":
			m.queryMode = true
			m.queryResult = ""
			m.queryInput.Focus()
			return m, textinput.Blink

		case "c":
			if err := copyToClipboard(m.currentID); err == nil {
				m.queryResult = styleDim.Render("✓ " + m.currentID)
			} else {
				m.queryResult = styleErr.Render("copy failed: " + err.Error())
			}
			return m, nil

		case "e":
			m.exportMode = true
			m.exportDepStep = false
			m.exportFmtIdx = 0
			m.exportDepthIdx = 0
			m.exportInput.Blur()
			m.exportInput.SetValue("")
			return m, nil

		case "f":
			m.searchMode = true
			m.searchInput.SetValue(m.searchQuery)
			m.searchInput.Focus()
			return m, textinput.Blink

		case "esc":
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.grouped = buildGroupedView(m.links, "")
				m.rCursor = 0
				m.lCursor = 0
				m.rOffset = 0
				m.lOffset = 0
				m = m.refreshBody()
			}
			return m, nil

		case "r":
			delete(m.cache, m.currentID)
			m.loading = true
			m.linksTotal = 0
			m.queryResult = ""
			m.loadGen++
			return m, fetchVertexCmd(m.currentID, m.loadGen)

		case "ctrl+r":
			m.cache = make(map[string]cachedVertex)
			m.loading = true
			m.linksTotal = 0
			m.queryResult = ""
			m.loadGen++
			return m, fetchVertexCmd(m.currentID, m.loadGen)

		case "R":
			m.queryResult = ""
			m.queryResults = nil
			m.errMsg = ""
			m.history = nil
			m.searchQuery = ""
			m.searchInput.SetValue("")
			m.outTypes2 = nil
			m.inTypes2 = nil
			m.grouped = buildGroupedView(nil, "")
			m.loadGen++
			m.loading = true
			m.linksTotal = 0
			return m, fetchVertexCmd("root", m.loadGen)

		case "g":
			m.bodyVP.HalfPageUp()
			return m, nil
		case "G":
			m.bodyVP.HalfPageDown()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.bodyVP, cmd = m.bodyVP.Update(msg)
	return m, cmd
}

// toggleCollapse flips the collapsed state of the group at item.groupIdx in the
// active panel, rebuilds both flat lists, and keeps the cursor on the header.
func (m tuiModel) toggleCollapse(item flatItem) tuiModel {
	if item.kind != flatTypeGroup {
		return m
	}
	if m.focus == panelOut {
		if item.groupIdx >= len(m.grouped.outGroups) {
			return m
		}
		m.grouped.outGroups[item.groupIdx].collapsed = !m.grouped.outGroups[item.groupIdx].collapsed
	} else {
		if item.groupIdx >= len(m.grouped.inGroups) {
			return m
		}
		m.grouped.inGroups[item.groupIdx].collapsed = !m.grouped.inGroups[item.groupIdx].collapsed
	}
	m.grouped.rebuildFlat()

	// Keep cursor on the same group header after rebuild.
	for i, fi := range m.activeFlat() {
		if fi.kind == flatTypeGroup && fi.groupIdx == item.groupIdx {
			m = m.setActiveCursor(i)
			break
		}
	}
	m = m.clampScroll()
	return m
}

// ── Query mode ────────────────────────────────────────────────────────────────

func (m tuiModel) updateQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.queryMode = false
			m.queryInput.Blur()
			m.queryInput.SetValue("")
			return m, nil
		case "enter":
			q := strings.TrimSpace(m.queryInput.Value())
			m.queryInput.SetValue("")
			if q == "" {
				m.queryMode = false
				m.queryInput.Blur()
				return m, nil
			}
			m.queryMode = false
			m.queryInput.Blur()
			return m, runQueryCmd(m.loadGen, m.currentID, q)
		}
	}
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(msg)
	return m, cmd
}

// ── Search mode ───────────────────────────────────────────────────────────────

func (m tuiModel) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.searchQuery = ""
			m.grouped = buildGroupedView(m.links, "")
			m.rCursor = 0
			m.lCursor = 0
			m.rOffset = 0
			m.lOffset = 0
			m = m.refreshBody()
			return m, nil
		case "enter":
			m.searchQuery = m.searchInput.Value()
			m.searchMode = false
			m.searchInput.Blur()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchQuery = m.searchInput.Value()
	m.grouped = buildGroupedView(m.links, m.searchQuery)
	m.rCursor = 0
	m.lCursor = 0
	m.rOffset = 0
	m.lOffset = 0
	m = m.refreshBody()
	return m, cmd
}

// ── Form mode ─────────────────────────────────────────────────────────────────

func (m tuiModel) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	kMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Cursor blink and the like belong to the widget.
		if ed := m.form.editor(); ed != nil {
			var cmd tea.Cmd
			ed.ta, cmd = ed.ta.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	f, action := m.form.handleKey(kMsg.String())
	m.form = &f

	switch action {
	case actClose:
		m.form = nil
		return m, nil
	case actSubmit:
		// Deleting the vertex we are standing on must move us somewhere alive.
		// gWalkTo persists the current id on every successful load, so staying
		// put would leave a dead id in $FOLIAGE_CLI_DIR/gwalk and the NEXT
		// launch of `tui` (and `gwalk inspect`) would fail to load at all.
		// Decided here, before the delete, while the history is still intact.
		if f.kind == formDeleteVertex && f.ctx.fromID == m.currentID {
			m.pendingNavAfterDelete = "root"
			if n := len(m.history); n > 0 {
				m.pendingNavAfterDelete = m.history[n-1]
				m.history = m.history[:n-1]
			}
		}
		return m, submitFormCmd(f)
	case actOpenEditor:
		if ed := f.editor(); ed != nil {
			return m, openEditorCmd(ed.ta.Value())
		}
		return m, nil
	}

	// Not consumed by the state machine: it belongs to the focused widget.
	if ed := f.editor(); ed != nil {
		var cmd tea.Cmd
		ed.ta, cmd = ed.ta.Update(msg)
		ed.validate()
		vf := f.validate()
		m.form = &vf
		return m, cmd
	}
	return m, nil
}

// applyMutationResult folds a completed mutation back into the model: report
// it, evict what it invalidated, and reload if the view is now stale.
func (m tuiModel) applyMutationResult(msg mutationResultMsg) (tea.Model, tea.Cmd) {
	m.form = nil
	m.queryResult = toastFor(msg)

	navTo := msg.navTo
	if m.pendingNavAfterDelete != "" {
		if msg.res.status != opFailed {
			navTo = m.pendingNavAfterDelete
		}
		m.pendingNavAfterDelete = ""
	}

	if msg.res.status == opFailed {
		// Failures also go to the header, which persists until the next
		// successful load — a cursor move must not wipe the reason.
		m.errMsg = msg.res.details
		return m, nil
	}
	m.errMsg = ""

	if msg.clearAnchorIf != "" && m.anchor != nil && m.anchor.id == msg.clearAnchorIf {
		m.anchor = nil
	}

	if msg.clearAll {
		m = m.invalidateAll()
	} else {
		m = m.invalidate(msg.invalidate...)
	}

	if navTo != "" {
		m.loadGen++
		m.loading = true
		m.linksTotal = 0
		return m, fetchVertexCmd(navTo, m.loadGen)
	}

	if msg.refresh {
		// Keep the user where they were standing across the reload.
		m.restore = captureRestore(m)
		m.loadGen++
		m.loading = true
		m.linksTotal = 0
		return m, fetchVertexCmd(m.currentID, m.loadGen)
	}
	return m, nil
}

// ── Export mode ───────────────────────────────────────────────────────────────

func (m tuiModel) updateExport(msg tea.Msg) (tea.Model, tea.Cmd) {
	kMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if m.exportDepStep {
			var cmd tea.Cmd
			m.exportInput, cmd = m.exportInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	key := kMsg.String()

	if !m.exportDepStep {
		// ── Step 1: format selection ──────────────────────────────────────
		n := len(exportFmts)
		switch key {
		case "ctrl+c":
			m.exportMode = false
			return m, nil
		case "esc":
			m.exportMode = false
			return m, nil
		case "right", "l", "tab":
			m.exportFmtIdx = (m.exportFmtIdx + 1) % n
		case "left", "h", "shift+tab":
			m.exportFmtIdx = (m.exportFmtIdx - 1 + n) % n
		case "enter":
			m.exportDepStep = true
			m.exportDepthIdx = 0
			m.exportInput.SetValue("")
			m.exportInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}

	// ── Step 2: depth entry ───────────────────────────────────────────────
	switch key {
	case "ctrl+c":
		m.exportMode = false
		m.exportDepStep = false
		m.exportInput.Blur()
		m.exportInput.SetValue("")
		return m, nil
	case "esc":
		// back to format step
		m.exportDepStep = false
		m.exportInput.Blur()
		m.exportInput.SetValue("")
		return m, nil
	case "tab":
		m.exportDepthIdx = (m.exportDepthIdx + 1) % len(exportDepthPresets)
		m.exportInput.SetValue(exportDepthPresets[m.exportDepthIdx])
		m.exportInput.CursorEnd()
		return m, nil
	case "shift+tab":
		m.exportDepthIdx = (m.exportDepthIdx - 1 + len(exportDepthPresets)) % len(exportDepthPresets)
		m.exportInput.SetValue(exportDepthPresets[m.exportDepthIdx])
		m.exportInput.CursorEnd()
		return m, nil
	case "enter":
		depth := -1
		if val := strings.TrimSpace(m.exportInput.Value()); val != "" {
			if n, err := strconv.Atoi(val); err == nil {
				depth = n
			}
		}
		format := exportFmts[m.exportFmtIdx].value
		m.exportMode = false
		m.exportDepStep = false
		m.exportInput.Blur()
		m.exportInput.SetValue("")
		m.queryResult = styleLoading.Render("exporting…")
		return m, runExportCmd(m.loadGen, m.currentID, format, depth)
	}

	// Pass other keystrokes to textinput (digits, backspace, etc.)
	var cmd tea.Cmd
	m.exportInput, cmd = m.exportInput.Update(msg)
	return m, cmd
}

// ── Query results navigation ──────────────────────────────────────────────────

func (m tuiModel) updateNavQueryResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if len(m.queryResults) > 0 {
				m.qCursor = (m.qCursor + 1) % len(m.queryResults)
				m = m.clampQueryScroll()
			}
			return m, nil
		case "k", "up":
			if len(m.queryResults) > 0 {
				m.qCursor = (m.qCursor - 1 + len(m.queryResults)) % len(m.queryResults)
				m = m.clampQueryScroll()
			}
			return m, nil
		case "enter":
			target := m.queryResults[m.qCursor]
			m.queryResults = nil
			m.queryResult = ""
			if target == m.currentID {
				return m, nil
			}
			m.history = append(m.history, m.currentID)
			m.loadGen++
			if cv, ok := m.cache[target]; ok {
				return m, cacheHitCmd(target, m.loadGen, cv)
			}
			m.loading = true
			m.linksTotal = 0
			return m, fetchVertexCmd(target, m.loadGen)
		case "esc", "b", "backspace":
			m.queryResults = nil
			m.queryResult = ""
			return m, nil
		}
	}
	return m, nil
}

// ── Scroll helpers ────────────────────────────────────────────────────────────

func (m tuiModel) clampScroll() tuiModel {
	flat := m.activeFlat()
	cursor := m.activeCursorVal()
	offset := m.activeOffsetVal()

	visible := m.panelContentH() - 1
	if visible < 1 {
		visible = 1
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	if offset < 0 {
		offset = 0
	}
	maxOffset := len(flat) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m = m.setActiveOffset(offset)
	return m
}

func (m tuiModel) clampQueryScroll() tuiModel {
	visible := m.panelContentH() - 1
	if visible < 1 {
		visible = 1
	}
	if m.qCursor < m.qOffset {
		m.qOffset = m.qCursor
	}
	if m.qCursor >= m.qOffset+visible {
		m.qOffset = m.qCursor - visible + 1
	}
	return m
}
