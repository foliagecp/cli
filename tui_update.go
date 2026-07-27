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
			// The key-value body bakes the width into the string, so it has to
			// be re-rendered rather than just re-sized.
			m = m.refreshBody()
		}
		if m.form != nil {
			if ed := m.form.jsonField(); ed != nil {
				ed.setSize(m.editorSizeFor(len(m.form.contextRows) + 2))
			}
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
		m, peek := m.peekCursorLink()
		return m, tea.Batch(fetchDepth2TypesCmd(m.loadGen, m.links), peek)

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
		if m.restore != nil {
			// Put the user back where they were standing. This never ran: the
			// snapshot was taken on the post-mutation refresh, but only the
			// CACHE-HIT handler consulted it — and a refresh deliberately
			// bypasses the cache. So every edit bounced the cursor to the top
			// of the list, and the stale snapshot was then applied to whatever
			// unrelated vertex was next served from cache.
			m = applyRestore(m, m.restore)
			m.restore = nil
		} else {
			m.rCursor = 0
			m.lCursor = 0
			m.rOffset = 0
			m.lOffset = 0
		}
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
		m, peek := m.peekCursorLink()
		return m, tea.Batch(fetchDepth2TypesCmd(m.loadGen, m.links), peek)

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

	case linkPeekMsg:
		return m.applyLinkPeek(msg)

	case linkDetailMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		return m.applyLinkDetail(msg), nil

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
			ed = m.form.jsonField()
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

	// ctrl+c quits from every mode, without exception. It used to quit in two
	// of them, close in three, and in search fall through into the text input
	// — so the one key every terminal program shares could not be relied on.
	if kMsg, isKey := msg.(tea.KeyMsg); isKey && kMsg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode() {
	case modeHelp:
		if _, isKey := msg.(tea.KeyMsg); isKey {
			m.helpOpen = false
		}
		return m, nil
	case modeForm:
		return m.updateForm(msg)
	case modeQuery:
		return m.updateQuery(msg)
	case modeSearch:
		return m.updateSearch(msg)
	case modeExport:
		return m.updateExport(msg)
	case modeGoto:
		return m.updateGoto(msg)
	case modeResults:
		return m.updateNavQueryResults(msg)
	}
	return m.updateNav(msg)
}

// ── Normal navigation ─────────────────────────────────────────────────────────

func (m tuiModel) updateNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit

		case "j", "down", "k", "up":
			dir := +1
			if msg.String() == "k" || msg.String() == "up" {
				dir = -1
			}
			flat := m.activeFlat()
			if len(flat) == 0 {
				return m, nil
			}
			m = m.setActiveCursor(nextSelectable(flat, m.activeCursorVal(), dir))
			m = m.clampScroll()
			m.queryResult = ""
			return m.peekCursorLink()

		case "enter":
			if m.loading {
				return m, nil
			}
			flat := m.activeFlat()
			cursor := m.activeCursorVal()
			if cursor < 0 || cursor >= len(flat) {
				return m, nil
			}
			item := flat[cursor]
			switch item.kind {
			case flatLink:
				dl, ok := linkForItemIn(m.activeGroups(), item)
				if !ok {
					return m, nil
				}
				m.queryResult = ""
				return m.navigateTo(dl.target())
			case flatTypeGroup:
				m = m.toggleCollapse(item)
			}
			return m, nil

		case "tab":
			flat := m.activeFlat()
			cursor := m.activeCursorVal()
			if cursor < 0 || cursor >= len(flat) {
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
			m.focus = m.focus.left()
			return m.peekCursorLink()

		case "l", "right":
			m.focus = m.focus.right()
			return m.peekCursorLink()

		case "b", "backspace":
			if len(m.history) > 0 {
				prev := m.history[len(m.history)-1]
				m.history = m.history[:len(m.history)-1]
				m.queryResult = ""
				return m.jumpTo(prev)
			}
			return m, nil

		case "v":
			m.rawBody = !m.rawBody
			m = m.refreshBody()
			if m.ready {
				m.bodyVP.GotoTop()
			}
			return m, nil

		case "i", "I":
			// Edit the SUBJECT — the selected link, or the vertex when the
			// cursor is not on one. `I` is the escape hatch that always means
			// the vertex, mirroring how `d`/`D` already work. Refused while a
			// load is in
			// flight: the form snapshots the body at open time, and there is
			// no point snapshotting one that is about to be replaced.
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			subj := m.subject()
			if msg.String() == "I" {
				subj = subject{kind: subjVertex}
			}
			return m.openSubjectEditor(subj, "body")

		case "L":
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			// Start here, walk anywhere, commit there. Two presses of the same
			// key, with a banner in between saying what is pending — rather
			// than a form asking the user to type a target id, which is the
			// one thing a graph browser exists to avoid.
			if m.linking == nil {
				kind, tp := m.vertexKind()
				m.linking = &pendingLink{fromID: m.currentID, kind: kind, typeName: tp}
				m.queryResult = ""
				return m, nil
			}
			f, refusal := openLinkCreateForm(m)
			if refusal != "" {
				m.queryResult = styleDim.Render(refusal)
				return m, nil
			}
			m.form = &f
			m.queryResult = ""
			return m, nil

		case "n":
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			f := openCreateMenu(m)
			m.form = &f
			m.queryResult = ""
			return m, nil

		case "t":
			// The same editor as `i`, landing on the tags row. Kept as its own
			// key because it was already documented as "edit the link's tags"
			// and that promise still holds exactly — what changed is that the
			// tags shown are now the real ones.
			if m.loading {
				m.queryResult = styleDim.Render("still loading…")
				return m, nil
			}
			subj := m.subject()
			if subj.kind != subjLink {
				m.queryResult = styleDim.Render("put the cursor on a link to edit its tags")
				return m, nil
			}
			return m.openSubjectEditor(subj, "tags")

		case "y":
			subj := m.subject()
			body, ok := m.bodyOfSubject(subj)
			if !ok {
				m.queryResult = styleDim.Render("nothing to yank")
				return m, nil
			}
			m.bodyRegister = prettyJSON(body)
			m.queryResult = styleDim.Render("yanked body of " + m.subjectLabel(subj))
			return m, nil

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

		case "?":
			m.helpOpen = true
			m.queryResult = ""
			return m, nil

		case "x":
			// Which API the CRUD keys use. Session state, and shown in both
			// positions in the status bar, so the default is a stated choice
			// rather than an unlabelled one.
			m.llMode = !m.llMode
			if m.llMode {
				m.queryResult = styleDim.Render(
					"CRUD switched to the low-level API — raw vertices and links, no CMDB semantics")
			} else {
				m.queryResult = styleDim.Render(
					"CRUD switched to the high-level API — types, objects and their links")
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

		case ":":
			m.gotoMode = true
			m.gotoInput.SetValue("")
			m.gotoInput.Focus()
			return m, textinput.Blink

		case "f":
			m.searchMode = true
			m.searchPrev = m.searchQuery
			m.searchInput.SetValue(m.searchQuery)
			m.searchInput.Focus()
			return m, textinput.Blink

		case "esc":
			if m.linking != nil {
				m.linking = nil
				m.queryResult = styleDim.Render("link cancelled")
				return m, nil
			}
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
			m = m.forgetLinkDetails()
			m.loading = true
			m.linksTotal = 0
			m.queryResult = ""
			m.loadGen++
			return m, fetchVertexCmd(m.currentID, m.loadGen)

		case "R":
			m.linking = nil
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
			return m, fetchVertexCmd(hubID("root"), m.loadGen)

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
		case "esc":
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
			// Cancel the edit, restoring the filter that was active when `f`
			// was pressed. It used to DESTROY the filter — even though `f`
			// deliberately pre-seeds the input with it, so the one thing Esc
			// could not do was leave things as they were found.
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m = m.applySearch(m.searchPrev)
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
	return m.applySearch(m.searchInput.Value()), cmd
}

// applySearch sets the live filter and rebuilds what depends on it.
func (m tuiModel) applySearch(q string) tuiModel {
	m.searchQuery = q
	m.grouped = buildGroupedView(m.links, q)
	m.rCursor = 0
	m.lCursor = 0
	m.rOffset = 0
	m.lOffset = 0
	return m.refreshBody()
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

	// The create menu is a letter-accelerator list, not a field form: a
	// keystroke picks an entry outright.
	if m.form.kind == formCreateMenu {
		return m.updateCreateMenu(kMsg)
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
			m.pendingNavAfterDelete = hubID("root")
			if n := len(m.history); n > 0 {
				m.pendingNavAfterDelete = m.history[n-1]
				m.history = m.history[:n-1]
			}
		}
		return m, submitFormCmd(f)
	case actOpenEditor:
		if ed := f.jsonField(); ed != nil {
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

// navigateTo walks to a vertex, pushing the current one onto the history.
//
// This is the ONE way to move. There used to be four copies of it — Enter on a
// link, `b`, the create menu and the query-result list each open-coded the same
// six lines — and they disagreed about the id form, so the same vertex could
// end up cached twice under `srv` and `hub/srv`.
func (m tuiModel) navigateTo(id string) (tuiModel, tea.Cmd) {
	id = canonID(id)
	if id == "" || id == m.currentID {
		return m, nil
	}
	m.history = append(m.history, m.currentID)
	// Deliberately does NOT clear the toast: several callers set one that
	// explains why they are moving you (the create menu says where types live
	// before taking you there), and clearing here would erase the explanation
	// in the same keystroke that earns it. Callers that want a clean slate
	// clear it themselves.
	return m.jumpTo(id)
}

// jumpTo loads a vertex without touching the history — for going back, and for
// landing on something just created.
func (m tuiModel) jumpTo(id string) (tuiModel, tea.Cmd) {
	id = canonID(id)
	if id == "" {
		return m, nil
	}
	m.loadGen++
	if cv, ok := m.cache[id]; ok {
		return m, cacheHitCmd(id, m.loadGen, cv)
	}
	m.loading = true
	m.linksTotal = 0
	return m, fetchVertexCmd(id, m.loadGen)
}

// updateCreateMenu turns a letter into the form it names.
func (m tuiModel) updateCreateMenu(kMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := kMsg.String()
	if k == "esc" || k == "q" {
		m.form = nil
		return m, nil
	}

	for _, e := range createMenuFor(m) {
		if e.key != k {
			continue
		}
		if !e.available() {
			// Rather than refusing, take the user where the action lives. The
			// menu already said where that is; going there makes the rule
			// concrete instead of theoretical.
			m.form = nil
			m.queryResult = styleDim.Render(e.why)
			return m.navigateTo(e.unavailableAt)
		}
		var f formState
		switch e.kind {
		case formVertexCreate:
			f = openVertexCreateForm(m)
		case formTypeCreate:
			f = openTypeCreateForm(m)
		case formObjectCreate:
			f = openObjectCreateForm(m)
		case formSubTypeSet:
			f = openSubTypeForm(m)
		case formLinkCreate:
			lf, refusal := openLinkCreateForm(m)
			if refusal != "" {
				m.form.err = refusal
				return m, nil
			}
			f = lf
		default:
			return m, nil
		}
		m.form = &f
		return m, nil
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
		// Only an APPLIED delete moves you off the vertex. A no-op delete —
		// the vertex was already gone — used to walk you away and pop the
		// history exactly as a real one would, so "nothing happened" and "it
		// is gone" were indistinguishable from the outside.
		if msg.res.status == opApplied {
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

	// A pending link is cleared only when its source really did go away.
	if msg.clearLinkIf != "" && msg.res.status == opApplied &&
		m.linking != nil && m.linking.fromID == msg.clearLinkIf {
		m.linking = nil
	}

	if msg.clearAll {
		m = m.invalidateAll()
	} else {
		m = m.invalidate(msg.invalidate...)
	}
	// Any write can rewrite the edges hanging off the vertices it touched, and
	// an edge detail is one small read to recover. Keeping a stale tag list
	// would be the same class of bug as the stale link list.
	m = m.forgetLinkDetails()

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
		case "q":
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
			return m.navigateTo(target)
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

// openSubjectEditor opens the editor for whatever the cursor is on.
//
// The link case cannot open synchronously: the vertex read that fills the link
// panels deliberately skips tags and bodies, so a form built from the model
// would show blanks over real data — and a REPLACE submit would then destroy
// what the user was never shown. It waits for the detail read instead, and
// says so.
func (m tuiModel) openSubjectEditor(subj subject, focusKey string) (tuiModel, tea.Cmd) {
	if subj.kind == subjLink {
		f, refusal := openLinkEditForm(m, subj.link, focusKey)
		if refusal != "" {
			m.queryResult = styleDim.Render(refusal)
			// Nothing has been read for this edge yet — ask for it now rather
			// than telling the user to press the key again.
			if _, known := m.linkDetails[keyOf(subj.link)]; !known {
				return m.applyLinkPeek(linkPeekMsg{key: keyOf(subj.link), gen: m.loadGen})
			}
			return m, nil
		}
		m.form = &f
		m.queryResult = ""
		return m, textarea.Blink
	}

	f, ok := openBodyEditForm(m)
	if !ok {
		m.queryResult = styleDim.Render("nothing to edit here")
		return m, nil
	}
	m.form = &f
	m.queryResult = ""
	return m, textarea.Blink
}

// updateGoto handles the id prompt.
//
// A graph browser with no address bar means the only way to a vertex you can
// name is to remember the path there — and R, the sole shortcut, resets the
// history, the filter and any pending link along with it. This is the one
// place where typing an id is the right interface rather than a failure of it.
func (m tuiModel) updateGoto(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kMsg, isKey := msg.(tea.KeyMsg); isKey {
		switch kMsg.String() {
		case "esc":
			m.gotoMode = false
			m.gotoInput.Blur()
			return m, nil
		case "tab":
			// Complete to the next structural destination. They are the ids
			// worth having a shortcut to and the ones hardest to remember the
			// path back to.
			m.gotoInput.SetValue(nextGotoSuggestion(m.gotoInput.Value()))
			m.gotoInput.CursorEnd()
			return m, nil
		case "enter":
			id := strings.TrimSpace(m.gotoInput.Value())
			m.gotoMode = false
			m.gotoInput.Blur()
			if id == "" {
				return m, nil
			}
			if err := validateID("vertex", id); err != nil {
				m.queryResult = styleErr.Render(err.Error())
				return m, nil
			}
			m.queryResult = ""
			return m.navigateTo(id)
		}
	}
	var cmd tea.Cmd
	m.gotoInput, cmd = m.gotoInput.Update(msg)
	return m, cmd
}

// gotoSuggestions are the structural vertices, offered because they are the
// destinations a user most often wants and least often remembers a route to.
var gotoSuggestions = []string{"root", "types", "objects", "trash_can", "group", "nav"}

func nextGotoSuggestion(cur string) string {
	cur = strings.TrimSpace(cur)
	for i, s := range gotoSuggestions {
		if s == cur || canonID(s) == cur {
			return gotoSuggestions[(i+1)%len(gotoSuggestions)]
		}
	}
	// Not on the list: start the cycle, unless the user is part-way through
	// typing one of them, in which case jump to the first match.
	if cur != "" {
		for _, s := range gotoSuggestions {
			if strings.HasPrefix(s, cur) {
				return s
			}
		}
	}
	return gotoSuggestions[0]
}
