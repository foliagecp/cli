package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/foliagecp/easyjson"
)

// ── truncateCells ─────────────────────────────────────────────────────────────

// TestTruncate_CountsCellsNotEscapeBytes. The loop used to measure every byte
// of an SGR sequence as a visible cell, so a styled string lost ten to fifteen
// columns per span — and the cut could land mid-sequence, leaving colour
// bleeding across the rest of the screen.
func TestTruncate_CountsCellsNotEscapeBytes(t *testing.T) {
	styled := styleOk.Render("hello") + " " + styleErr.Render("world")
	if w := lipgloss.Width(styled); w != 11 {
		t.Fatalf("fixture width = %d, want 11", w)
	}

	got := truncateCells(styled, 8)
	if w := lipgloss.Width(got); w > 8 {
		t.Errorf("truncated to %d visible cells, want at most 8: %q", w, got)
	}
	if plain := stripANSI(got); !strings.HasPrefix(plain, "hello") {
		t.Errorf("visible text = %q — the escape bytes were counted as content", plain)
	}
}

func TestTruncate_NeverLeavesAnUnterminatedSequence(t *testing.T) {
	styled := styleOk.Render("aaaaaaaaaaaaaaaaaaaa")
	got := truncateCells(styled, 6)

	if strings.Count(got, "\x1b[") > 0 && !strings.HasSuffix(stripTrailing(got), "\x1b[0m") {
		t.Errorf("truncation left a style open: %q", got)
	}
}

// stripTrailing drops the ellipsis so the reset can be checked at the end.
func stripTrailing(s string) string { return strings.TrimSuffix(s, "…") }

func TestTruncate_PassesShortStringsThrough(t *testing.T) {
	for _, s := range []string{"", "abc", styleOk.Render("abc")} {
		if got := truncateCells(s, 10); got != s {
			t.Errorf("truncateCells(%q, 10) = %q, want it unchanged", s, got)
		}
	}
}

// ── Narrow layout ─────────────────────────────────────────────────────────────

func narrow(m tuiModel) tuiModel {
	m.width, m.height = 60, 24
	return m
}

// TestNarrow_AModalIsNeverInvisible is the fix for the worst rendering bug in
// the TUI: below 90 columns every chromeCenter form drew nothing at all while
// swallowing every keystroke. The user saw an unchanged screen with ordinary
// nav hints under it, and their typing went into an input box that was not on
// screen.
func TestNarrow_AModalIsNeverInvisible(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := narrow(makeModel("hub/x", nil, &fvi))
	m = update(m, key("n")) // the create menu — chromeCenter

	if m.form == nil {
		t.Fatal("n should open the create menu")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "New…") {
		t.Errorf("a modal that eats every key must be on screen:\n%s", out)
	}
}

func TestNarrow_ShowsTheLowLevelMarker(t *testing.T) {
	// x arms the low-level API for every subsequent write. The marker lived in
	// the centre panel title, which the narrow layout does not have — so the
	// mode was invisible exactly where the screen is most cramped.
	fvi := makeVertexInfo("hub/x", nil, nil)
	m := narrow(makeModel("hub/x", nil, &fvi))
	m = update(m, key("x"))

	if !strings.Contains(stripANSI(m.View()), "[LL]") {
		t.Error("low-level mode must be visible at every width — it changes what writes do")
	}
}

func TestNarrow_ShowsTheSelectedLink(t *testing.T) {
	m := narrow(linkWithDetail(t, rawLink(), []string{"prod"}, numBody("weight", 7)))

	out := stripANSI(m.View())
	if !strings.Contains(out, "prod") {
		t.Errorf("the selected link's detail should render at 60 columns:\n%s", out)
	}
}

func TestNarrow_ExplainsAnEmptyFilterResult(t *testing.T) {
	// The guard tested m.links while the rendering read m.grouped, so a filter
	// that matched nothing produced a blank region with no explanation.
	m := narrow(makeModel("hub/x", threeLinks(), nil))
	m = m.applySearch("zzzz")

	if !strings.Contains(stripANSI(m.View()), "no matches") {
		t.Error("a filter that matches nothing must say so, not render blank")
	}
}

func TestNarrow_EveryModeRendersSomething(t *testing.T) {
	fvi := makeVertexInfo("hub/x", nil, nil)
	base := func() tuiModel { return narrow(makeModel("hub/x", threeLinks(), &fvi)) }

	cases := map[string]tuiModel{
		"browse": base(),
		"query":  update(base(), key("/")),
		"search": update(base(), key("f")),
		"export": update(base(), key("e")),
		"create": update(base(), key("n")),
		"help":   update(base(), key("?")),
	}
	for name, m := range cases {
		out := strings.TrimSpace(stripANSI(m.View()))
		if out == "" {
			t.Errorf("%s renders an empty screen at 60 columns", name)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > m.width+2 {
				t.Errorf("%s: line overflows %d columns (%d cells): %q", name, m.width, w, line)
			}
		}
	}
}

func TestNarrow_BodyEditorStillOwnsTheScreen(t *testing.T) {
	body := easyjson.NewJSONObject()
	body.SetByPath("k", easyjson.NewJSON("v"))
	fvi := fullVertexInfo{id: "hub/x", body: body.GetPtr()}
	m := narrow(makeModel("hub/x", nil, &fvi))
	m = update(m, key("I"))

	if m.form == nil || m.form.chrome != chromeFull {
		t.Fatal("I should open the full-screen editor")
	}
	if !strings.Contains(stripANSI(m.View()), "\"k\"") {
		t.Error("the editor should render its content at 60 columns")
	}
}

// ── Honest reporting ──────────────────────────────────────────────────────────

// TestNoop_DoesNotWalkYouOffTheVertex. A delete that deleted nothing used to
// pop the history and navigate away exactly as a real one did, so "already
// gone" and "just removed" were indistinguishable from the outside.
func TestNoop_DoesNotWalkYouOffTheVertex(t *testing.T) {
	withOps(t, graphOps{
		vertexDelete: func(string) opResult {
			return opResult{status: opNoop, details: "does not exist"}
		},
	})

	m := makeModel("hub/x", nil, nil)
	m.history = []string{"hub/parent"}
	m = update(m, key("D"))
	_, cmd := updateCmd(m, key("y"))

	next := update(m, runCmd(cmd))
	if next.currentID != "hub/x" {
		t.Errorf("currentID = %q — a no-op delete must leave you where you are", next.currentID)
	}
	if len(next.history) != 1 {
		t.Errorf("history = %v — a no-op delete must not pop it", next.history)
	}
}

func TestStatus_KeepsTheHintsAlongsideAToast(t *testing.T) {
	// A single toast used to take the whole line, and the keymap vanished
	// until something happened to clear it — which for several keys is never.
	m := makeModel("hub/x", nil, nil)
	m.width = 120
	m.queryResult = "yanked body of x"

	out := stripANSI(m.renderStatus())
	if !strings.Contains(out, "yanked") {
		t.Error("the toast should be shown")
	}
	if !strings.Contains(out, "?") {
		t.Errorf("the hints must survive a toast:\n%s", out)
	}
}

func TestStatus_AnErrorIsAlwaysReachable(t *testing.T) {
	// errMsg sat below both the toast and the active-filter case, so an error
	// raised while either was showing was displayed nowhere at all.
	m := makeModel("hub/x", threeLinks(), nil)
	m.width = 120
	m = m.applySearch("l1")
	m.queryResult = "a toast"
	m.errMsg = "the server said no"

	if !strings.Contains(stripANSI(m.renderStatus()), "the server said no") {
		t.Error("an error must reach the screen regardless of what else is showing")
	}
}

func TestRestore_PutsTheCursorBackAfterAnEdit(t *testing.T) {
	// The snapshot was taken on the post-mutation refresh, but only the
	// CACHE-HIT handler read it — and a refresh deliberately bypasses the
	// cache. So it never once ran, and the stale snapshot was later applied to
	// an unrelated vertex.
	links := threeLinks()
	m := makeModel("hub/x", links, nil)
	m.rCursor = 2 // the second link
	m.restore = captureRestore(m)

	m = update(m, linksLoadedMsg{id: "hub/x", gen: m.loadGen, links: links})

	if m.rCursor != 2 {
		t.Errorf("rCursor = %d after a refresh, want 2 — the cursor must not jump to the top", m.rCursor)
	}
	if m.restore != nil {
		t.Error("the snapshot must be consumed, or it gets applied to the next vertex")
	}
}
