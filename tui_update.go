package main

import (
	"strings"

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

	// vertexLoadedMsg: cache hit or test — delivers fully-loaded state in one shot.
	case vertexLoadedMsg:
		if msg.gen != 0 && msg.gen != m.loadGen {
			return m, nil // stale
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
		m.showAll = false
		if m.cursor >= len(m.links) {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
			m.bodyVP.SetContent(m.bodyContent())
			m.bodyVP.GotoTop()
		}
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, nil

	// vertexInfoMsg: phase 1 complete — body is ready, start loading links.
	case vertexInfoMsg:
		if msg.gen != m.loadGen {
			return m, nil // stale, navigation changed
		}
		if msg.err != nil {
			m.loading = false
			m.linksTotal = 0
			m.errMsg = msg.err.Error()
			if m.ready {
				m.bodyVP.SetContent(m.bodyContent())
			}
			return m, nil
		}
		m.errMsg = ""
		m.currentID = msg.id
		m.fvi = msg.fvi
		m.links = nil
		m.showAll = false
		m.linksTotal = len(msg.fvi.outLinks) + len(msg.fvi.inLinks)
		if m.cursor >= m.linksTotal {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.Width = m.vpWidth()
			m.bodyVP.Height = m.vpHeight()
			m.bodyVP.SetContent(m.bodyContent())
			m.bodyVP.GotoTop()
		}
		if m.linksTotal == 0 {
			m.loading = false
			if err := gWalkTo(msg.id); err != nil {
				m.errMsg = "persist failed: " + err.Error()
			}
			return m, nil
		}
		return m, fetchLinksCmd(msg.id, msg.gen, msg.fvi)

	// linksLoadedMsg: phase 2 complete — all links fetched.
	case linksLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil // stale
		}
		m.loading = false
		m.linksTotal = 0
		m.links = msg.links
		if msg.partialErr != nil {
			m.errMsg = msg.partialErr.Error()
		}
		if m.fvi != nil {
			m.cache[msg.id] = cachedVertex{fvi: m.fvi, links: msg.links}
		}
		if m.cursor >= len(m.links) {
			m.cursor = 0
			m.linkOff = 0
		}
		if m.ready {
			m.bodyVP.SetContent(m.bodyContent())
		}
		if err := gWalkTo(msg.id); err != nil {
			if m.errMsg != "" {
				m.errMsg += "; "
			}
			m.errMsg += "persist failed: " + err.Error()
		}
		return m, nil

	case queryResultMsg:
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
	}

	if m.queryMode {
		return m.updateQuery(msg)
	}
	if m.searchMode {
		return m.updateSearch(msg)
	}
	return m.updateNav(msg)
}

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
			if len(m.links) > 0 {
				m.cursor = (m.cursor + 1) % len(m.links)
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil
		case "k", "up":
			if len(m.links) > 0 {
				m.cursor = (m.cursor - 1 + len(m.links)) % len(m.links)
				m = m.clampScroll()
				m.queryResult = ""
			}
			return m, nil

		case "enter":
			if m.loading || len(m.links) == 0 {
				break
			}
			target := m.links[m.cursor].target()
			if target == "" {
				break
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

		case "f":
			m.searchMode = true
			m.searchInput.SetValue(m.searchQuery)
			m.searchInput.Focus()
			return m, textinput.Blink

		case "esc":
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.searchInput.SetValue("")
				if m.ready {
					m.bodyVP.SetContent(m.bodyContent())
				}
			}
			return m, nil

		case "a":
			m.showAll = !m.showAll
			if m.ready {
				m.bodyVP.SetContent(m.bodyContent())
				m.bodyVP.GotoTop()
			}

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

		case "g":
			m.bodyVP.HalfPageUp()
		case "G":
			m.bodyVP.HalfPageDown()
		}
	}

	var cmd tea.Cmd
	m.bodyVP, cmd = m.bodyVP.Update(msg)
	return m, cmd
}

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
			return m, runQueryCmd(m.currentID, q)
		}
	}
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(msg)
	return m, cmd
}

func (m tuiModel) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.searchQuery = ""
			if m.ready {
				m.bodyVP.SetContent(m.bodyContent())
			}
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
	if m.ready {
		m.bodyVP.SetContent(m.bodyContent())
	}
	return m, cmd
}

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

// clampScroll keeps cursor visible in the link list.
func (m tuiModel) clampScroll() tuiModel {
	visible := m.panelContentH() - 1 // -1 for section title
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.linkOff {
		m.linkOff = m.cursor
	}
	if m.cursor >= m.linkOff+visible {
		m.linkOff = m.cursor - visible + 1
	}
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

// ── Search ────────────────────────────────────────────────────────────────────

// highlightMatches wraps all case-insensitive occurrences of query in text
// with styleSearch. Safe to call on strings that already contain ANSI codes
// because ANSI escape sequences don't contain printable search characters.
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
