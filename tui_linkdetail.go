package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/foliagecp/easyjson"
)

// Link details — the tags and body of one edge.
//
// The vertex read that fills the link panels deliberately does not carry them
// (one request instead of one-per-link, which matters on a type with thousands
// of instances). fullLinkInfo has the fields anyway, and on that path they are
// always nil — a struct that lies about itself. Every consumer so far believed
// it: the tags editor seeded its field from `dl.info.tags`, so it opened BLANK
// over links that had tags, and ticking `replace` then erased them. The user
// never saw what they destroyed.
//
// So details are fetched, once, for the link the user is actually looking at,
// and nothing reads them out of displayLink again.

type linkKey struct{ from, name string }

func keyOf(dl displayLink) linkKey {
	return linkKey{from: dl.info.id.from, name: dl.info.id.name}
}

func (k linkKey) empty() bool { return k.from == "" && k.name == "" }

// linkDetail is what a read returned. `loaded` distinguishes "fetched and the
// link genuinely has no tags" from "not fetched yet" — the distinction the old
// code could not make, and the reason it could not tell an empty tag list from
// an unknown one.
type linkDetail struct {
	loaded bool
	tags   []string
	body   easyjson.JSON
	err    error
}

// linkPeekMsg fires after the cursor has rested on a link. Holding `j` down a
// list of fifty links should not issue fifty reads, and a debounce is the
// honest way to say "the user stopped here" — the alternative, fetching on
// every cursor move, turns scrolling into a request storm.
type linkPeekMsg struct {
	key linkKey
	gen int
}

type linkDetailMsg struct {
	key    linkKey
	gen    int
	detail linkDetail
}

const linkPeekDelay = 120 * time.Millisecond

func peekLinkCmd(key linkKey, gen int) tea.Cmd {
	return tea.Tick(linkPeekDelay, func(time.Time) tea.Msg {
		return linkPeekMsg{key: key, gen: gen}
	})
}

// fetchLinkDetailCmd reads one edge through the low-level API.
//
// The tier does not matter here. `typeslink.read` and `objectslink.read` both
// delegate to the same graph link read, so the raw call returns the same body
// and tags for less ceremony — and it works even when the CMDB layer would
// refuse to resolve the edge, which is exactly when a user most wants to look
// at it. Tier routing belongs on the write path, where it changes what happens.
func fetchLinkDetailCmd(key linkKey, gen int) tea.Cmd {
	return func() tea.Msg {
		data, err := ops.linkRead(key.from, key.name)
		if err != nil {
			return linkDetailMsg{key: key, gen: gen, detail: linkDetail{loaded: true, err: err}}
		}
		d := linkDetail{loaded: true, tags: []string{}, body: easyjson.NewJSONObject()}
		if arr, ok := data.GetByPath("tags").AsArrayString(); ok {
			d.tags = arr
		}
		if b := data.GetByPath("body"); b.IsObject() {
			d.body = b
		}
		return linkDetailMsg{key: key, gen: gen, detail: d}
	}
}

// linkDetailFor returns what is known about a link right now. The second value
// is false while a read is outstanding, so callers can say "loading" instead of
// showing an empty body as though it were the truth.
func (m tuiModel) linkDetailFor(dl displayLink) (linkDetail, bool) {
	d, ok := m.linkDetails[keyOf(dl)]
	return d, ok && d.loaded
}

// peekCursorLink schedules a detail read for whatever the cursor now rests on.
// Call it after any cursor or focus movement.
func (m tuiModel) peekCursorLink() (tuiModel, tea.Cmd) {
	dl, ok := m.cursorLink()
	if !ok {
		m.linkPeek = linkKey{}
		return m, nil
	}
	key := keyOf(dl)
	if key.empty() {
		m.linkPeek = linkKey{}
		return m, nil
	}
	m.linkPeek = key
	if _, known := m.linkDetails[key]; known {
		return m, nil
	}
	return m, peekLinkCmd(key, m.loadGen)
}

// applyLinkPeek handles the debounce firing: read the link only if the cursor
// is still on it and nothing has answered for it in the meantime.
func (m tuiModel) applyLinkPeek(msg linkPeekMsg) (tuiModel, tea.Cmd) {
	if msg.gen != m.loadGen || msg.key != m.linkPeek {
		return m, nil
	}
	if _, known := m.linkDetails[msg.key]; known {
		return m, nil
	}
	if m.linkDetails == nil {
		m.linkDetails = map[linkKey]linkDetail{}
	}
	// Recorded as in-flight so a second debounce cannot double-fetch; `loaded`
	// stays false, which is what makes the view say "reading…".
	m.linkDetails[msg.key] = linkDetail{}
	return m, fetchLinkDetailCmd(msg.key, m.loadGen)
}

func (m tuiModel) applyLinkDetail(msg linkDetailMsg) tuiModel {
	if m.linkDetails == nil {
		m.linkDetails = map[linkKey]linkDetail{}
	}
	m.linkDetails[msg.key] = msg.detail
	return m
}

// forgetLinkDetails drops cached edge contents. Any write to a vertex can
// rewrite the edges hanging off it, so the details go whenever the vertex
// cache does.
func (m tuiModel) forgetLinkDetails() tuiModel {
	m.linkDetails = map[linkKey]linkDetail{}
	return m
}
