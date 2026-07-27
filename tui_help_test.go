package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ── Discoverability ───────────────────────────────────────────────────────────

// TestHelp_ListsEveryNavKey is the guard that keeps a binding from shipping
// invisibly. The status bar can only advertise a handful of keys, so a key that
// is neither there nor in the help screen may as well not exist — which is
// exactly how i/d/D/x first shipped.
//
// Rather than re-listing the keymap (which would just drift in a second place),
// this walks the help table and checks every navigation binding appears in it.
func TestHelp_ListsEveryNavKey(t *testing.T) {
	navKeys := []string{
		"j", "k", "h", "l", "enter", "tab", "b", "R", "g", "G",
		"v", "c", "/", "f", "e", "r",
		"n", "L",
		"i", "t", "x", "y",
		"d", "D",
		"?",
	}

	var help strings.Builder
	for _, sec := range helpSections {
		for _, e := range sec.entries {
			help.WriteString(e.keys)
			help.WriteString(" ")
		}
	}
	listed := help.String()

	// "?" documents itself by being the way in; everything else must appear.
	var missing []string
	for _, k := range navKeys {
		if k == "?" {
			continue
		}
		if !mentionsKey(listed, k) {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("keys bound in the TUI but absent from the help screen: %v\n"+
			"a key nobody can discover may as well not exist — add it to helpSections", missing)
	}
}

// mentionsKey looks for a key as a standalone token, so "l" does not match the
// "l" inside "ctrl+s". Case-insensitive: the help spells the named keys the way
// a user reads them ("Enter", "Tab") while the bindings are lowercase.
func mentionsKey(haystack, key string) bool {
	for _, field := range strings.Fields(haystack) {
		if strings.EqualFold(field, key) {
			return true
		}
	}
	return false
}

func TestHelp_OpensAndAnyKeyCloses(t *testing.T) {
	m := makeModel("root", nil, nil)

	m = update(m, key("?"))
	if !m.helpOpen {
		t.Fatal("? should open the help screen")
	}
	if !strings.Contains(m.View(), "Keys") {
		t.Error("the help screen should be what View renders while it is open")
	}

	m = update(m, key("j"))
	if m.helpOpen {
		t.Error("any key should close the help screen")
	}
}

func TestHelp_DoesNotActOnTheKeyThatClosesIt(t *testing.T) {
	// Closing must swallow the keystroke: pressing D to dismiss help should not
	// then stage a delete.
	m := makeModel("hub/x", nil, nil)
	m = update(m, key("?"))
	m = update(m, key("D"))

	if m.helpOpen {
		t.Fatal("D should have closed the help screen")
	}
	if m.form != nil {
		t.Error("the key that closed the help screen must not also act")
	}
}

func TestHelp_CoversEveryEditorChord(t *testing.T) {
	var listed strings.Builder
	for _, sec := range helpSections {
		for _, e := range sec.entries {
			listed.WriteString(e.keys + " ")
		}
	}
	for _, chord := range []string{"ctrl+s", "ctrl+r", "ctrl+e"} {
		if !strings.Contains(listed.String(), chord) {
			t.Errorf("%s is bound inside the body editor but not documented", chord)
		}
	}
}

func TestHelp_RendersInANarrowTerminal(t *testing.T) {
	m := makeModel("root", nil, nil)
	m.width = 60
	m.height = 30
	m.helpOpen = true

	out := m.renderHelp()
	if out == "" {
		t.Fatal("help should render at 60 columns")
	}
	for _, line := range strings.Split(out, "\n") {
		if w := len([]rune(stripANSI(line))); w > m.width+2 {
			t.Errorf("help line overflows a %d-column terminal (%d cells): %q", m.width, w, line)
		}
	}
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
